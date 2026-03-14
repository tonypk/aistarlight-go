package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// YearEndCloseService handles year-end closing entries.
// It closes revenue and expense accounts to Income Summary,
// then closes Income Summary to Retained Earnings.
type YearEndCloseService struct {
	q          *sqlc.Queries
	journalSvc *JournalService
	accountSvc *AccountService
}

func NewYearEndCloseService(q *sqlc.Queries, journalSvc *JournalService, accountSvc *AccountService) *YearEndCloseService {
	return &YearEndCloseService{q: q, journalSvc: journalSvc, accountSvc: accountSvc}
}

type YearEndCloseResult struct {
	FiscalYear      int                    `json:"fiscal_year"`
	RevenueTotal    decimal.Decimal        `json:"revenue_total"`
	COGSTotal       decimal.Decimal        `json:"cogs_total"`
	ExpenseTotal    decimal.Decimal        `json:"expense_total"`
	NetIncome       decimal.Decimal        `json:"net_income"`
	JournalEntryIDs []uuid.UUID            `json:"journal_entry_ids"`
	Entries         []*domain.JournalEntry `json:"entries,omitempty"`
}

// Close performs year-end closing for a given fiscal year.
// Steps:
// 1. Close revenue accounts (4xxx) → Income Summary (3900)
// 2. Close COGS (5xxx) and expense accounts (6xxx) → Income Summary (3900)
// 3. Close Income Summary → Retained Earnings (3100)
func (s *YearEndCloseService) Close(ctx context.Context, companyID uuid.UUID, fiscalYear int, closedBy uuid.UUID) (*YearEndCloseResult, error) {
	// Determine fiscal year dates
	startDate := time.Date(fiscalYear, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(fiscalYear, 12, 31, 0, 0, 0, 0, time.UTC)
	closeDate := endDate

	// Find Income Summary account (3900)
	incomeSummaryAcct, err := s.q.GetAccountByNumber(ctx, sqlc.GetAccountByNumberParams{
		CompanyID:     companyID,
		AccountNumber: "3900",
	})
	if err != nil {
		return nil, fmt.Errorf("Income Summary account (3900) not found: seed COA first")
	}

	// Find Retained Earnings account (3100)
	retainedEarningsAcct, err := s.q.GetAccountByNumber(ctx, sqlc.GetAccountByNumberParams{
		CompanyID:     companyID,
		AccountNumber: "3100",
	})
	if err != nil {
		return nil, fmt.Errorf("Retained Earnings account (3100) not found: seed COA first")
	}

	// Get all period balances for revenue/COGS/expense accounts
	rows, err := s.q.PeriodAllAccountBalances(ctx, sqlc.PeriodAllAccountBalancesParams{
		CompanyID: companyID,
		FromDate:  pgtype.Date{Time: startDate, Valid: true},
		ToDate:    pgtype.Date{Time: endDate, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("query period balances: %w", err)
	}

	var revenueLines []CreateJournalLineInput
	var expenseLines []CreateJournalLineInput
	revenueTotal := decimal.Zero
	cogsTotal := decimal.Zero
	expenseTotal := decimal.Zero

	for _, row := range rows {
		balance := computeNormalBalance(
			numericToDecimal(row.TotalDebit),
			numericToDecimal(row.TotalCredit),
			row.NormalBalance,
		)
		if balance.IsZero() {
			continue
		}

		prefix := row.AccountNumber[:1]
		switch prefix {
		case "4": // Revenue (credit-normal → debit to close)
			revenueTotal = revenueTotal.Add(balance)
			revenueLines = append(revenueLines, CreateJournalLineInput{
				AccountID: row.AccountID,
				Debit:     balance,
				Credit:    decimal.Zero,
			})
		case "5": // COGS (debit-normal → credit to close)
			cogsTotal = cogsTotal.Add(balance)
			expenseLines = append(expenseLines, CreateJournalLineInput{
				AccountID: row.AccountID,
				Debit:     decimal.Zero,
				Credit:    balance,
			})
		case "6": // Expenses (debit-normal → credit to close)
			expenseTotal = expenseTotal.Add(balance)
			expenseLines = append(expenseLines, CreateJournalLineInput{
				AccountID: row.AccountID,
				Debit:     decimal.Zero,
				Credit:    balance,
			})
		}
	}

	result := &YearEndCloseResult{
		FiscalYear:   fiscalYear,
		RevenueTotal: revenueTotal,
		COGSTotal:    cogsTotal,
		ExpenseTotal: expenseTotal,
		NetIncome:    revenueTotal.Sub(cogsTotal).Sub(expenseTotal),
	}

	sourceType := string(domain.SourceYearEndClose)

	// Entry 1: Close Revenue → Income Summary
	if len(revenueLines) > 0 {
		// Revenue accounts are debited; Income Summary is credited
		revenueLines = append(revenueLines, CreateJournalLineInput{
			AccountID: incomeSummaryAcct.ID,
			Debit:     decimal.Zero,
			Credit:    revenueTotal,
		})
		ref := fmt.Sprintf("YE-%d Close Revenue", fiscalYear)
		desc := fmt.Sprintf("Year-end close: revenue accounts to Income Summary for FY%d", fiscalYear)
		entry, err := s.journalSvc.Create(ctx, CreateJournalEntryInput{
			CompanyID:   companyID,
			EntryDate:   closeDate,
			Reference:   &ref,
			Description: &desc,
			SourceType:  &sourceType,
			CreatedBy:   closedBy,
			Lines:       revenueLines,
		})
		if err != nil {
			return nil, fmt.Errorf("create revenue closing entry: %w", err)
		}
		// Auto-post
		if err := s.journalSvc.Post(ctx, entry.ID, closedBy); err != nil {
			return nil, fmt.Errorf("post revenue closing entry: %w", err)
		}
		result.JournalEntryIDs = append(result.JournalEntryIDs, entry.ID)
		result.Entries = append(result.Entries, entry)
	}

	// Entry 2: Close COGS + Expenses → Income Summary
	totalExpenses := cogsTotal.Add(expenseTotal)
	if len(expenseLines) > 0 {
		// Expense accounts are credited; Income Summary is debited
		expenseLines = append(expenseLines, CreateJournalLineInput{
			AccountID: incomeSummaryAcct.ID,
			Debit:     totalExpenses,
			Credit:    decimal.Zero,
		})
		ref := fmt.Sprintf("YE-%d Close Expenses", fiscalYear)
		desc := fmt.Sprintf("Year-end close: COGS and expense accounts to Income Summary for FY%d", fiscalYear)
		entry, err := s.journalSvc.Create(ctx, CreateJournalEntryInput{
			CompanyID:   companyID,
			EntryDate:   closeDate,
			Reference:   &ref,
			Description: &desc,
			SourceType:  &sourceType,
			CreatedBy:   closedBy,
			Lines:       expenseLines,
		})
		if err != nil {
			return nil, fmt.Errorf("create expense closing entry: %w", err)
		}
		if err := s.journalSvc.Post(ctx, entry.ID, closedBy); err != nil {
			return nil, fmt.Errorf("post expense closing entry: %w", err)
		}
		result.JournalEntryIDs = append(result.JournalEntryIDs, entry.ID)
		result.Entries = append(result.Entries, entry)
	}

	// Entry 3: Close Income Summary → Retained Earnings
	netIncome := revenueTotal.Sub(totalExpenses)
	if !netIncome.IsZero() {
		var lines []CreateJournalLineInput
		if netIncome.IsPositive() {
			// Net income: debit Income Summary, credit Retained Earnings
			lines = []CreateJournalLineInput{
				{AccountID: incomeSummaryAcct.ID, Debit: netIncome, Credit: decimal.Zero},
				{AccountID: retainedEarningsAcct.ID, Debit: decimal.Zero, Credit: netIncome},
			}
		} else {
			// Net loss: credit Income Summary, debit Retained Earnings
			absLoss := netIncome.Abs()
			lines = []CreateJournalLineInput{
				{AccountID: retainedEarningsAcct.ID, Debit: absLoss, Credit: decimal.Zero},
				{AccountID: incomeSummaryAcct.ID, Debit: decimal.Zero, Credit: absLoss},
			}
		}
		ref := fmt.Sprintf("YE-%d Close Income Summary", fiscalYear)
		desc := fmt.Sprintf("Year-end close: Income Summary to Retained Earnings for FY%d", fiscalYear)
		entry, err := s.journalSvc.Create(ctx, CreateJournalEntryInput{
			CompanyID:   companyID,
			EntryDate:   closeDate,
			Reference:   &ref,
			Description: &desc,
			SourceType:  &sourceType,
			CreatedBy:   closedBy,
			Lines:       lines,
		})
		if err != nil {
			return nil, fmt.Errorf("create income summary closing entry: %w", err)
		}
		if err := s.journalSvc.Post(ctx, entry.ID, closedBy); err != nil {
			return nil, fmt.Errorf("post income summary closing entry: %w", err)
		}
		result.JournalEntryIDs = append(result.JournalEntryIDs, entry.ID)
		result.Entries = append(result.Entries, entry)
	}

	return result, nil
}
