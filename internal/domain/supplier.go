package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Supplier struct {
	ID              uuid.UUID        `json:"id"`
	CompanyID       uuid.UUID        `json:"company_id"`
	TIN             string           `json:"tin"`
	Name            string           `json:"name"`
	Address         *string          `json:"address,omitempty"`
	SupplierType    string           `json:"supplier_type"`
	DefaultEWTRate  *decimal.Decimal `json:"default_ewt_rate,omitempty"`
	DefaultATCCode  *string          `json:"default_atc_code,omitempty"`
	IsVATRegistered bool             `json:"is_vat_registered"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}
