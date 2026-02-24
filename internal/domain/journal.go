package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type JournalStatus string

const (
	JournalStatusDraft    JournalStatus = "draft"
	JournalStatusPosted   JournalStatus = "posted"
	JournalStatusReversed JournalStatus = "reversed"
)

type JournalSourceType string

const (
	SourceManual         JournalSourceType = "manual"
	SourceReceipt        JournalSourceType = "receipt"
	SourceQBOSync        JournalSourceType = "qbo_sync"
	SourceReconciliation JournalSourceType = "reconciliation"
)

type JournalEntry struct {
	ID           uuid.UUID     `json:"id"`
	CompanyID    uuid.UUID     `json:"company_id"`
	PeriodID     *uuid.UUID    `json:"period_id,omitempty"`
	EntryNumber  int           `json:"entry_number"`
	EntryDate    time.Time     `json:"entry_date"`
	Reference    *string       `json:"reference,omitempty"`
	Description  *string       `json:"description,omitempty"`
	SourceType   *string       `json:"source_type,omitempty"`
	SourceID     *uuid.UUID    `json:"source_id,omitempty"`
	Status       JournalStatus `json:"status"`
	PostedBy     *uuid.UUID    `json:"posted_by,omitempty"`
	PostedAt     *time.Time    `json:"posted_at,omitempty"`
	ReversedByID *uuid.UUID    `json:"reversed_by_id,omitempty"`
	ReversesID   *uuid.UUID    `json:"reverses_id,omitempty"`
	Memo         *string       `json:"memo,omitempty"`
	CreatedBy    *uuid.UUID    `json:"created_by,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	Lines        []JournalLine `json:"lines,omitempty"`
}

type JournalLine struct {
	ID             uuid.UUID       `json:"id"`
	JournalEntryID uuid.UUID       `json:"journal_entry_id"`
	AccountID      uuid.UUID       `json:"account_id"`
	LineNumber     int             `json:"line_number"`
	Description    *string         `json:"description,omitempty"`
	Debit          decimal.Decimal `json:"debit"`
	Credit         decimal.Decimal `json:"credit"`
	AccountName    string          `json:"account_name,omitempty"`
	AccountNumber  string          `json:"account_number,omitempty"`
}

// TrialBalanceRow represents a single row in the trial balance report.
type TrialBalanceRow struct {
	AccountID     uuid.UUID       `json:"account_id"`
	AccountNumber string          `json:"account_number"`
	AccountName   string          `json:"account_name"`
	AccountType   AccountType     `json:"account_type"`
	DebitBalance  decimal.Decimal `json:"debit_balance"`
	CreditBalance decimal.Decimal `json:"credit_balance"`
}

// AccountLedgerRow represents a single entry in an account's ledger.
type AccountLedgerRow struct {
	JournalEntryID uuid.UUID       `json:"journal_entry_id"`
	EntryNumber    int             `json:"entry_number"`
	EntryDate      time.Time       `json:"entry_date"`
	Reference      *string         `json:"reference,omitempty"`
	Description    *string         `json:"description,omitempty"`
	Debit          decimal.Decimal `json:"debit"`
	Credit         decimal.Decimal `json:"credit"`
	RunningBalance decimal.Decimal `json:"running_balance"`
}
