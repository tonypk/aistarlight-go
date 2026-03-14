package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// ExchangeRateService handles exchange rate CRUD.
type ExchangeRateService struct {
	q *sqlc.Queries
}

func NewExchangeRateService(q *sqlc.Queries) *ExchangeRateService {
	return &ExchangeRateService{q: q}
}

type ExchangeRateResponse struct {
	ID            string  `json:"id"`
	FromCurrency  string  `json:"from_currency"`
	ToCurrency    string  `json:"to_currency"`
	Rate          float64 `json:"rate"`
	EffectiveDate string  `json:"effective_date"`
	Source        string  `json:"source"`
}

func exchangeRateToResponse(er sqlc.ExchangeRate) ExchangeRateResponse {
	resp := ExchangeRateResponse{
		ID:            er.ID.String(),
		FromCurrency:  er.FromCurrency,
		ToCurrency:    er.ToCurrency,
		EffectiveDate: er.EffectiveDate.Time.Format("2006-01-02"),
		Source:        *er.Source,
	}
	if f, err := er.Rate.Float64Value(); err == nil {
		resp.Rate = f.Float64
	}
	return resp
}

type CreateExchangeRateInput struct {
	FromCurrency  string  `json:"from_currency" binding:"required"`
	ToCurrency    string  `json:"to_currency" binding:"required"`
	Rate          float64 `json:"rate" binding:"required"`
	EffectiveDate string  `json:"effective_date" binding:"required"`
	Source        string  `json:"source"`
}

func (s *ExchangeRateService) Create(ctx context.Context, companyID uuid.UUID, input CreateExchangeRateInput) (*ExchangeRateResponse, error) {
	effDate, err := time.Parse("2006-01-02", input.EffectiveDate)
	if err != nil {
		return nil, fmt.Errorf("invalid effective_date: %w", err)
	}

	source := input.Source
	if source == "" {
		source = "manual"
	}

	var rateNum pgtype.Numeric
	scanNumeric(&rateNum, input.Rate)

	er, err := s.q.CreateExchangeRate(ctx, sqlc.CreateExchangeRateParams{
		ID:            uuid.New(),
		CompanyID:     companyID,
		FromCurrency:  input.FromCurrency,
		ToCurrency:    input.ToCurrency,
		Rate:          rateNum,
		EffectiveDate: pgtype.Date{Time: effDate, Valid: true},
		Source:        &source,
	})
	if err != nil {
		return nil, fmt.Errorf("create exchange rate: %w", err)
	}

	resp := exchangeRateToResponse(er)
	return &resp, nil
}

func (s *ExchangeRateService) List(ctx context.Context, companyID uuid.UUID, limit, offset int) ([]ExchangeRateResponse, int64, error) {
	rates, err := s.q.ListExchangeRatesByCompany(ctx, sqlc.ListExchangeRatesByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list exchange rates: %w", err)
	}

	total, err := s.q.CountExchangeRatesByCompany(ctx, companyID)
	if err != nil {
		return nil, 0, err
	}

	result := make([]ExchangeRateResponse, len(rates))
	for i, er := range rates {
		result[i] = exchangeRateToResponse(er)
	}
	return result, total, nil
}

func (s *ExchangeRateService) GetLatest(ctx context.Context, companyID uuid.UUID, from, to string, asOf time.Time) (*ExchangeRateResponse, error) {
	er, err := s.q.GetLatestExchangeRate(ctx, sqlc.GetLatestExchangeRateParams{
		CompanyID:     companyID,
		FromCurrency:  from,
		ToCurrency:    to,
		EffectiveDate: pgtype.Date{Time: asOf, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("exchange rate not found")
	}
	resp := exchangeRateToResponse(er)
	return &resp, nil
}

func (s *ExchangeRateService) Delete(ctx context.Context, id, companyID uuid.UUID) error {
	return s.q.DeleteExchangeRate(ctx, sqlc.DeleteExchangeRateParams{
		ID:        id,
		CompanyID: companyID,
	})
}
