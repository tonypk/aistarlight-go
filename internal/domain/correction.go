package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Correction struct {
	ID          uuid.UUID `json:"id"`
	CompanyID   uuid.UUID `json:"company_id"`
	UserID      uuid.UUID `json:"user_id"`
	EntityType  string    `json:"entity_type"`
	EntityID    uuid.UUID `json:"entity_id"`
	FieldName   string    `json:"field_name"`
	OldValue    *string   `json:"old_value,omitempty"`
	NewValue    string    `json:"new_value"`
	Reason      *string   `json:"reason,omitempty"`
	ContextData JSON      `json:"context_data,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type CorrectionRule struct {
	ID                    uuid.UUID       `json:"id"`
	CompanyID             uuid.UUID       `json:"company_id"`
	RuleType              string          `json:"rule_type"`
	MatchCriteria         JSON            `json:"match_criteria"`
	CorrectionField       string          `json:"correction_field"`
	CorrectionValue       string          `json:"correction_value"`
	Confidence            decimal.Decimal `json:"confidence"`
	SourceCorrectionCount int             `json:"source_correction_count"`
	IsActive              bool            `json:"is_active"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

type ValidationResult struct {
	ID           uuid.UUID `json:"id"`
	ReportID     uuid.UUID `json:"report_id"`
	CompanyID    uuid.UUID `json:"company_id"`
	OverallScore int       `json:"overall_score"`
	CheckResults JSON      `json:"check_results"`
	RAGFindings  JSON      `json:"rag_findings,omitempty"`
	ValidatedAt  time.Time `json:"validated_at"`
}
