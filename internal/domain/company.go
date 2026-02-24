package domain

import (
	"time"

	"github.com/google/uuid"
)

type Company struct {
	ID                uuid.UUID  `json:"id"`
	OrganizationID    *uuid.UUID `json:"organization_id,omitempty"`
	CompanyName       string     `json:"company_name"`
	TINNumber         *string    `json:"tin_number,omitempty"`
	RDOCode           *string    `json:"rdo_code,omitempty"`
	VATClassification string     `json:"vat_classification"`
	FiscalYearEnd     string     `json:"fiscal_year_end"`
	Industry          *string    `json:"industry,omitempty"`
	Address           *string    `json:"address,omitempty"`
	Plan              string     `json:"plan"`
	Settings          JSON       `json:"settings"`
	IsActive          bool       `json:"is_active"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}
