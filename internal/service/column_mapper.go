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
Per RR 16-2005 Section 4.114-3, as amended by RR 1-2012.

Target fields by report type:

BIR_2550M / BIR_2550Q (VAT) — covers SLS, SLP, SLI:
  General: date, taxable_month, description, tin, registered_name, supplier_name, address, invoice_number
  Amounts: amount, gross_amount, vat_amount, taxable_amount, exempt_amount, zero_rated_amount
  Classification: vat_type (vatable/exempt/zero_rated/government), category (goods/services/capital/imports)
  SLP (Purchases): gross_purchase, exempt_purchase, zero_rated_purchase, taxable_purchase, purchase_services, purchase_goods, purchase_capital_goods, input_tax, gross_taxable_purchase
  SLS (Sales): gross_sales, exempt_sales, zero_rated_sales, taxable_sales, output_tax, gross_taxable_sales
  SLI (Importations): import_entry_number, assessment_date, importation_date, country_of_origin, landed_cost, dutiable_value, customs_charges, taxable_imports, exempt_imports, vat_paid, vat_payment_date
  EWT: ewt_rate, ewt_amount, atc_code

BIR_1601C (Withholding on Compensation):
  employee_name, tin, total_compensation, statutory_minimum_wage,
  basic_pay, overtime_pay, holiday_pay,
  nontaxable_13th_month, nontaxable_deminimis,
  sss_gsis_phic_hdmf, sss, philhealth, pagibig,
  other_nontaxable, taxable_compensation, tax_withheld

BIR_0619E (Expanded Withholding):
  payee_name, tin, address, atc_code, nature_of_income,
  income_payment, ewt_rate, tax_withheld

Bank_Statement:
  date, description, amount, debit, credit, reference, balance

Rules:
- Match column headers to the closest target field based on meaning, not exact name.
- Common aliases: "OR No." / "Receipt No." -> invoice_number, "Vendor" / "Payee" -> supplier_name, "Net of VAT" -> taxable_amount, "VAT" -> vat_amount, "Gross" -> gross_amount.
- For purchase/expense sheets: prefer SLP fields (gross_purchase, taxable_purchase, input_tax).
- For sales/revenue sheets: prefer SLS fields (gross_sales, taxable_sales, output_tax).
- If a column clearly contains TIN numbers (###-###-###-###), map to "tin".

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

	// Strip markdown code fences if present (e.g. ```json ... ```)
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if strings.HasPrefix(content, "```") {
		if idx := strings.Index(content, "\n"); idx != -1 {
			content = content[idx+1:]
		}
		if strings.HasSuffix(content, "```") {
			content = content[:len(content)-3]
		}
		content = strings.TrimSpace(content)
	}

	var result ColumnMapping
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		slog.Warn("failed to parse column mapping response", "error", err, "raw", content)
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
