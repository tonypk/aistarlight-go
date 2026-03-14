package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// AccountBalance represents a single account's balance in a financial statement.
type AccountBalance struct {
	AccountID     uuid.UUID       `json:"account_id"`
	AccountCode   string          `json:"account_code"`
	AccountName   string          `json:"account_name"`
	SubType       string          `json:"sub_type,omitempty"`
	Balance       decimal.Decimal `json:"balance"`
	NormalBalance NormalBalance   `json:"normal_balance"`
	// Comparative period fields (omitted when no comparison requested)
	PriorBalance *decimal.Decimal `json:"prior_balance,omitempty"`
	Change       *decimal.Decimal `json:"change,omitempty"`
}

// AccountGroup represents a group of accounts in a financial statement section.
type AccountGroup struct {
	GroupName  string           `json:"group_name"`
	Accounts   []AccountBalance `json:"accounts"`
	Total      decimal.Decimal  `json:"total"`
	PriorTotal *decimal.Decimal `json:"prior_total,omitempty"`
}

// BalanceSheet represents a point-in-time balance sheet.
type BalanceSheet struct {
	AsOfDate         time.Time      `json:"as_of_date"`
	Assets           []AccountGroup `json:"assets"`
	Liabilities      []AccountGroup `json:"liabilities"`
	Equity           []AccountGroup `json:"equity"`
	TotalAssets      decimal.Decimal `json:"total_assets"`
	TotalLiabilities decimal.Decimal `json:"total_liabilities"`
	TotalEquity      decimal.Decimal `json:"total_equity"`
	RetainedEarnings decimal.Decimal `json:"retained_earnings"`
	IsBalanced       bool            `json:"is_balanced"`
	// Comparative fields (omitted when no comparison requested)
	PriorAsOfDate      *time.Time       `json:"prior_as_of_date,omitempty"`
	PriorTotalAssets   *decimal.Decimal  `json:"prior_total_assets,omitempty"`
	PriorTotalLiab     *decimal.Decimal  `json:"prior_total_liabilities,omitempty"`
	PriorTotalEquity   *decimal.Decimal  `json:"prior_total_equity,omitempty"`
	PriorRetainedEarn  *decimal.Decimal  `json:"prior_retained_earnings,omitempty"`
}

// IncomeStatement represents a period income statement.
type IncomeStatement struct {
	PeriodStart   time.Time        `json:"period_start"`
	PeriodEnd     time.Time        `json:"period_end"`
	Revenue       []AccountBalance `json:"revenue"`
	COGS          []AccountBalance `json:"cogs"`
	Expenses      []AccountBalance `json:"expenses"`
	TotalRevenue  decimal.Decimal  `json:"total_revenue"`
	TotalCOGS     decimal.Decimal  `json:"total_cogs"`
	GrossProfit   decimal.Decimal  `json:"gross_profit"`
	TotalExpenses decimal.Decimal  `json:"total_expenses"`
	NetIncome     decimal.Decimal  `json:"net_income"`
	// Comparative fields (omitted when no comparison requested)
	PriorPeriodStart *time.Time       `json:"prior_period_start,omitempty"`
	PriorPeriodEnd   *time.Time       `json:"prior_period_end,omitempty"`
	PriorRevenue     *decimal.Decimal `json:"prior_total_revenue,omitempty"`
	PriorCOGS        *decimal.Decimal `json:"prior_total_cogs,omitempty"`
	PriorGrossProfit *decimal.Decimal `json:"prior_gross_profit,omitempty"`
	PriorExpenses    *decimal.Decimal `json:"prior_total_expenses,omitempty"`
	PriorNetIncome   *decimal.Decimal `json:"prior_net_income,omitempty"`
}

// CashFlowStatement represents a cash flow statement (indirect method).
type CashFlowStatement struct {
	PeriodStart time.Time       `json:"period_start"`
	PeriodEnd   time.Time       `json:"period_end"`

	// Operating Activities
	NetIncome             decimal.Decimal  `json:"net_income"`
	DepreciationAmort     decimal.Decimal  `json:"depreciation_amortization"`
	WorkingCapitalChanges []AccountBalance `json:"working_capital_changes"`
	OperatingTotal        decimal.Decimal  `json:"operating_total"`

	// Investing Activities
	InvestingItems []AccountBalance `json:"investing_items"`
	InvestingTotal decimal.Decimal  `json:"investing_total"`

	// Financing Activities
	FinancingItems []AccountBalance `json:"financing_items"`
	FinancingTotal decimal.Decimal  `json:"financing_total"`

	// Summary
	NetChange      decimal.Decimal `json:"net_change"`
	BeginningCash  decimal.Decimal `json:"beginning_cash"`
	EndingCash     decimal.Decimal `json:"ending_cash"`
}
