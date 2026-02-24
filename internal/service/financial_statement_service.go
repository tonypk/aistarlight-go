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

// FinancialStatementService generates financial statements from GL data.
type FinancialStatementService struct {
	q *sqlc.Queries
}

// NewFinancialStatementService creates a FinancialStatementService.
func NewFinancialStatementService(q *sqlc.Queries) *FinancialStatementService {
	return &FinancialStatementService{q: q}
}

// BalanceSheet generates a balance sheet as of a given date.
func (s *FinancialStatementService) BalanceSheet(ctx context.Context, companyID uuid.UUID, asOfDate time.Time) (*domain.BalanceSheet, error) {
	rows, err := s.q.AllAccountBalancesAsOf(ctx, sqlc.AllAccountBalancesAsOfParams{
		CompanyID: companyID,
		AsOfDate:  pgtype.Date{Time: asOfDate, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("query account balances: %w", err)
	}

	bs := &domain.BalanceSheet{
		AsOfDate: asOfDate,
	}

	// Also compute income statement totals for retained earnings
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

		ab := domain.AccountBalance{
			AccountID:     row.AccountID,
			AccountCode:   row.AccountNumber,
			AccountName:   row.AccountName,
			Balance:       balance,
			NormalBalance: domain.NormalBalance(row.NormalBalance),
		}

		prefix := row.AccountNumber[:1]
		switch prefix {
		case "1": // Assets
			bs.Assets = append(bs.Assets, ab)
			bs.TotalAssets = bs.TotalAssets.Add(balance)
		case "2": // Liabilities
			bs.Liabilities = append(bs.Liabilities, ab)
			bs.TotalLiabilities = bs.TotalLiabilities.Add(balance)
		case "3": // Equity
			bs.Equity = append(bs.Equity, ab)
			bs.TotalEquity = bs.TotalEquity.Add(balance)
		case "4": // Revenue (for retained earnings calc)
			revenueTotal = revenueTotal.Add(balance)
		case "5": // COGS
			cogsTotal = cogsTotal.Add(balance)
		case "6": // Expenses
			expenseTotal = expenseTotal.Add(balance)
		}
	}

	// Net income = Revenue - COGS - Expenses (added to equity as retained earnings)
	netIncome := revenueTotal.Sub(cogsTotal).Sub(expenseTotal)
	bs.RetainedEarnings = netIncome
	bs.TotalEquity = bs.TotalEquity.Add(netIncome)
	bs.IsBalanced = bs.TotalAssets.Equal(bs.TotalLiabilities.Add(bs.TotalEquity))

	return bs, nil
}

// IncomeStatement generates an income statement for a date range.
func (s *FinancialStatementService) IncomeStatement(ctx context.Context, companyID uuid.UUID, fromDate, toDate time.Time) (*domain.IncomeStatement, error) {
	rows, err := s.q.PeriodAllAccountBalances(ctx, sqlc.PeriodAllAccountBalancesParams{
		CompanyID: companyID,
		FromDate:  pgtype.Date{Time: fromDate, Valid: true},
		ToDate:    pgtype.Date{Time: toDate, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("query period balances: %w", err)
	}

	is := &domain.IncomeStatement{
		PeriodStart: fromDate,
		PeriodEnd:   toDate,
	}

	for _, row := range rows {
		balance := computeNormalBalance(
			numericToDecimal(row.TotalDebit),
			numericToDecimal(row.TotalCredit),
			row.NormalBalance,
		)

		if balance.IsZero() {
			continue
		}

		ab := domain.AccountBalance{
			AccountID:     row.AccountID,
			AccountCode:   row.AccountNumber,
			AccountName:   row.AccountName,
			Balance:       balance,
			NormalBalance: domain.NormalBalance(row.NormalBalance),
		}

		prefix := row.AccountNumber[:1]
		switch prefix {
		case "4": // Revenue
			is.Revenue = append(is.Revenue, ab)
			is.TotalRevenue = is.TotalRevenue.Add(balance)
		case "5": // COGS
			is.COGS = append(is.COGS, ab)
			is.TotalCOGS = is.TotalCOGS.Add(balance)
		case "6": // Expenses
			is.Expenses = append(is.Expenses, ab)
			is.TotalExpenses = is.TotalExpenses.Add(balance)
		}
	}

	is.GrossProfit = is.TotalRevenue.Sub(is.TotalCOGS)
	is.NetIncome = is.GrossProfit.Sub(is.TotalExpenses)

	return is, nil
}

// AccountBalancesByPrefix returns account balances filtered by account number prefix.
func (s *FinancialStatementService) AccountBalancesByPrefix(ctx context.Context, companyID uuid.UUID, prefix string, asOfDate time.Time) ([]domain.AccountBalance, decimal.Decimal, error) {
	rows, err := s.q.AccountBalancesByPrefix(ctx, sqlc.AccountBalancesByPrefixParams{
		CompanyID: companyID,
		AsOfDate:  pgtype.Date{Time: asOfDate, Valid: true},
		Prefix:    &prefix,
	})
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("query balances by prefix %s: %w", prefix, err)
	}

	var balances []domain.AccountBalance
	total := decimal.Zero

	for _, row := range rows {
		balance := computeNormalBalance(
			numericToDecimal(row.TotalDebit),
			numericToDecimal(row.TotalCredit),
			row.NormalBalance,
		)
		if balance.IsZero() {
			continue
		}

		balances = append(balances, domain.AccountBalance{
			AccountID:     row.AccountID,
			AccountCode:   row.AccountNumber,
			AccountName:   row.AccountName,
			Balance:       balance,
			NormalBalance: domain.NormalBalance(row.NormalBalance),
		})
		total = total.Add(balance)
	}

	return balances, total, nil
}

// PeriodBalancesByPrefix returns account balances for a period, filtered by prefix.
func (s *FinancialStatementService) PeriodBalancesByPrefix(ctx context.Context, companyID uuid.UUID, prefix string, fromDate, toDate time.Time) ([]domain.AccountBalance, decimal.Decimal, error) {
	rows, err := s.q.PeriodAccountBalances(ctx, sqlc.PeriodAccountBalancesParams{
		CompanyID: companyID,
		FromDate:  pgtype.Date{Time: fromDate, Valid: true},
		ToDate:    pgtype.Date{Time: toDate, Valid: true},
		Prefix:    &prefix,
	})
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("query period balances by prefix %s: %w", prefix, err)
	}

	var balances []domain.AccountBalance
	total := decimal.Zero

	for _, row := range rows {
		balance := computeNormalBalance(
			numericToDecimal(row.TotalDebit),
			numericToDecimal(row.TotalCredit),
			row.NormalBalance,
		)
		if balance.IsZero() {
			continue
		}

		balances = append(balances, domain.AccountBalance{
			AccountID:     row.AccountID,
			AccountCode:   row.AccountNumber,
			AccountName:   row.AccountName,
			Balance:       balance,
			NormalBalance: domain.NormalBalance(row.NormalBalance),
		})
		total = total.Add(balance)
	}

	return balances, total, nil
}

// computeNormalBalance computes the net balance based on normal balance direction.
// For debit-normal accounts: balance = debit - credit (positive = normal)
// For credit-normal accounts: balance = credit - debit (positive = normal)
func computeNormalBalance(debit, credit decimal.Decimal, normalBalance string) decimal.Decimal {
	if strings.ToLower(normalBalance) == "debit" {
		return debit.Sub(credit)
	}
	return credit.Sub(debit)
}
