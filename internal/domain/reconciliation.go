package domain

import (
	"time"

	"github.com/google/uuid"
)

type ReconciliationSession struct {
	ID                   uuid.UUID  `json:"id"`
	CompanyID            uuid.UUID  `json:"company_id"`
	CreatedBy            uuid.UUID  `json:"created_by"`
	Period               string     `json:"period"`
	Status               string     `json:"status"`
	ReportID             *uuid.UUID `json:"report_id,omitempty"`
	SourceFiles          JSON       `json:"source_files,omitempty"`
	Summary              JSON       `json:"summary,omitempty"`
	ReconciliationResult JSON       `json:"reconciliation_result,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}
