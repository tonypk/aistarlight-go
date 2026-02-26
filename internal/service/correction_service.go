package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

const autoAnalyzeThreshold = 5 // min total corrections before auto-analyze
const autoAnalyzeNewMin    = 3 // min new corrections since last analyze

// CorrectionService records accountant corrections and tracks patterns.
type CorrectionService struct {
	q        *sqlc.Queries
	analyzer *CorrectionAnalyzer
}

// NewCorrectionService creates a CorrectionService.
func NewCorrectionService(q *sqlc.Queries) *CorrectionService {
	return &CorrectionService{q: q, analyzer: NewCorrectionAnalyzer(q)}
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

	output := &CorrectionOutput{
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
	}

	// Auto-analyze: trigger rule learning when enough corrections accumulate
	newRules := s.tryAutoAnalyze(ctx, input.CompanyID)
	if newRules > 0 {
		output.ContextData["auto_learned_rules"] = newRules
	}

	return output, nil
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

// tryAutoAnalyze checks if enough new corrections accumulated and auto-generates rules.
// Returns the number of new rules persisted (0 if skipped).
func (s *CorrectionService) tryAutoAnalyze(ctx context.Context, companyID uuid.UUID) int {
	if s.analyzer == nil {
		return 0
	}

	total, err := s.q.CountCorrectionsByCompany(ctx, companyID)
	if err != nil || total < int64(autoAnalyzeThreshold) {
		return 0
	}

	// Check existing rules to estimate "new" corrections
	existingRules, _ := s.q.ListActiveCorrectionRulesByCompany(ctx, companyID)
	var coveredCount int32
	for _, r := range existingRules {
		coveredCount += r.SourceCorrectionCount
	}
	newCorrections := int(total) - int(coveredCount)
	if newCorrections < autoAnalyzeNewMin {
		return 0
	}

	candidates, err := s.analyzer.AnalyzeCorrections(ctx, companyID)
	if err != nil || len(candidates) == 0 {
		return 0
	}

	// Only persist high-confidence candidates
	var highConf []RuleCandidate
	for _, c := range candidates {
		if c.Confidence >= 0.80 {
			highConf = append(highConf, c)
		}
	}
	if len(highConf) == 0 {
		return 0
	}

	persisted, err := s.analyzer.PersistCandidateRules(ctx, companyID, highConf)
	if err != nil {
		slog.Warn("auto-analyze: persist rules failed", "error", err)
		return 0
	}

	slog.Info("auto-analyze: learned new rules",
		"company_id", companyID,
		"candidates", len(highConf),
		"persisted", len(persisted),
	)
	return len(persisted)
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
