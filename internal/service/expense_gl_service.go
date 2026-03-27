package service

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

type ExpenseGLService struct {
	q          *sqlc.Queries
	journalSvc *JournalService
}

func NewExpenseGLService(q *sqlc.Queries, journalSvc *JournalService) *ExpenseGLService {
	return &ExpenseGLService{q: q, journalSvc: journalSvc}
}

// CreateAccrualEntry creates a GL journal entry on expense report approval.
// DR: expense accounts (grouped by GL account), CR: employee reimbursement payable.
// Returns nil, nil if GL is not configured (no EXPENSE_PAYABLE_ACCOUNT_ID env var).
func (s *ExpenseGLService) CreateAccrualEntry(ctx context.Context, report *domain.ExpenseReport, items []domain.ExpenseItem) (*uuid.UUID, error) {
	payableAccountIDStr := os.Getenv("EXPENSE_PAYABLE_ACCOUNT_ID")
	if payableAccountIDStr == "" {
		return nil, nil // no GL config, skip
	}
	payableAccountID, err := uuid.Parse(payableAccountIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid EXPENSE_PAYABLE_ACCOUNT_ID: %w", err)
	}

	suspenseAccountIDStr := os.Getenv("EXPENSE_SUSPENSE_ACCOUNT_ID")
	var suspenseAccountID *uuid.UUID
	if suspenseAccountIDStr != "" {
		id, err := uuid.Parse(suspenseAccountIDStr)
		if err == nil {
			suspenseAccountID = &id
		}
	}

	// Group items by GL account, summing amounts per group
	type accountGroup struct {
		accountID uuid.UUID
		total     decimal.Decimal
	}
	groups := make(map[uuid.UUID]*accountGroup)
	var totalAmount decimal.Decimal

	for _, item := range items {
		var accountID uuid.UUID
		if item.GLAccountID != nil {
			accountID = *item.GLAccountID
		} else if suspenseAccountID != nil {
			accountID = *suspenseAccountID
		} else {
			continue // skip items without GL mapping
		}

		g, ok := groups[accountID]
		if !ok {
			g = &accountGroup{accountID: accountID, total: decimal.Zero}
			groups[accountID] = g
		}
		g.total = g.total.Add(item.Amount)
		totalAmount = totalAmount.Add(item.Amount)
	}

	if len(groups) == 0 || totalAmount.IsZero() {
		return nil, nil // nothing to post
	}

	// Build debit lines: one per expense account group
	lines := make([]CreateJournalLineInput, 0, len(groups)+1)
	for _, g := range groups {
		lines = append(lines, CreateJournalLineInput{
			AccountID: g.accountID,
			Debit:     g.total,
			Credit:    decimal.Zero,
		})
	}
	// Credit line: employee reimbursement payable account
	lines = append(lines, CreateJournalLineInput{
		AccountID: payableAccountID,
		Debit:     decimal.Zero,
		Credit:    totalAmount,
	})

	sourceType := string(domain.SourceExpenseReimbursement)
	memo := fmt.Sprintf("Expense Report %s: %s", report.ReportNumber, report.Title)

	je, err := s.journalSvc.Create(ctx, CreateJournalEntryInput{
		CompanyID:  report.CompanyID,
		EntryDate:  time.Now(),
		SourceType: &sourceType,
		SourceID:   &report.ID,
		Memo:       &memo,
		CreatedBy:  report.SubmitterUserID,
		Lines:      lines,
	})
	if err != nil {
		return nil, fmt.Errorf("create accrual journal entry: %w", err)
	}

	return &je.ID, nil
}

// CreatePaymentEntry creates a GL journal entry when an expense report is marked paid.
// DR: employee reimbursement payable, CR: cash/bank account.
// Returns nil, nil if GL is not configured.
func (s *ExpenseGLService) CreatePaymentEntry(ctx context.Context, report *domain.ExpenseReport) (*uuid.UUID, error) {
	payableAccountIDStr := os.Getenv("EXPENSE_PAYABLE_ACCOUNT_ID")
	cashAccountIDStr := os.Getenv("EXPENSE_CASH_ACCOUNT_ID")
	if payableAccountIDStr == "" || cashAccountIDStr == "" {
		return nil, nil // no GL config
	}

	payableAccountID, err := uuid.Parse(payableAccountIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid EXPENSE_PAYABLE_ACCOUNT_ID: %w", err)
	}
	cashAccountID, err := uuid.Parse(cashAccountIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid EXPENSE_CASH_ACCOUNT_ID: %w", err)
	}

	sourceType := string(domain.SourceExpensePayment)
	memo := fmt.Sprintf("Payment: Expense Report %s", report.ReportNumber)

	lines := []CreateJournalLineInput{
		{
			AccountID: payableAccountID,
			Debit:     report.TotalAmount,
			Credit:    decimal.Zero,
		},
		{
			AccountID: cashAccountID,
			Debit:     decimal.Zero,
			Credit:    report.TotalAmount,
		},
	}

	je, err := s.journalSvc.Create(ctx, CreateJournalEntryInput{
		CompanyID:  report.CompanyID,
		EntryDate:  time.Now(),
		SourceType: &sourceType,
		SourceID:   &report.ID,
		Memo:       &memo,
		CreatedBy:  report.SubmitterUserID,
		Lines:      lines,
	})
	if err != nil {
		return nil, fmt.Errorf("create payment journal entry: %w", err)
	}

	return &je.ID, nil
}
