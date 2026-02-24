package domain

import (
	"time"

	"github.com/google/uuid"
)

type AccountType string

const (
	AccountTypeAsset     AccountType = "asset"
	AccountTypeLiability AccountType = "liability"
	AccountTypeEquity    AccountType = "equity"
	AccountTypeRevenue   AccountType = "revenue"
	AccountTypeExpense   AccountType = "expense"
)

type NormalBalance string

const (
	NormalDebit  NormalBalance = "debit"
	NormalCredit NormalBalance = "credit"
)

type Account struct {
	ID            uuid.UUID     `json:"id"`
	CompanyID     uuid.UUID     `json:"company_id"`
	AccountNumber string        `json:"account_number"`
	Name          string        `json:"name"`
	AccountType   AccountType   `json:"account_type"`
	SubType       *string       `json:"sub_type,omitempty"`
	ParentID      *uuid.UUID    `json:"parent_id,omitempty"`
	Description   *string       `json:"description,omitempty"`
	IsActive      bool          `json:"is_active"`
	IsSystem      bool          `json:"is_system"`
	NormalBalance NormalBalance `json:"normal_balance"`
	QBOAccountID  *string       `json:"qbo_account_id,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// NormalBalanceFor returns the standard normal balance for a given account type.
func NormalBalanceFor(at AccountType) NormalBalance {
	switch at {
	case AccountTypeAsset, AccountTypeExpense:
		return NormalDebit
	default:
		return NormalCredit
	}
}
