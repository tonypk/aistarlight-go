package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Vendor struct {
	ID               uuid.UUID        `json:"id"`
	CompanyID        uuid.UUID        `json:"company_id"`
	TIN              string           `json:"tin"`
	Name             string           `json:"name"`
	Address          *string          `json:"address,omitempty"`
	VendorType       string           `json:"vendor_type"`
	DefaultEWTRate   *decimal.Decimal `json:"default_ewt_rate,omitempty"`
	DefaultATCCode   *string          `json:"default_atc_code,omitempty"`
	IsVATRegistered  bool             `json:"is_vat_registered"`
	Email            *string          `json:"email,omitempty"`
	Phone            *string          `json:"phone,omitempty"`
	PaymentTermsDays *int32           `json:"payment_terms_days,omitempty"`
	CurrencyCode     *string          `json:"currency_code,omitempty"`
	DefaultAccountID *uuid.UUID       `json:"default_account_id,omitempty"`
	IsActive         bool             `json:"is_active"`
	Notes            *string          `json:"notes,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}
