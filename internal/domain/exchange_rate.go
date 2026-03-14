package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type ExchangeRate struct {
	ID            uuid.UUID       `json:"id"`
	CompanyID     uuid.UUID       `json:"company_id"`
	FromCurrency  string          `json:"from_currency"`
	ToCurrency    string          `json:"to_currency"`
	Rate          decimal.Decimal `json:"rate"`
	EffectiveDate time.Time       `json:"effective_date"`
	Source        string          `json:"source"`
	CreatedAt     time.Time       `json:"created_at"`
}
