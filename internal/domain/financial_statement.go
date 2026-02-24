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
	Balance       decimal.Decimal `json:"balance"`
	NormalBalance NormalBalance   `json:"normal_balance"`
}

// BalanceSheet represents a point-in-time balance sheet.
type BalanceSheet struct {
	AsOfDate         time.Time        `json:"as_of_date"`
	Assets           []AccountBalance `json:"assets"`
	Liabilities      []AccountBalance `json:"liabilities"`
	Equity           []AccountBalance `json:"equity"`
	TotalAssets      decimal.Decimal  `json:"total_assets"`
	TotalLiabilities decimal.Decimal  `json:"total_liabilities"`
	TotalEquity      decimal.Decimal  `json:"total_equity"`
	RetainedEarnings decimal.Decimal  `json:"retained_earnings"`
	IsBalanced       bool             `json:"is_balanced"`
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
}
