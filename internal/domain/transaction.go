package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Transaction struct {
	ID                   uuid.UUID       `json:"id"`
	CompanyID            uuid.UUID       `json:"company_id"`
	SessionID            uuid.UUID       `json:"session_id"`
	SourceType           string          `json:"source_type"`
	SourceFileID         string          `json:"source_file_id"`
	RowIndex             int             `json:"row_index"`
	Date                 *time.Time      `json:"date,omitempty"`
	Description          *string         `json:"description,omitempty"`
	Amount               decimal.Decimal `json:"amount"`
	VATAmount            decimal.Decimal `json:"vat_amount"`
	VATType              string          `json:"vat_type"`
	Category             string          `json:"category"`
	TIN                  *string         `json:"tin,omitempty"`
	Confidence           decimal.Decimal `json:"confidence"`
	ClassificationSource string          `json:"classification_source"`
	RawData              JSON            `json:"raw_data,omitempty"`
	MatchGroupID         *uuid.UUID      `json:"match_group_id,omitempty"`
	MatchStatus          string          `json:"match_status"`
	EWTRate              *decimal.Decimal `json:"ewt_rate,omitempty"`
	EWTAmount            *decimal.Decimal `json:"ewt_amount,omitempty"`
	ATCCode              *string         `json:"atc_code,omitempty"`
	SupplierID           *uuid.UUID      `json:"supplier_id,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}
