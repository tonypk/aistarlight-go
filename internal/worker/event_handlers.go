package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/event"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/birforms"
)

// handleJournalPosted reacts to a posted journal entry by auto-calculating
// a tax draft for the entry's period.
func (s *Server) handleJournalPosted(ctx context.Context, task *asynq.Task) error {
	var payload event.JournalPostedPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		slog.Error("event handler: unmarshal journal_posted", "error", err)
		return err
	}

	slog.Info("event: journal_posted received",
		"journal_entry_id", payload.JournalEntryID,
		"company_id", payload.CompanyID,
	)

	if s.svc.GLTaxBridge == nil {
		slog.Warn("event handler: GLTaxBridge not available, skipping tax draft calculation")
		return nil
	}

	// Get the journal entry to determine its period
	entry, err := s.q.GetJournalEntryByID(ctx, payload.JournalEntryID)
	if err != nil {
		slog.Error("event handler: get journal entry", "error", err, "id", payload.JournalEntryID)
		return nil // don't retry — entry might have been deleted
	}

	if !entry.EntryDate.Valid {
		slog.Warn("event handler: journal entry has no entry_date", "id", payload.JournalEntryID)
		return nil
	}

	// Derive the monthly period from the entry date
	entryDate := entry.EntryDate.Time
	periodStart := time.Date(entryDate.Year(), entryDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, -1) // last day of month

	// Calculate 2550M (Monthly VAT) from GL
	result, err := s.svc.GLTaxBridge.Calculate2550M(ctx, payload.CompanyID, periodStart, periodEnd)
	if err != nil {
		slog.Error("event handler: calculate 2550M failed",
			"error", err,
			"company_id", payload.CompanyID,
			"period", periodStart.Format("2006-01"),
		)
		return nil // don't retry — GL data issue, not transient
	}

	// Persist the tax draft
	resultJSON, err := json.Marshal(result.Result)
	if err != nil {
		slog.Error("event handler: marshal tax result", "error", err)
		return nil
	}

	triggeredBy := "journal_posted"
	_, err = s.q.UpsertTaxDraft(ctx, sqlc.UpsertTaxDraftParams{
		CompanyID:   payload.CompanyID,
		FormType:    birforms.FormBIR2550M,
		PeriodStart: pgtype.Date{Time: periodStart, Valid: true},
		PeriodEnd:   pgtype.Date{Time: periodEnd, Valid: true},
		Result:      resultJSON,
		TriggeredBy: &triggeredBy,
	})
	if err != nil {
		slog.Error("event handler: upsert tax draft", "error", err)
		return err // retry — DB issue might be transient
	}

	slog.Info("event: tax draft auto-calculated",
		"company_id", payload.CompanyID,
		"form_type", birforms.FormBIR2550M,
		"period", periodStart.Format("2006-01"),
		"triggered_by", triggeredBy,
	)

	return nil
}

// handleReconciliationCompleted auto-creates draft journal entries for
// matched transactions that don't already have journals.
func (s *Server) handleReconciliationCompleted(ctx context.Context, task *asynq.Task) error {
	var payload event.ReconciliationCompletedPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		slog.Error("event handler: unmarshal reconciliation_completed", "error", err)
		return err
	}

	slog.Info("event: reconciliation_completed received",
		"batch_id", payload.BatchID,
		"company_id", payload.CompanyID,
		"match_count", payload.MatchCount,
		"period", payload.Period,
	)

	if s.svc.Journal == nil {
		slog.Warn("event handler: JournalService not available, skipping auto-journal")
		return nil
	}

	// Find matched transactions without journal entries
	txns, err := s.q.GetMatchedTransactionsWithoutJournal(ctx, payload.CompanyID)
	if err != nil {
		slog.Error("event handler: get matched transactions", "error", err)
		return nil
	}

	if len(txns) == 0 {
		slog.Info("event handler: no matched transactions needing journals")
		return nil
	}

	// Resolve the bank account (1010 = Cash in Bank)
	bankAccount, err := s.q.GetAccountByNumber(ctx, sqlc.GetAccountByNumberParams{
		CompanyID:     payload.CompanyID,
		AccountNumber: "1010",
	})
	if err != nil {
		slog.Warn("event handler: bank account 1010 not found, skipping auto-journal",
			"company_id", payload.CompanyID, "error", err)
		return nil
	}

	created := 0
	for _, txn := range txns {
		je, err := s.createJournalForTransaction(ctx, txn, bankAccount.ID, payload.CompanyID)
		if err != nil {
			slog.Warn("event handler: create journal for transaction failed",
				"txn_id", txn.ID, "error", err)
			continue
		}

		// Link the transaction to the journal entry
		_ = s.q.LinkTransactionToJournalEntry(ctx, sqlc.LinkTransactionToJournalEntryParams{
			ID:             txn.ID,
			JournalEntryID: pgtype.UUID{Bytes: je.ID, Valid: true},
		})
		created++
	}

	slog.Info("event: auto-journals created from reconciliation",
		"batch_id", payload.BatchID,
		"total_matched", len(txns),
		"journals_created", created,
	)

	return nil
}

// createJournalForTransaction builds a draft journal entry for a single matched transaction.
// Sales (source_type=sales_record/sales) → DR Bank, CR Revenue
// Purchases → DR Expense, CR Bank
func (s *Server) createJournalForTransaction(
	ctx context.Context,
	txn sqlc.Transaction,
	bankAccountID uuid.UUID,
	companyID uuid.UUID,
) (*domain.JournalEntry, error) {
	amount := numericToDecimal(txn.Amount)
	if amount.IsZero() {
		return nil, fmt.Errorf("zero amount transaction")
	}

	// Determine the contra account based on source_type and category
	contraAccountID, err := s.resolveContraAccount(ctx, companyID, txn.SourceType, txn.Category)
	if err != nil {
		return nil, fmt.Errorf("resolve contra account: %w", err)
	}

	absAmount := amount.Abs()

	var lines []service.CreateJournalLineInput
	isSale := txn.SourceType == "sales_record" || txn.SourceType == "sales"

	desc := "Auto-generated from bank reconciliation"
	if txn.Description != nil {
		desc = *txn.Description
	}

	if isSale {
		// Sales: DR Bank, CR Revenue
		lines = []service.CreateJournalLineInput{
			{AccountID: bankAccountID, Debit: absAmount, Credit: decimal.Zero},
			{AccountID: contraAccountID, Debit: decimal.Zero, Credit: absAmount},
		}
	} else {
		// Purchases/expenses: DR Expense, CR Bank
		lines = []service.CreateJournalLineInput{
			{AccountID: contraAccountID, Debit: absAmount, Credit: decimal.Zero},
			{AccountID: bankAccountID, Debit: decimal.Zero, Credit: absAmount},
		}
	}

	entryDate := time.Now()
	if txn.Date.Valid {
		entryDate = txn.Date.Time
	}

	sourceType := string(domain.SourceReconciliation)
	sourceID := txn.ID
	ref := fmt.Sprintf("Recon:%s", txn.ID.String()[:8])

	return s.svc.Journal.Create(ctx, service.CreateJournalEntryInput{
		CompanyID:   companyID,
		EntryDate:   entryDate,
		Reference:   &ref,
		Description: &desc,
		SourceType:  &sourceType,
		SourceID:    &sourceID,
		CreatedBy:   uuid.Nil, // system-generated
		Lines:       lines,
	})
}

// resolveContraAccount maps transaction source_type + category to a COA account.
func (s *Server) resolveContraAccount(ctx context.Context, companyID uuid.UUID, sourceType, category string) (uuid.UUID, error) {
	// Default account numbers by category
	accountNumber := categoryToAccountNumber(sourceType, category)

	acct, err := s.q.GetAccountByNumber(ctx, sqlc.GetAccountByNumberParams{
		CompanyID:     companyID,
		AccountNumber: accountNumber,
	})
	if err != nil {
		// Fallback: try sub_type mapping
		subType := category
		acct2, err2 := s.q.GetAccountBySubType(ctx, sqlc.GetAccountBySubTypeParams{
			CompanyID: companyID,
			SubType:   &subType,
		})
		if err2 != nil {
			return uuid.Nil, fmt.Errorf("no account for category %q (tried %s): %w", category, accountNumber, err)
		}
		return acct2.ID, nil
	}
	return acct.ID, nil
}

// categoryToAccountNumber maps transaction category to the default COA account number.
func categoryToAccountNumber(sourceType, category string) string {
	isSale := sourceType == "sales_record" || sourceType == "sales"

	if isSale {
		return "4000" // Sales Revenue - Vatable
	}

	switch category {
	case "goods":
		return "5000" // COGS
	case "services":
		return "5010" // Cost of Services
	case "capital":
		return "1500" // Property, Plant & Equipment
	case "imports":
		return "5000" // COGS (imported goods)
	default:
		return "6900" // Miscellaneous Expense
	}
}

func numericToDecimal(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}
