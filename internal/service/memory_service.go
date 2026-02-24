package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// MemoryService manages user preferences per company + report type.
type MemoryService struct {
	q *sqlc.Queries
}

// NewMemoryService creates a MemoryService.
func NewMemoryService(q *sqlc.Queries) *MemoryService {
	return &MemoryService{q: q}
}

// PreferenceOutput represents a user preference.
type PreferenceOutput struct {
	ID             uuid.UUID              `json:"id"`
	CompanyID      uuid.UUID              `json:"company_id"`
	ReportType     string                 `json:"report_type"`
	ColumnMappings map[string]interface{} `json:"column_mappings"`
	FormatRules    map[string]interface{} `json:"format_rules"`
	AutoFillRules  map[string]interface{} `json:"auto_fill_rules"`
	UpdatedAt      string                 `json:"updated_at"`
}

// GetPreference returns a single preference by company and report type.
func (s *MemoryService) GetPreference(ctx context.Context, companyID uuid.UUID, reportType string) (*PreferenceOutput, error) {
	pref, err := s.q.GetUserPreferenceByCompanyAndType(ctx, sqlc.GetUserPreferenceByCompanyAndTypeParams{
		CompanyID:  companyID,
		ReportType: reportType,
	})
	if err != nil {
		return nil, fmt.Errorf("preference not found: %w", err)
	}

	return toPreferenceOutput(pref), nil
}

// UpsertPreference creates or updates a preference with merge semantics.
func (s *MemoryService) UpsertPreference(ctx context.Context, companyID uuid.UUID, reportType string, columnMappings, formatRules, autoFillRules map[string]interface{}) (*PreferenceOutput, error) {
	// Load existing to merge
	existing, _ := s.q.GetUserPreferenceByCompanyAndType(ctx, sqlc.GetUserPreferenceByCompanyAndTypeParams{
		CompanyID:  companyID,
		ReportType: reportType,
	})

	mergedMappings := mergeJSON(existing.ColumnMappings, columnMappings)
	mergedFormats := mergeJSON(existing.FormatRules, formatRules)
	mergedAutoFill := mergeJSON(existing.AutoFillRules, autoFillRules)

	mappingsJSON, _ := json.Marshal(mergedMappings)
	formatsJSON, _ := json.Marshal(mergedFormats)
	autoFillJSON, _ := json.Marshal(mergedAutoFill)

	err := s.q.UpsertUserPreference(ctx, sqlc.UpsertUserPreferenceParams{
		ID:             uuid.New(),
		CompanyID:      companyID,
		ReportType:     reportType,
		ColumnMappings: mappingsJSON,
		FormatRules:    formatsJSON,
		AutoFillRules:  autoFillJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert preference: %w", err)
	}

	return s.GetPreference(ctx, companyID, reportType)
}

func toPreferenceOutput(p sqlc.UserPreference) *PreferenceOutput {
	out := &PreferenceOutput{
		ID:         p.ID,
		CompanyID:  p.CompanyID,
		ReportType: p.ReportType,
		UpdatedAt:  p.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if len(p.ColumnMappings) > 0 {
		_ = json.Unmarshal(p.ColumnMappings, &out.ColumnMappings)
	}
	if len(p.FormatRules) > 0 {
		_ = json.Unmarshal(p.FormatRules, &out.FormatRules)
	}
	if len(p.AutoFillRules) > 0 {
		_ = json.Unmarshal(p.AutoFillRules, &out.AutoFillRules)
	}
	return out
}

func mergeJSON(existing []byte, updates map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &merged)
	}
	for k, v := range updates {
		merged[k] = v
	}
	return merged
}
