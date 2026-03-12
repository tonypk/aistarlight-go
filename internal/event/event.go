package event

import (
	"time"

	"github.com/google/uuid"
)

// Event type constants use "event:" prefix to distinguish from regular asynq tasks.
const (
	TypeJournalPosted           = "event:journal_posted"
	TypeReconciliationCompleted = "event:reconciliation_completed"
)

// JournalPostedPayload is fired when a journal entry transitions to "posted".
type JournalPostedPayload struct {
	JournalEntryID uuid.UUID `json:"journal_entry_id"`
	CompanyID      uuid.UUID `json:"company_id"`
	PostedBy       uuid.UUID `json:"posted_by"`
	PostedAt       time.Time `json:"posted_at"`
}

// ReconciliationCompletedPayload is fired when a bank reconciliation batch finishes.
type ReconciliationCompletedPayload struct {
	BatchID     uuid.UUID `json:"batch_id"`
	CompanyID   uuid.UUID `json:"company_id"`
	MatchCount  int       `json:"match_count"`
	Period      string    `json:"period"`
	CompletedAt time.Time `json:"completed_at"`
}
