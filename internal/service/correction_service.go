package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// CorrectionService records accountant corrections and tracks patterns.
type CorrectionService struct {
	q *sqlc.Queries
}

// NewCorrectionService creates a CorrectionService.
func NewCorrectionService(q *sqlc.Queries) *CorrectionService {
	return &CorrectionService{q: q}
}

// RecordCorrectionInput holds input for recording a correction.
type RecordCorrectionInput struct {
	CompanyID  uuid.UUID
	UserID     uuid.UUID
	EntityType string // transaction_classification, report_field, ewt_classification
	EntityID   uuid.UUID
	FieldName  string
	OldValue   *string
	NewValue   string
	Reason     *string
}

// CorrectionOutput represents a persisted correction.
type CorrectionOutput struct {
	ID          uuid.UUID              `json:"id"`
	CompanyID   uuid.UUID              `json:"company_id"`
	UserID      uuid.UUID              `json:"user_id"`
	EntityType  string                 `json:"entity_type"`
	EntityID    uuid.UUID              `json:"entity_id"`
	FieldName   string                 `json:"field_name"`
	OldValue    *string                `json:"old_value"`
	NewValue    string                 `json:"new_value"`
	Reason      *string                `json:"reason"`
	ContextData map[string]interface{} `json:"context_data,omitempty"`
	CreatedAt   string                 `json:"created_at"`
}

// RecordCorrection persists a correction with context snapshot.
func (s *CorrectionService) RecordCorrection(ctx context.Context, input RecordCorrectionInput) (*CorrectionOutput, error) {
	// Build context snapshot based on entity type
	contextData := s.buildContextSnapshot(ctx, input.EntityType, input.EntityID)
	contextJSON, _ := json.Marshal(contextData)

	correction, err := s.q.CreateCorrection(ctx, sqlc.CreateCorrectionParams{
		ID:          uuid.New(),
		CompanyID:   input.CompanyID,
		UserID:      input.UserID,
		EntityType:  input.EntityType,
		EntityID:    input.EntityID,
		FieldName:   input.FieldName,
		OldValue:    input.OldValue,
		NewValue:    input.NewValue,
		Reason:      input.Reason,
		ContextData: contextJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("create correction: %w", err)
	}

	return &CorrectionOutput{
		ID:          correction.ID,
		CompanyID:   correction.CompanyID,
		UserID:      correction.UserID,
		EntityType:  correction.EntityType,
		EntityID:    correction.EntityID,
		FieldName:   correction.FieldName,
		OldValue:    correction.OldValue,
		NewValue:    correction.NewValue,
		Reason:      correction.Reason,
		ContextData: contextData,
		CreatedAt:   correction.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// GetEntityCorrections returns all corrections for a specific entity.
func (s *CorrectionService) GetEntityCorrections(ctx context.Context, entityType string, entityID uuid.UUID) ([]CorrectionOutput, error) {
	rows, err := s.q.ListCorrectionsByEntity(ctx, sqlc.ListCorrectionsByEntityParams{
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return nil, err
	}

	return s.toCorrectionOutputs(rows), nil
}

// ListCorrections returns paginated corrections for a company.
func (s *CorrectionService) ListCorrections(ctx context.Context, companyID uuid.UUID, limit, offset int) ([]CorrectionOutput, int64, error) {
	rows, err := s.q.ListCorrectionsByCompany(ctx, sqlc.ListCorrectionsByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}

	total, err := s.q.CountCorrectionsByCompany(ctx, companyID)
	if err != nil {
		return nil, 0, err
	}

	return s.toCorrectionOutputs(rows), total, nil
}

// GetCorrectionStats returns aggregate statistics.
func (s *CorrectionService) GetCorrectionStats(ctx context.Context, companyID uuid.UUID) (map[string]interface{}, error) {
	total, err := s.q.CountCorrectionsByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}

	corrections, err := s.q.ListCorrectionsByCompany(ctx, sqlc.ListCorrectionsByCompanyParams{
		CompanyID: companyID,
		Limit:     500,
		Offset:    0,
	})
	if err != nil {
		return nil, err
	}

	byEntity := make(map[string]int)
	byField := make(map[string]int)
	for _, c := range corrections {
		byEntity[c.EntityType]++
		byField[c.FieldName]++
	}

	return map[string]interface{}{
		"total_corrections": total,
		"by_entity_type":    byEntity,
		"by_field":          byField,
	}, nil
}

func (s *CorrectionService) buildContextSnapshot(ctx context.Context, entityType string, entityID uuid.UUID) map[string]interface{} {
	snapshot := map[string]interface{}{"entity_type": entityType, "entity_id": entityID.String()}

	switch entityType {
	case "report_field":
		report, err := s.q.GetReportByID(ctx, entityID)
		if err == nil {
			snapshot["report_type"] = report.ReportType
			snapshot["period"] = report.Period
			snapshot["status"] = report.Status
		}
	}

	return snapshot
}

func (s *CorrectionService) toCorrectionOutputs(rows []sqlc.Correction) []CorrectionOutput {
	results := make([]CorrectionOutput, len(rows))
	for i, c := range rows {
		var ctx map[string]interface{}
		if len(c.ContextData) > 0 {
			_ = json.Unmarshal(c.ContextData, &ctx)
		}
		results[i] = CorrectionOutput{
			ID:          c.ID,
			CompanyID:   c.CompanyID,
			UserID:      c.UserID,
			EntityType:  c.EntityType,
			EntityID:    c.EntityID,
			FieldName:   c.FieldName,
			OldValue:    c.OldValue,
			NewValue:    c.NewValue,
			Reason:      c.Reason,
			ContextData: ctx,
			CreatedAt:   c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return results
}
