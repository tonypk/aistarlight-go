package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
)

const columnMapperSystemPrompt = `Expert Philippine tax accountant assistant.
Map spreadsheet column names to standard BIR form fields.

Target fields by report type:

BIR_2550M / BIR_2550Q (VAT):
  date, description, amount, vat_amount, vat_type, category, tin,
  invoice_number, supplier_name, address, gross_amount, taxable_amount,
  ewt_rate, ewt_amount, atc_code

BIR_1601C (Withholding on Compensation):
  employee_name, tin, total_compensation, statutory_minimum_wage,
  nontaxable_13th_month, nontaxable_deminimis, sss_gsis_phic_hdmf,
  other_nontaxable, tax_withheld, basic_pay, overtime_pay, holiday_pay,
  sss, philhealth, pagibig, taxable_compensation

BIR_0619E (Expanded Withholding):
  payee_name, tin, atc_code, income_payment, tax_withheld,
  address, nature_of_income, ewt_rate

Bank Statements:
  date, description, amount, debit, credit, reference, balance

Respond ONLY with valid JSON:
{
  "mappings": {"source_column_name": "target_field_name", ...},
  "unmapped": ["column_names_that_dont_map"],
  "confidence": 0.95,
  "field_confidence": {"source_column_name": 0.95, ...}
}

field_confidence: per-column confidence (0.0-1.0) indicating how sure you are about each mapping.`

// ColumnMapperService handles AI-powered column mapping.
type ColumnMapperService struct {
	ai *openai.Client
}

// NewColumnMapperService creates a column mapper.
func NewColumnMapperService(ai *openai.Client) *ColumnMapperService {
	return &ColumnMapperService{ai: ai}
}

// ColumnMapping holds the result of column mapping.
type ColumnMapping struct {
	Mappings        map[string]string  `json:"mappings"`
	Unmapped        []string           `json:"unmapped"`
	Confidence      float64            `json:"confidence"`
	FieldConfidence map[string]float64 `json:"field_confidence,omitempty"`
}

// AutoMapColumns maps spreadsheet columns to BIR form fields using AI.
func (s *ColumnMapperService) AutoMapColumns(
	ctx context.Context,
	columns []string,
	sampleRows []map[string]interface{},
	reportType string,
	existingMappings map[string]string,
) (*ColumnMapping, error) {
	// If existing mappings cover all columns, reuse them
	if len(existingMappings) > 0 && allColumnsMapped(columns, existingMappings) {
		fc := make(map[string]float64, len(columns))
		for _, col := range columns {
			fc[col] = 1.0
		}
		return &ColumnMapping{
			Mappings:        existingMappings,
			Unmapped:        []string{},
			Confidence:      1.0,
			FieldConfidence: fc,
		}, nil
	}

	// Build user prompt
	maxSampleRows := 3
	if len(sampleRows) < maxSampleRows {
		maxSampleRows = len(sampleRows)
	}
	sampleJSON, _ := json.Marshal(sampleRows[:maxSampleRows])

	userPrompt := fmt.Sprintf(
		"Report type: %s\nColumns: %s\nSample data (first %d rows): %s",
		reportType,
		strings.Join(columns, ", "),
		maxSampleRows,
		string(sampleJSON),
	)

	if len(existingMappings) > 0 {
		existingJSON, _ := json.Marshal(existingMappings)
		userPrompt += fmt.Sprintf("\n\nPrevious mappings (prefer reusing): %s", string(existingJSON))
	}

	resp, err := s.ai.ChatCompletion(ctx, []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: columnMapperSystemPrompt},
		{Role: oai.ChatMessageRoleUser, Content: userPrompt},
	}, openai.WithTemperature(0.1))

	if err != nil {
		slog.Warn("column mapping LLM failed", "error", err)
		return &ColumnMapping{
			Mappings:   map[string]string{},
			Unmapped:   columns,
			Confidence: 0.0,
		}, nil
	}

	if len(resp.Choices) == 0 {
		return &ColumnMapping{
			Mappings:   map[string]string{},
			Unmapped:   columns,
			Confidence: 0.0,
		}, nil
	}

	var result ColumnMapping
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		slog.Warn("failed to parse column mapping response", "error", err)
		return &ColumnMapping{
			Mappings:   map[string]string{},
			Unmapped:   columns,
			Confidence: 0.0,
		}, nil
	}

	return &result, nil
}

func allColumnsMapped(columns []string, mappings map[string]string) bool {
	for _, col := range columns {
		if _, ok := mappings[col]; !ok {
			return false
		}
	}
	return true
}
