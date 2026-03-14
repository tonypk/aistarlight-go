package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type TaxCode struct {
	ID               uuid.UUID       `json:"id"`
	CompanyID        uuid.UUID       `json:"company_id"`
	Code             string          `json:"code"`
	Name             string          `json:"name"`
	TaxType          string          `json:"tax_type"` // vat/ewt/gst/wht
	Rate             decimal.Decimal `json:"rate"`
	IsInclusive      bool            `json:"is_inclusive"`
	AffectsAccountID *uuid.UUID      `json:"affects_account_id,omitempty"`
	Jurisdiction     string          `json:"jurisdiction"`
	IsActive         bool            `json:"is_active"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}
