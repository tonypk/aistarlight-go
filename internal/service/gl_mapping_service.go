package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqlc "github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

var (
	ErrGLMappingNotFound = errors.New("GL mapping rule not found")
)

// GLMappingService manages GL mapping rules for payroll→journal integration.
type GLMappingService struct {
	q *sqlc.Queries
}

// NewGLMappingService creates a new service.
func NewGLMappingService(q *sqlc.Queries) *GLMappingService {
	return &GLMappingService{q: q}
}

// CreateGLMappingInput holds data for creating a mapping rule.
type CreateGLMappingInput struct {
	CompanyID       uuid.UUID
	Jurisdiction    string
	SourceDimension string
	SourceValue     string
	TargetAccountID uuid.UUID
	DebitCredit     string
	Priority        int32
	EffectiveFrom   time.Time
}

// Create creates a new GL mapping rule.
func (s *GLMappingService) Create(ctx context.Context, input CreateGLMappingInput) (sqlc.GlMappingRule, error) {
	return s.q.CreateGLMappingRule(ctx, sqlc.CreateGLMappingRuleParams{
		CompanyID:       input.CompanyID,
		Jurisdiction:    input.Jurisdiction,
		SourceDimension: input.SourceDimension,
		SourceValue:     input.SourceValue,
		TargetAccountID: input.TargetAccountID,
		DebitCredit:     input.DebitCredit,
		Priority:        input.Priority,
		EffectiveFrom:   pgtype.Date{Time: input.EffectiveFrom, Valid: true},
	})
}

// List returns all active GL mapping rules for a company, optionally filtered by jurisdiction.
func (s *GLMappingService) List(ctx context.Context, companyID uuid.UUID, jurisdiction string) ([]sqlc.ListGLMappingRulesRow, error) {
	return s.q.ListGLMappingRules(ctx, sqlc.ListGLMappingRulesParams{
		CompanyID:    companyID,
		Jurisdiction: jurisdiction,
	})
}

// Get returns a single GL mapping rule.
func (s *GLMappingService) Get(ctx context.Context, companyID, id uuid.UUID) (sqlc.GlMappingRule, error) {
	rule, err := s.q.GetGLMappingRule(ctx, sqlc.GetGLMappingRuleParams{
		ID:        id,
		CompanyID: companyID,
	})
	if err != nil {
		return sqlc.GlMappingRule{}, ErrGLMappingNotFound
	}
	return rule, nil
}

// UpdateGLMappingInput holds data for updating a mapping rule.
type UpdateGLMappingInput struct {
	CompanyID       uuid.UUID
	ID              uuid.UUID
	TargetAccountID uuid.UUID
	DebitCredit     string
	Priority        int32
	EffectiveTo     *time.Time
	IsActive        bool
}

// Update updates a GL mapping rule.
func (s *GLMappingService) Update(ctx context.Context, input UpdateGLMappingInput) (sqlc.GlMappingRule, error) {
	var effectiveTo pgtype.Date
	if input.EffectiveTo != nil {
		effectiveTo = pgtype.Date{Time: *input.EffectiveTo, Valid: true}
	}

	return s.q.UpdateGLMappingRule(ctx, sqlc.UpdateGLMappingRuleParams{
		ID:              input.ID,
		CompanyID:       input.CompanyID,
		TargetAccountID: input.TargetAccountID,
		DebitCredit:     input.DebitCredit,
		Priority:        input.Priority,
		EffectiveTo:     effectiveTo,
		IsActive:        input.IsActive,
	})
}

// Delete removes a GL mapping rule.
func (s *GLMappingService) Delete(ctx context.Context, companyID, id uuid.UUID) error {
	return s.q.DeleteGLMappingRule(ctx, sqlc.DeleteGLMappingRuleParams{
		ID:        id,
		CompanyID: companyID,
	})
}

// GetActiveMappings returns active GL mappings for a company+jurisdiction at a given date.
func (s *GLMappingService) GetActiveMappings(ctx context.Context, companyID uuid.UUID, jurisdiction string, asOfDate time.Time) ([]sqlc.GetActiveGLMappingsRow, error) {
	return s.q.GetActiveGLMappings(ctx, sqlc.GetActiveGLMappingsParams{
		CompanyID:    companyID,
		Jurisdiction: jurisdiction,
		Column3:      pgtype.Date{Time: asOfDate, Valid: true},
	})
}

// MappingIndex builds a lookup map from (dimension, value) → mapping row.
type MappingIndex map[string]sqlc.GetActiveGLMappingsRow

// BuildIndex loads active mappings and indexes them by "dimension:value".
func (s *GLMappingService) BuildIndex(ctx context.Context, companyID uuid.UUID, jurisdiction string, asOfDate time.Time) (MappingIndex, error) {
	rows, err := s.GetActiveMappings(ctx, companyID, jurisdiction, asOfDate)
	if err != nil {
		return nil, fmt.Errorf("load GL mappings: %w", err)
	}

	idx := make(MappingIndex, len(rows))
	for _, r := range rows {
		key := r.SourceDimension + ":" + r.SourceValue
		// Higher priority wins (already sorted by priority DESC)
		if _, exists := idx[key]; !exists {
			idx[key] = r
		}
	}
	return idx, nil
}
