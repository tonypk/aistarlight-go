package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// FormSchemaService loads form schemas from the database with hardcoded fallback.
type FormSchemaService struct {
	q *sqlc.Queries
}

// NewFormSchemaService creates a form schema service.
func NewFormSchemaService(q *sqlc.Queries) *FormSchemaService {
	return &FormSchemaService{q: q}
}

// FormSchemaDTO is the API representation of a form schema.
type FormSchemaDTO struct {
	FormType         string      `json:"form_type"`
	Name             string      `json:"name"`
	Frequency        string      `json:"frequency"`
	Version          int32       `json:"version"`
	SchemaDef        interface{} `json:"schema_def"`
	CalculationRules interface{} `json:"calculation_rules"`
}

// FormSummaryDTO is a lightweight form listing entry.
type FormSummaryDTO struct {
	FormType  string `json:"form_type"`
	Name      string `json:"name"`
	Frequency string `json:"frequency"`
	Status    string `json:"status"`
}

// GetSchema retrieves a form schema by type, DB first, hardcoded fallback.
func (s *FormSchemaService) GetSchema(ctx context.Context, formType string) (*FormSchemaDTO, error) {
	schema, err := s.q.GetFormSchemaByType(ctx, formType)
	if err == nil {
		return dbSchemaToDTO(schema), nil
	}
	slog.Debug("form schema not in DB, checking hardcoded", "form_type", formType, "error", err)
	return nil, fmt.Errorf("form schema %s not found", formType)
}

// ListForms returns all active form schemas filtered by jurisdiction.
func (s *FormSchemaService) ListForms(ctx context.Context, jurisdiction string) ([]FormSummaryDTO, error) {
	if jurisdiction == "" {
		jurisdiction = "PH"
	}
	schemas, err := s.q.ListActiveFormSchemas(ctx, jurisdiction)
	if err != nil {
		return nil, fmt.Errorf("list form schemas: %w", err)
	}

	summaries := make([]FormSummaryDTO, len(schemas))
	for i, sc := range schemas {
		summaries[i] = FormSummaryDTO{
			FormType:  sc.FormType,
			Name:      sc.Name,
			Frequency: sc.Frequency,
			Status:    "active",
		}
	}
	return summaries, nil
}

func dbSchemaToDTO(sc sqlc.FormSchema) *FormSchemaDTO {
	var schemaDef, calcRules interface{}
	_ = json.Unmarshal(sc.SchemaDef, &schemaDef)
	_ = json.Unmarshal(sc.CalculationRules, &calcRules)

	return &FormSchemaDTO{
		FormType:         sc.FormType,
		Name:             sc.Name,
		Frequency:        sc.Frequency,
		Version:          sc.Version,
		SchemaDef:        schemaDef,
		CalculationRules: calcRules,
	}
}
