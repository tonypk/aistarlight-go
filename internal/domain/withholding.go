package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type WithholdingCertificate struct {
	ID           uuid.UUID       `json:"id"`
	CompanyID    uuid.UUID       `json:"company_id"`
	SessionID    *uuid.UUID      `json:"session_id,omitempty"`
	SupplierID   uuid.UUID       `json:"supplier_id"`
	Period       string          `json:"period"`
	Quarter      string          `json:"quarter"`
	ATCCode      string          `json:"atc_code"`
	IncomeType   string          `json:"income_type"`
	IncomeAmount decimal.Decimal `json:"income_amount"`
	EWTRate      decimal.Decimal `json:"ewt_rate"`
	TaxWithheld  decimal.Decimal `json:"tax_withheld"`
	Status       string          `json:"status"`
	FilePath     *string         `json:"file_path,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}
