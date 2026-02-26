package domain

import (
	"time"

	"github.com/google/uuid"
)

type Report struct {
	ID                     uuid.UUID  `json:"id"`
	CompanyID              uuid.UUID  `json:"company_id"`
	ReportType             string     `json:"report_type"`
	Period                 string     `json:"period"`
	Status                 string     `json:"status"`
	InputData              JSON       `json:"input_data,omitempty"`
	CalculatedData         JSON       `json:"calculated_data,omitempty"`
	FilePath               *string    `json:"file_path,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	ConfirmedAt            *time.Time `json:"confirmed_at,omitempty"`
	CreatedBy              *uuid.UUID `json:"created_by,omitempty"`
	UpdatedBy              *uuid.UUID `json:"updated_by,omitempty"`
	UpdatedAt              *time.Time `json:"updated_at,omitempty"`
	Version                int        `json:"version"`
	Overrides              JSON       `json:"overrides,omitempty"`
	OriginalCalculatedData JSON       `json:"original_calculated_data,omitempty"`
	Notes                  *string    `json:"notes,omitempty"`
	ComplianceScore        *int       `json:"compliance_score,omitempty"`
	AmendmentNumber        int        `json:"amendment_number"`
	OriginalReportID       *uuid.UUID `json:"original_report_id,omitempty"`
}
