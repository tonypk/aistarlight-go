package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// JournalGenerator auto-generates double-entry journal entries from transactions.
type JournalGenerator struct {
	q  *sqlc.Queries
	js *JournalService
}

// NewJournalGenerator creates a JournalGenerator.
func NewJournalGenerator(q *sqlc.Queries, js *JournalService) *JournalGenerator {
	return &JournalGenerator{q: q, js: js}
}

// GenerateFromTransaction creates a journal entry from a single transaction.
func (g *JournalGenerator) GenerateFromTransaction(ctx context.Context, companyID uuid.UUID, txnID uuid.UUID, userID uuid.UUID) (*domain.JournalEntry, error) {
	txn, err := g.q.GetTransactionByID(ctx, txnID)
	if err != nil {
		return nil, fmt.Errorf("get transaction: %w", err)
	}
	if txn.CompanyID != companyID {
		return nil, fmt.Errorf("transaction does not belong to company")
	}
	if txn.JournalEntryID.Valid {
		return nil, fmt.Errorf("transaction already linked to journal entry")
	}

	entry, err := g.createJournalFromTxn(ctx, companyID, txn, userID)
	if err != nil {
		return nil, err
	}

	// Link transaction to journal entry
	if err := g.q.LinkTransactionToJournalEntry(ctx, sqlc.LinkTransactionToJournalEntryParams{
		ID:             txnID,
		JournalEntryID: pgtype.UUID{Bytes: entry.ID, Valid: true},
	}); err != nil {
		return nil, fmt.Errorf("link transaction to JE: %w", err)
	}

	// Auto-post the journal entry
	if err := g.js.Post(ctx, entry.ID, userID); err != nil {
		return nil, fmt.Errorf("auto-post JE: %w", err)
	}

	return g.js.GetByID(ctx, entry.ID)
}

// GenerateFromSession creates journal entries for all unlinked transactions in a session.
func (g *JournalGenerator) GenerateFromSession(ctx context.Context, companyID uuid.UUID, sessionID uuid.UUID, userID uuid.UUID) ([]*domain.JournalEntry, error) {
	txns, err := g.q.ListAllTransactionsBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session transactions: %w", err)
	}

	var entries []*domain.JournalEntry
	for _, txn := range txns {
		if txn.CompanyID != companyID {
			continue
		}
		if txn.JournalEntryID.Valid {
			continue // already linked
		}

		entry, err := g.createJournalFromTxn(ctx, companyID, txn, userID)
		if err != nil {
			return nil, fmt.Errorf("generate JE for txn %s: %w", txn.ID, err)
		}

		if err := g.q.LinkTransactionToJournalEntry(ctx, sqlc.LinkTransactionToJournalEntryParams{
			ID:             txn.ID,
			JournalEntryID: pgtype.UUID{Bytes: entry.ID, Valid: true},
		}); err != nil {
			return nil, fmt.Errorf("link txn %s: %w", txn.ID, err)
		}

		if err := g.js.Post(ctx, entry.ID, userID); err != nil {
			return nil, fmt.Errorf("auto-post JE for txn %s: %w", txn.ID, err)
		}

		posted, err := g.js.GetByID(ctx, entry.ID)
		if err != nil {
			return nil, err
		}
		entries = append(entries, posted)
	}

	return entries, nil
}

// BatchGenerate creates journal entries for specified transaction IDs.
func (g *JournalGenerator) BatchGenerate(ctx context.Context, companyID uuid.UUID, txnIDs []uuid.UUID, userID uuid.UUID) ([]*domain.JournalEntry, error) {
	var entries []*domain.JournalEntry
	for _, txnID := range txnIDs {
		entry, err := g.GenerateFromTransaction(ctx, companyID, txnID, userID)
		if err != nil {
			return nil, fmt.Errorf("generate JE for txn %s: %w", txnID, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (g *JournalGenerator) createJournalFromTxn(ctx context.Context, companyID uuid.UUID, txn sqlc.Transaction, userID uuid.UUID) (*domain.JournalEntry, error) {
	lines, err := g.buildJournalLines(ctx, companyID, txn)
	if err != nil {
		return nil, fmt.Errorf("build journal lines: %w", err)
	}

	desc := buildJournalDescription(txn)
	ref := fmt.Sprintf("TXN-%s", txn.ID.String()[:8])
	sourceType := "transaction"
	sourceID := txn.ID

	var entryDate time.Time
	if txn.Date.Valid {
		entryDate = txn.Date.Time
	} else {
		entryDate = time.Now()
	}

	entry, err := g.js.Create(ctx, CreateJournalEntryInput{
		CompanyID:   companyID,
		EntryDate:   entryDate,
		Reference:   &ref,
		Description: &desc,
		SourceType:  &sourceType,
		SourceID:    &sourceID,
		CreatedBy:   userID,
		Lines:       lines,
	})
	if err != nil {
		return nil, fmt.Errorf("create journal entry: %w", err)
	}

	return entry, nil
}

func (g *JournalGenerator) buildJournalLines(ctx context.Context, companyID uuid.UUID, txn sqlc.Transaction) ([]CreateJournalLineInput, error) {
	amount := numericToDecimal(txn.Amount)
	vatAmount := numericToDecimal(txn.VatAmount)
	netAmount := amount.Sub(vatAmount)

	category := txn.Category
	vatType := txn.VatType

	// Determine if this is a sale or purchase/expense
	isSale := category == "sale"

	var lines []CreateJournalLineInput

	if isSale {
		lines = g.buildSaleLines(netAmount, vatAmount, amount, vatType)
	} else {
		lines = g.buildPurchaseExpenseLines(netAmount, vatAmount, amount, vatType, category, txn)
	}

	// Resolve account numbers to IDs
	resolved, err := g.resolveAccountIDs(ctx, companyID, lines)
	if err != nil {
		return nil, err
	}

	return resolved, nil
}

// accountMapping maps account numbers to journal line amounts.
type accountMapping struct {
	accountNumber string
	description   string
	debit         decimal.Decimal
	credit        decimal.Decimal
}

func (g *JournalGenerator) buildSaleLines(netAmount, vatAmount, totalAmount decimal.Decimal, vatType string) []CreateJournalLineInput {
	var mappings []accountMapping

	switch vatType {
	case "vatable":
		// DR Cash/AR for total, CR Sales Revenue + CR Output VAT
		mappings = append(mappings, accountMapping{
			accountNumber: "1010", description: "Cash received from sale",
			debit: totalAmount,
		})
		mappings = append(mappings, accountMapping{
			accountNumber: "4000", description: "Vatable sales revenue",
			credit: netAmount,
		})
		if vatAmount.IsPositive() {
			mappings = append(mappings, accountMapping{
				accountNumber: "2200", description: "Output VAT on sale",
				credit: vatAmount,
			})
		}

	case "zero_rated":
		mappings = append(mappings, accountMapping{
			accountNumber: "1010", description: "Cash received from zero-rated sale",
			debit: totalAmount,
		})
		mappings = append(mappings, accountMapping{
			accountNumber: "4010", description: "Zero-rated sales revenue",
			credit: totalAmount,
		})

	case "exempt":
		mappings = append(mappings, accountMapping{
			accountNumber: "1010", description: "Cash received from exempt sale",
			debit: totalAmount,
		})
		mappings = append(mappings, accountMapping{
			accountNumber: "4020", description: "VAT-exempt sales revenue",
			credit: totalAmount,
		})

	default: // government or other
		mappings = append(mappings, accountMapping{
			accountNumber: "1010", description: "Cash received from sale",
			debit: totalAmount,
		})
		mappings = append(mappings, accountMapping{
			accountNumber: "4000", description: "Sales revenue",
			credit: netAmount,
		})
		if vatAmount.IsPositive() {
			mappings = append(mappings, accountMapping{
				accountNumber: "2200", description: "Output VAT",
				credit: vatAmount,
			})
		}
	}

	return mappingsToLines(mappings)
}

func (g *JournalGenerator) buildPurchaseExpenseLines(netAmount, vatAmount, totalAmount decimal.Decimal, vatType, category string, txn sqlc.Transaction) []CreateJournalLineInput {
	var mappings []accountMapping

	// Determine expense/purchase account
	expenseAcct := mapCategoryToAccount(category)

	isVatable := vatType == "vatable" && vatAmount.IsPositive()
	hasEWT := txn.EwtAmount.Valid && numericToDecimal(txn.EwtAmount).IsPositive()

	// Debit: Expense/Purchase account
	if isVatable {
		mappings = append(mappings, accountMapping{
			accountNumber: expenseAcct, description: fmt.Sprintf("Purchase/expense: %s", category),
			debit: netAmount,
		})
		// Debit: Input VAT
		mappings = append(mappings, accountMapping{
			accountNumber: "1400", description: "Input VAT on purchase",
			debit: vatAmount,
		})
	} else {
		// No VAT separation
		mappings = append(mappings, accountMapping{
			accountNumber: expenseAcct, description: fmt.Sprintf("Purchase/expense: %s", category),
			debit: totalAmount,
		})
	}

	// Credit: Cash or AP
	cashCredit := totalAmount
	if hasEWT {
		ewtAmt := numericToDecimal(txn.EwtAmount)
		cashCredit = totalAmount.Sub(ewtAmt)
		// Credit: EWT Payable
		mappings = append(mappings, accountMapping{
			accountNumber: "2210", description: "EWT payable",
			credit: ewtAmt,
		})
	}

	mappings = append(mappings, accountMapping{
		accountNumber: "1010", description: "Cash payment",
		credit: cashCredit,
	})

	return mappingsToLines(mappings)
}

// mapCategoryToAccount maps transaction categories to COA account numbers.
func mapCategoryToAccount(category string) string {
	switch strings.ToLower(category) {
	case "goods":
		return "5000" // Cost of Goods Sold
	case "services":
		return "5010" // Cost of Services
	case "capital":
		return "1500" // Property, Plant & Equipment
	case "imports":
		return "5000" // COGS for imports
	case "rent":
		return "6100" // Rent Expense
	case "utilities":
		return "6110" // Utilities Expense
	case "professional_fees":
		return "6300" // Professional Fees
	case "office_supplies":
		return "6200" // Office Supplies
	case "transportation":
		return "6400" // Transportation & Travel
	case "representation":
		return "6310" // Advertising & Marketing
	case "salaries", "payroll":
		return "6000" // Salaries & Wages
	case "depreciation":
		return "6500" // Depreciation Expense
	case "insurance":
		return "6600" // Insurance Expense
	case "taxes":
		return "6700" // Taxes & Licenses
	default:
		return "6900" // Miscellaneous Expense
	}
}

func mappingsToLines(mappings []accountMapping) []CreateJournalLineInput {
	lines := make([]CreateJournalLineInput, len(mappings))
	for i, m := range mappings {
		desc := m.description
		lines[i] = CreateJournalLineInput{
			// AccountID will be resolved later
			Description: &desc,
			Debit:       m.debit,
			Credit:      m.credit,
		}
		// Store account number temporarily in a tag field — we'll use a wrapper
		lines[i].accountNumber = m.accountNumber
	}
	return lines
}

func (g *JournalGenerator) resolveAccountIDs(ctx context.Context, companyID uuid.UUID, lines []CreateJournalLineInput) ([]CreateJournalLineInput, error) {
	resolved := make([]CreateJournalLineInput, len(lines))
	for i, line := range lines {
		acctNumber := line.accountNumber
		if acctNumber == "" {
			return nil, fmt.Errorf("missing account number for line %d", i)
		}

		acct, err := g.q.GetAccountByNumber(ctx, sqlc.GetAccountByNumberParams{
			CompanyID:     companyID,
			AccountNumber: acctNumber,
		})
		if err != nil {
			return nil, fmt.Errorf("account %s not found (seed COA first): %w", acctNumber, err)
		}

		resolved[i] = CreateJournalLineInput{
			AccountID:   acct.ID,
			Description: line.Description,
			Debit:       line.Debit,
			Credit:      line.Credit,
		}
	}
	return resolved, nil
}

func buildJournalDescription(txn sqlc.Transaction) string {
	parts := []string{}
	if txn.Description != nil && *txn.Description != "" {
		parts = append(parts, *txn.Description)
	}
	parts = append(parts, fmt.Sprintf("Category: %s", txn.Category))
	parts = append(parts, fmt.Sprintf("VAT: %s", txn.VatType))
	return strings.Join(parts, " | ")
}
