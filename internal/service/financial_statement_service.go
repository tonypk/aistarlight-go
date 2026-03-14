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

// subTypeToGroup maps sub_type values to display group names for balance sheet.
func subTypeToGroup(accountType, subType string) string {
	switch accountType {
	case "asset":
		switch subType {
		case "cash", "receivable", "inventory", "prepaid", "tax_credit":
			return "Current Assets"
		case "fixed", "intangible":
			return "Non-Current Assets"
		default:
			return "Other Assets"
		}
	case "liability":
		switch subType {
		case "payable", "notes", "tax_payable", "statutory", "deferred":
			return "Current Liabilities"
		case "long_term":
			return "Long-term Liabilities"
		default:
			return "Other Liabilities"
		}
	case "equity":
		return "Owner's Equity"
	default:
		return "Other"
	}
}

// BalanceSheet generates a balance sheet as of a given date, grouped by sub_type.
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

	// Group maps for building AccountGroup slices
	assetGroups := make(map[string]*domain.AccountGroup)
	liabGroups := make(map[string]*domain.AccountGroup)
	equityGroups := make(map[string]*domain.AccountGroup)

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

		subType := ""
		if row.SubType != nil {
			subType = *row.SubType
		}

		ab := domain.AccountBalance{
			AccountID:     row.AccountID,
			AccountCode:   row.AccountNumber,
			AccountName:   row.AccountName,
			SubType:       subType,
			Balance:       balance,
			NormalBalance: domain.NormalBalance(row.NormalBalance),
		}

		prefix := row.AccountNumber[:1]
		switch prefix {
		case "1": // Assets
			groupName := subTypeToGroup("asset", subType)
			grp, ok := assetGroups[groupName]
			if !ok {
				grp = &domain.AccountGroup{GroupName: groupName}
				assetGroups[groupName] = grp
			}
			grp.Accounts = append(grp.Accounts, ab)
			grp.Total = grp.Total.Add(balance)
			bs.TotalAssets = bs.TotalAssets.Add(balance)
		case "2": // Liabilities
			groupName := subTypeToGroup("liability", subType)
			grp, ok := liabGroups[groupName]
			if !ok {
				grp = &domain.AccountGroup{GroupName: groupName}
				liabGroups[groupName] = grp
			}
			grp.Accounts = append(grp.Accounts, ab)
			grp.Total = grp.Total.Add(balance)
			bs.TotalLiabilities = bs.TotalLiabilities.Add(balance)
		case "3": // Equity
			groupName := subTypeToGroup("equity", subType)
			grp, ok := equityGroups[groupName]
			if !ok {
				grp = &domain.AccountGroup{GroupName: groupName}
				equityGroups[groupName] = grp
			}
			grp.Accounts = append(grp.Accounts, ab)
			grp.Total = grp.Total.Add(balance)
			bs.TotalEquity = bs.TotalEquity.Add(balance)
		case "4": // Revenue (for retained earnings calc)
			revenueTotal = revenueTotal.Add(balance)
		case "5": // COGS
			cogsTotal = cogsTotal.Add(balance)
		case "6": // Expenses
			expenseTotal = expenseTotal.Add(balance)
		}
	}

	// Convert group maps to ordered slices
	bs.Assets = groupMapToSlice(assetGroups, []string{"Current Assets", "Non-Current Assets", "Other Assets"})
	bs.Liabilities = groupMapToSlice(liabGroups, []string{"Current Liabilities", "Long-term Liabilities", "Other Liabilities"})
	bs.Equity = groupMapToSlice(equityGroups, []string{"Owner's Equity"})

	// Net income = Revenue - COGS - Expenses (added to equity as retained earnings)
	netIncome := revenueTotal.Sub(cogsTotal).Sub(expenseTotal)
	bs.RetainedEarnings = netIncome
	bs.TotalEquity = bs.TotalEquity.Add(netIncome)
	bs.IsBalanced = bs.TotalAssets.Equal(bs.TotalLiabilities.Add(bs.TotalEquity))

	return bs, nil
}

// groupMapToSlice converts a group map to an ordered slice based on a preference order.
func groupMapToSlice(groups map[string]*domain.AccountGroup, order []string) []domain.AccountGroup {
	var result []domain.AccountGroup
	seen := make(map[string]bool)
	for _, name := range order {
		if grp, ok := groups[name]; ok {
			result = append(result, *grp)
			seen[name] = true
		}
	}
	// Append any remaining groups not in the preferred order
	for name, grp := range groups {
		if !seen[name] {
			result = append(result, *grp)
		}
	}
	return result
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

		subType := ""
		if row.SubType != nil {
			subType = *row.SubType
		}

		ab := domain.AccountBalance{
			AccountID:     row.AccountID,
			AccountCode:   row.AccountNumber,
			AccountName:   row.AccountName,
			SubType:       subType,
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

// CashFlowStatement generates a cash flow statement (indirect method) for a date range.
func (s *FinancialStatementService) CashFlowStatement(ctx context.Context, companyID uuid.UUID, fromDate, toDate time.Time) (*domain.CashFlowStatement, error) {
	// Get income statement for net income
	is, err := s.IncomeStatement(ctx, companyID, fromDate, toDate)
	if err != nil {
		return nil, fmt.Errorf("income statement for cash flow: %w", err)
	}

	// Get all period balances to compute changes
	rows, err := s.q.PeriodAllAccountBalances(ctx, sqlc.PeriodAllAccountBalancesParams{
		CompanyID: companyID,
		FromDate:  pgtype.Date{Time: fromDate, Valid: true},
		ToDate:    pgtype.Date{Time: toDate, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("query period balances for cash flow: %w", err)
	}

	cf := &domain.CashFlowStatement{
		PeriodStart: fromDate,
		PeriodEnd:   toDate,
		NetIncome:   is.NetIncome,
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

		subType := ""
		if row.SubType != nil {
			subType = *row.SubType
		}

		category := ""
		if row.CashFlowCategory != nil {
			category = *row.CashFlowCategory
		}

		// Determine category based on cash_flow_category field or sub_type heuristics
		if category == "" {
			category = inferCashFlowCategory(row.AccountType, subType)
		}

		ab := domain.AccountBalance{
			AccountID:   row.AccountID,
			AccountCode: row.AccountNumber,
			AccountName: row.AccountName,
			SubType:     subType,
			Balance:     balance,
		}

		switch category {
		case "operating":
			// Non-cash items like depreciation/amortization
			if subType == "depreciation" || subType == "amortization" {
				cf.DepreciationAmort = cf.DepreciationAmort.Add(balance)
			} else {
				cf.WorkingCapitalChanges = append(cf.WorkingCapitalChanges, ab)
			}
		case "investing":
			cf.InvestingItems = append(cf.InvestingItems, ab)
			cf.InvestingTotal = cf.InvestingTotal.Add(balance)
		case "financing":
			cf.FinancingItems = append(cf.FinancingItems, ab)
			cf.FinancingTotal = cf.FinancingTotal.Add(balance)
		}
	}

	// Operating total = net income + depreciation + working capital changes
	wcTotal := decimal.Zero
	for _, wc := range cf.WorkingCapitalChanges {
		wcTotal = wcTotal.Add(wc.Balance)
	}
	cf.OperatingTotal = cf.NetIncome.Add(cf.DepreciationAmort).Add(wcTotal)

	cf.NetChange = cf.OperatingTotal.Add(cf.InvestingTotal).Add(cf.FinancingTotal)

	// Get beginning and ending cash balances
	beginCash, _ := s.getCashBalance(ctx, companyID, fromDate.AddDate(0, 0, -1))
	endCash, _ := s.getCashBalance(ctx, companyID, toDate)
	cf.BeginningCash = beginCash
	cf.EndingCash = endCash

	return cf, nil
}

func (s *FinancialStatementService) getCashBalance(ctx context.Context, companyID uuid.UUID, asOf time.Time) (decimal.Decimal, error) {
	prefix := "1"
	rows, err := s.q.AccountBalancesByPrefix(ctx, sqlc.AccountBalancesByPrefixParams{
		CompanyID: companyID,
		AsOfDate:  pgtype.Date{Time: asOf, Valid: true},
		Prefix:    &prefix,
	})
	if err != nil {
		return decimal.Zero, err
	}

	total := decimal.Zero
	for _, row := range rows {
		subType := ""
		if row.SubType != nil {
			subType = *row.SubType
		}
		if subType == "cash" {
			balance := computeNormalBalance(
				numericToDecimal(row.TotalDebit),
				numericToDecimal(row.TotalCredit),
				row.NormalBalance,
			)
			total = total.Add(balance)
		}
	}
	return total, nil
}

func inferCashFlowCategory(accountType, subType string) string {
	switch accountType {
	case "asset":
		switch subType {
		case "cash":
			return "" // Cash itself is not categorized
		case "receivable", "inventory", "prepaid", "tax_credit":
			return "operating"
		case "fixed", "intangible":
			return "investing"
		}
	case "liability":
		switch subType {
		case "payable", "tax_payable", "statutory", "deferred":
			return "operating"
		case "notes", "long_term":
			return "financing"
		}
	case "equity":
		return "financing"
	case "expense":
		if subType == "depreciation" || subType == "amortization" {
			return "operating"
		}
	}
	return ""
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

// ComparativeBalanceSheet generates a balance sheet with prior period comparison.
func (s *FinancialStatementService) ComparativeBalanceSheet(ctx context.Context, companyID uuid.UUID, asOfDate, priorAsOfDate time.Time) (*domain.BalanceSheet, error) {
	// Generate current period balance sheet
	bs, err := s.BalanceSheet(ctx, companyID, asOfDate)
	if err != nil {
		return nil, fmt.Errorf("current balance sheet: %w", err)
	}

	// Generate prior period balance sheet
	prior, err := s.BalanceSheet(ctx, companyID, priorAsOfDate)
	if err != nil {
		return nil, fmt.Errorf("prior balance sheet: %w", err)
	}

	// Build prior balance lookup: account_id → balance
	priorMap := buildBalanceMap(prior.Assets, prior.Liabilities, prior.Equity)

	// Merge prior balances into current groups
	mergeGroupsPrior(bs.Assets, priorMap)
	mergeGroupsPrior(bs.Liabilities, priorMap)
	mergeGroupsPrior(bs.Equity, priorMap)

	// Set prior totals
	bs.PriorAsOfDate = &priorAsOfDate
	bs.PriorTotalAssets = &prior.TotalAssets
	bs.PriorTotalLiab = &prior.TotalLiabilities
	bs.PriorTotalEquity = &prior.TotalEquity
	bs.PriorRetainedEarn = &prior.RetainedEarnings

	return bs, nil
}

// ComparativeIncomeStatement generates an income statement with prior period comparison.
func (s *FinancialStatementService) ComparativeIncomeStatement(ctx context.Context, companyID uuid.UUID, fromDate, toDate, priorFrom, priorTo time.Time) (*domain.IncomeStatement, error) {
	// Generate current period
	is, err := s.IncomeStatement(ctx, companyID, fromDate, toDate)
	if err != nil {
		return nil, fmt.Errorf("current income statement: %w", err)
	}

	// Generate prior period
	prior, err := s.IncomeStatement(ctx, companyID, priorFrom, priorTo)
	if err != nil {
		return nil, fmt.Errorf("prior income statement: %w", err)
	}

	// Build prior balance lookup
	priorMap := make(map[uuid.UUID]decimal.Decimal)
	for _, ab := range prior.Revenue {
		priorMap[ab.AccountID] = ab.Balance
	}
	for _, ab := range prior.COGS {
		priorMap[ab.AccountID] = ab.Balance
	}
	for _, ab := range prior.Expenses {
		priorMap[ab.AccountID] = ab.Balance
	}

	// Merge prior balances into current line items
	mergeAccountsPrior(is.Revenue, priorMap)
	mergeAccountsPrior(is.COGS, priorMap)
	mergeAccountsPrior(is.Expenses, priorMap)

	// Set prior totals
	is.PriorPeriodStart = &priorFrom
	is.PriorPeriodEnd = &priorTo
	is.PriorRevenue = &prior.TotalRevenue
	is.PriorCOGS = &prior.TotalCOGS
	is.PriorGrossProfit = &prior.GrossProfit
	is.PriorExpenses = &prior.TotalExpenses
	is.PriorNetIncome = &prior.NetIncome

	return is, nil
}

// buildBalanceMap extracts account_id → balance from grouped sections.
func buildBalanceMap(sections ...[]domain.AccountGroup) map[uuid.UUID]decimal.Decimal {
	m := make(map[uuid.UUID]decimal.Decimal)
	for _, groups := range sections {
		for _, grp := range groups {
			for _, ab := range grp.Accounts {
				m[ab.AccountID] = ab.Balance
			}
		}
	}
	return m
}

// mergeGroupsPrior adds prior balance data into account groups.
func mergeGroupsPrior(groups []domain.AccountGroup, priorMap map[uuid.UUID]decimal.Decimal) {
	for i := range groups {
		priorTotal := decimal.Zero
		for j := range groups[i].Accounts {
			if pb, ok := priorMap[groups[i].Accounts[j].AccountID]; ok {
				groups[i].Accounts[j].PriorBalance = &pb
				change := groups[i].Accounts[j].Balance.Sub(pb)
				groups[i].Accounts[j].Change = &change
				priorTotal = priorTotal.Add(pb)
			} else {
				zero := decimal.Zero
				groups[i].Accounts[j].PriorBalance = &zero
				change := groups[i].Accounts[j].Balance
				groups[i].Accounts[j].Change = &change
			}
		}
		groups[i].PriorTotal = &priorTotal
	}
}

// mergeAccountsPrior adds prior balance data into a flat account list.
func mergeAccountsPrior(accounts []domain.AccountBalance, priorMap map[uuid.UUID]decimal.Decimal) {
	for i := range accounts {
		if pb, ok := priorMap[accounts[i].AccountID]; ok {
			accounts[i].PriorBalance = &pb
			change := accounts[i].Balance.Sub(pb)
			accounts[i].Change = &change
		} else {
			zero := decimal.Zero
			accounts[i].PriorBalance = &zero
			change := accounts[i].Balance
			accounts[i].Change = &change
		}
	}
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
