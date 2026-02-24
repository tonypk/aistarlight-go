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

// GLService provides general ledger query operations.
type GLService struct {
	q *sqlc.Queries
}

func NewGLService(q *sqlc.Queries) *GLService {
	return &GLService{q: q}
}

// TrialBalanceResult holds the full trial balance report.
type TrialBalanceResult struct {
	Rows         []domain.TrialBalanceRow `json:"rows"`
	TotalDebits  decimal.Decimal          `json:"total_debits"`
	TotalCredits decimal.Decimal          `json:"total_credits"`
	IsBalanced   bool                     `json:"is_balanced"`
	StartDate    time.Time                `json:"start_date"`
	EndDate      time.Time                `json:"end_date"`
}

// TrialBalance generates a trial balance for the given date range.
func (s *GLService) TrialBalance(ctx context.Context, companyID uuid.UUID, startDate, endDate time.Time) (*TrialBalanceResult, error) {
	rows, err := s.q.TrialBalance(ctx, sqlc.TrialBalanceParams{
		CompanyID:   companyID,
		EntryDate:   pgtype.Date{Time: startDate, Valid: true},
		EntryDate_2: pgtype.Date{Time: endDate, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("trial balance query: %w", err)
	}

	totalDebits := decimal.Zero
	totalCredits := decimal.Zero

	result := make([]domain.TrialBalanceRow, len(rows))
	for i, r := range rows {
		debit := numericToDecimal(r.DebitBalance)
		credit := numericToDecimal(r.CreditBalance)

		result[i] = domain.TrialBalanceRow{
			AccountID:     r.AccountID,
			AccountNumber: r.AccountNumber,
			AccountName:   r.AccountName,
			AccountType:   domain.AccountType(r.AccountType),
			DebitBalance:  debit,
			CreditBalance: credit,
		}

		totalDebits = totalDebits.Add(debit)
		totalCredits = totalCredits.Add(credit)
	}

	return &TrialBalanceResult{
		Rows:         result,
		TotalDebits:  totalDebits,
		TotalCredits: totalCredits,
		IsBalanced:   totalDebits.Equal(totalCredits),
		StartDate:    startDate,
		EndDate:      endDate,
	}, nil
}

// AccountLedger returns the detailed ledger for a specific account with running balance.
func (s *GLService) AccountLedger(ctx context.Context, companyID, accountID uuid.UUID, startDate, endDate time.Time) ([]domain.AccountLedgerRow, error) {
	rows, err := s.q.AccountLedger(ctx, sqlc.AccountLedgerParams{
		AccountID:   accountID,
		CompanyID:   companyID,
		EntryDate:   pgtype.Date{Time: startDate, Valid: true},
		EntryDate_2: pgtype.Date{Time: endDate, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("account ledger query: %w", err)
	}

	result := make([]domain.AccountLedgerRow, len(rows))
	running := decimal.Zero

	for i, r := range rows {
		debit := numericToDecimal(r.Debit)
		credit := numericToDecimal(r.Credit)
		running = running.Add(debit).Sub(credit)

		var entryDate time.Time
		if r.EntryDate.Valid {
			entryDate = r.EntryDate.Time
		}
		var entryNum int
		if r.EntryNumber != nil {
			entryNum = int(*r.EntryNumber)
		}

		result[i] = domain.AccountLedgerRow{
			JournalEntryID: r.JournalEntryID,
			EntryNumber:    entryNum,
			EntryDate:      entryDate,
			Reference:      r.Reference,
			Description:    r.Description,
			Debit:          debit,
			Credit:         credit,
			RunningBalance: running,
		}
	}

	return result, nil
}
