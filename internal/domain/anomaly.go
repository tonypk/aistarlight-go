package domain

import (
	"time"

	"github.com/google/uuid"
)

type Anomaly struct {
	ID             uuid.UUID  `json:"id"`
	CompanyID      uuid.UUID  `json:"company_id"`
	SessionID      uuid.UUID  `json:"session_id"`
	TransactionID  *uuid.UUID `json:"transaction_id,omitempty"`
	AnomalyType    string     `json:"anomaly_type"`
	Severity       string     `json:"severity"`
	Description    string     `json:"description"`
	Details        JSON       `json:"details,omitempty"`
	Status         string     `json:"status"`
	ResolvedBy     *uuid.UUID `json:"resolved_by,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	ResolutionNote *string    `json:"resolution_note,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
