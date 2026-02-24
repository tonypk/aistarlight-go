package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

var (
	ErrPeriodNotFound    = errors.New("accounting period not found")
	ErrPeriodNotOpen     = errors.New("accounting period is not open")
	ErrPeriodNotClosed   = errors.New("accounting period is not closed")
	ErrPeriodHasDraftJE  = errors.New("period has draft journal entries; post or delete them first")
)

type AccountingPeriodService struct {
	q *sqlc.Queries
}

func NewAccountingPeriodService(q *sqlc.Queries) *AccountingPeriodService {
	return &AccountingPeriodService{q: q}
}

type CreatePeriodInput struct {
	CompanyID  uuid.UUID
	Name       string
	PeriodType domain.PeriodType
	StartDate  time.Time
	EndDate    time.Time
}

func (s *AccountingPeriodService) Create(ctx context.Context, input CreatePeriodInput) (*domain.AccountingPeriod, error) {
	dbPeriod, err := s.q.CreateAccountingPeriod(ctx, sqlc.CreateAccountingPeriodParams{
		ID:         uuid.New(),
		CompanyID:  input.CompanyID,
		Name:       input.Name,
		PeriodType: string(input.PeriodType),
		StartDate:  pgtype.Date{Time: input.StartDate, Valid: true},
		EndDate:    pgtype.Date{Time: input.EndDate, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("create period: %w", err)
	}
	return toPeriod(dbPeriod), nil
}

func (s *AccountingPeriodService) GetByID(ctx context.Context, id uuid.UUID) (*domain.AccountingPeriod, error) {
	dbPeriod, err := s.q.GetAccountingPeriodByID(ctx, id)
	if err != nil {
		return nil, ErrPeriodNotFound
	}
	return toPeriod(dbPeriod), nil
}

func (s *AccountingPeriodService) List(ctx context.Context, companyID uuid.UUID) ([]domain.AccountingPeriod, error) {
	periods, err := s.q.ListAccountingPeriods(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("list periods: %w", err)
	}
	result := make([]domain.AccountingPeriod, len(periods))
	for i, p := range periods {
		result[i] = *toPeriod(p)
	}
	return result, nil
}

func (s *AccountingPeriodService) Close(ctx context.Context, id uuid.UUID, closedBy uuid.UUID) error {
	period, err := s.q.GetAccountingPeriodByID(ctx, id)
	if err != nil {
		return ErrPeriodNotFound
	}
	if period.Status != string(domain.PeriodOpen) {
		return ErrPeriodNotOpen
	}

	// Check for draft journal entries in this period
	draftCount, err := s.q.CountDraftEntriesByPeriod(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		return fmt.Errorf("count draft entries: %w", err)
	}
	if draftCount > 0 {
		return ErrPeriodHasDraftJE
	}

	return s.q.CloseAccountingPeriod(ctx, sqlc.CloseAccountingPeriodParams{
		ID:       id,
		ClosedBy: pgtype.UUID{Bytes: closedBy, Valid: true},
	})
}

func (s *AccountingPeriodService) Reopen(ctx context.Context, id uuid.UUID) error {
	period, err := s.q.GetAccountingPeriodByID(ctx, id)
	if err != nil {
		return ErrPeriodNotFound
	}
	if period.Status != string(domain.PeriodClosed) {
		return ErrPeriodNotClosed
	}
	return s.q.ReopenAccountingPeriod(ctx, id)
}

// GenerateMonthly creates monthly periods for a fiscal year.
func (s *AccountingPeriodService) GenerateMonthly(ctx context.Context, companyID uuid.UUID, year int) ([]domain.AccountingPeriod, error) {
	var result []domain.AccountingPeriod
	for month := time.January; month <= time.December; month++ {
		start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, -1)
		name := start.Format("January 2006")

		p, err := s.Create(ctx, CreatePeriodInput{
			CompanyID:  companyID,
			Name:       name,
			PeriodType: domain.PeriodMonthly,
			StartDate:  start,
			EndDate:    end,
		})
		if err != nil {
			return result, fmt.Errorf("generate month %s: %w", name, err)
		}
		result = append(result, *p)
	}
	return result, nil
}

func toPeriod(p sqlc.AccountingPeriod) *domain.AccountingPeriod {
	ap := &domain.AccountingPeriod{
		ID:         p.ID,
		CompanyID:  p.CompanyID,
		Name:       p.Name,
		PeriodType: domain.PeriodType(p.PeriodType),
		Status:     domain.PeriodStatus(p.Status),
		CreatedAt:  p.CreatedAt,
		UpdatedAt:  p.UpdatedAt,
	}
	if p.StartDate.Valid {
		ap.StartDate = p.StartDate.Time
	}
	if p.EndDate.Valid {
		ap.EndDate = p.EndDate.Time
	}
	if p.ClosedBy.Valid {
		id := uuid.UUID(p.ClosedBy.Bytes)
		ap.ClosedBy = &id
	}
	if p.ClosedAt.Valid {
		t := p.ClosedAt.Time
		ap.ClosedAt = &t
	}
	return ap
}
