package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// TaxCodeService handles tax code CRUD.
type TaxCodeService struct {
	q *sqlc.Queries
}

func NewTaxCodeService(q *sqlc.Queries) *TaxCodeService {
	return &TaxCodeService{q: q}
}

type TaxCodeResponse struct {
	ID               string  `json:"id"`
	Code             string  `json:"code"`
	Name             string  `json:"name"`
	TaxType          string  `json:"tax_type"`
	Rate             float64 `json:"rate"`
	IsInclusive      bool    `json:"is_inclusive"`
	AffectsAccountID *string `json:"affects_account_id,omitempty"`
	Jurisdiction     string  `json:"jurisdiction"`
	IsActive         bool    `json:"is_active"`
}

func taxCodeToResponse(tc sqlc.TaxCode) TaxCodeResponse {
	resp := TaxCodeResponse{
		ID:          tc.ID.String(),
		Code:        tc.Code,
		Name:        tc.Name,
		TaxType:     tc.TaxType,
		IsInclusive: tc.IsInclusive,
		Jurisdiction: tc.Jurisdiction,
		IsActive:    tc.IsActive,
	}
	if f, err := tc.Rate.Float64Value(); err == nil {
		resp.Rate = f.Float64
	}
	if tc.AffectsAccountID.Valid {
		id := uuid.UUID(tc.AffectsAccountID.Bytes).String()
		resp.AffectsAccountID = &id
	}
	return resp
}

type CreateTaxCodeInput struct {
	Code             string  `json:"code" binding:"required"`
	Name             string  `json:"name" binding:"required"`
	TaxType          string  `json:"tax_type" binding:"required"`
	Rate             float64 `json:"rate" binding:"required"`
	IsInclusive      bool    `json:"is_inclusive"`
	AffectsAccountID *string `json:"affects_account_id"`
	Jurisdiction     string  `json:"jurisdiction"`
	IsActive         bool    `json:"is_active"`
}

func (s *TaxCodeService) Create(ctx context.Context, companyID uuid.UUID, input CreateTaxCodeInput) (*TaxCodeResponse, error) {
	jurisdiction := input.Jurisdiction
	if jurisdiction == "" {
		jurisdiction = "PH"
	}

	var rateNum pgtype.Numeric
	scanNumeric(&rateNum, input.Rate)

	var affectsID pgtype.UUID
	if input.AffectsAccountID != nil {
		id, err := uuid.Parse(*input.AffectsAccountID)
		if err == nil {
			affectsID = pgtype.UUID{Bytes: id, Valid: true}
		}
	}

	tc, err := s.q.CreateTaxCode(ctx, sqlc.CreateTaxCodeParams{
		ID:               uuid.New(),
		CompanyID:        companyID,
		Code:             input.Code,
		Name:             input.Name,
		TaxType:          input.TaxType,
		Rate:             rateNum,
		IsInclusive:      input.IsInclusive,
		AffectsAccountID: affectsID,
		Jurisdiction:     jurisdiction,
		IsActive:         input.IsActive,
	})
	if err != nil {
		return nil, fmt.Errorf("create tax code: %w", err)
	}

	resp := taxCodeToResponse(tc)
	return &resp, nil
}

func (s *TaxCodeService) List(ctx context.Context, companyID uuid.UUID) ([]TaxCodeResponse, error) {
	codes, err := s.q.ListTaxCodesByCompany(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("list tax codes: %w", err)
	}

	result := make([]TaxCodeResponse, len(codes))
	for i, tc := range codes {
		result[i] = taxCodeToResponse(tc)
	}
	return result, nil
}

func (s *TaxCodeService) GetByCode(ctx context.Context, companyID uuid.UUID, code string) (*TaxCodeResponse, error) {
	tc, err := s.q.GetTaxCodeByCode(ctx, sqlc.GetTaxCodeByCodeParams{
		CompanyID: companyID,
		Code:      code,
	})
	if err != nil {
		return nil, fmt.Errorf("tax code not found")
	}
	resp := taxCodeToResponse(tc)
	return &resp, nil
}

func (s *TaxCodeService) Delete(ctx context.Context, id, companyID uuid.UUID) error {
	return s.q.DeleteTaxCode(ctx, sqlc.DeleteTaxCodeParams{
		ID:        id,
		CompanyID: companyID,
	})
}
