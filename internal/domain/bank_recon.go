package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type BankReconciliationBatch struct {
	ID                uuid.UUID       `json:"id"`
	CompanyID         uuid.UUID       `json:"company_id"`
	CreatedBy         uuid.UUID       `json:"created_by"`
	SessionID         *uuid.UUID      `json:"session_id,omitempty"`
	Status            string          `json:"status"`
	SourceFiles       JSON            `json:"source_files,omitempty"`
	TotalEntries      int             `json:"total_entries"`
	ParseSummary      JSON            `json:"parse_summary,omitempty"`
	MatchResult       JSON            `json:"match_result,omitempty"`
	AISuggestions     JSON            `json:"ai_suggestions,omitempty"`
	AIExplanations    JSON            `json:"ai_explanations,omitempty"`
	AmountTolerance   decimal.Decimal `json:"amount_tolerance"`
	DateToleranceDays int             `json:"date_tolerance_days"`
	Period            string          `json:"period"`
	ErrorMessage      *string         `json:"error_message,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}
