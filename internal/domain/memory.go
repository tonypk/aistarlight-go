package domain

import (
	"time"

	"github.com/google/uuid"
)

type UserPreference struct {
	ID             uuid.UUID `json:"id"`
	CompanyID      uuid.UUID `json:"company_id"`
	ReportType     string    `json:"report_type"`
	ColumnMappings JSON      `json:"column_mappings"`
	FormatRules    JSON      `json:"format_rules"`
	AutoFillRules  JSON      `json:"auto_fill_rules"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CorrectionHistory struct {
	ID         uuid.UUID `json:"id"`
	CompanyID  uuid.UUID `json:"company_id"`
	ReportType string    `json:"report_type"`
	FieldName  *string   `json:"field_name,omitempty"`
	OldValue   *string   `json:"old_value,omitempty"`
	NewValue   *string   `json:"new_value,omitempty"`
	Reason     *string   `json:"reason,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
