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

const columnMapperSystemPrompt = `Expert Philippine CPA assistant for BIR VAT Return filing.
Map spreadsheet columns to official BIR 2550M/2550Q form fields.
Per BIR Form 2550M/2550Q, RR 16-2005, RR 1-2012.

=== Target fields by report type ===

BIR_2550M / BIR_2550Q (VAT Return):

  Sales (Output):
    sales_date, sales_invoice_number, customer_name, customer_tin, customer_address,
    gross_sales, vatable_sales, sales_to_government, zero_rated_sales, exempt_sales,
    total_sales, output_tax

  Purchases (Input):
    supplier_name, supplier_tin, supplier_address, purchase_date, purchase_invoice_number,
    gross_purchase, purchase_capital_goods_below_1m, purchase_capital_goods_above_1m,
    purchase_domestic_goods, purchase_importation, purchase_domestic_services,
    purchase_non_resident_services, purchase_not_qualified,
    input_tax, input_tax_capital_goods, input_tax_domestic_goods,
    input_tax_importation, input_tax_domestic_services, input_tax_non_resident_services

  VAT Deductions:
    input_tax_carried_over, deferred_input_tax_capital, transitional_input_tax,
    presumptive_input_tax, other_input_tax, total_allowable_input_tax

  Details:
    tin, registered_name, address, description, taxable_month

  Importation (SLI):
    import_entry_number, importation_date, assessment_date, country_of_origin,
    landed_cost, dutiable_value, customs_charges, vat_paid_imports

  EWT:
    ewt_rate, ewt_amount, atc_code

BIR_1601C (Withholding on Compensation):
  employee_name, tin, total_compensation, statutory_minimum_wage,
  basic_pay, overtime_pay, holiday_pay, nontaxable_13th_month,
  nontaxable_deminimis, sss_gsis_phic_hdmf, sss, philhealth, pagibig,
  other_nontaxable, taxable_compensation, tax_withheld

BIR_0619E (Monthly Remittance of Creditable Withholding Tax — Expanded):

  Payee Info:
    supplier_name, supplier_tin, supplier_address

  Transaction:
    invoice_date, invoice_number, description, expense_category

  Withholding Tax (per ATC line):
    atc_code, nature_of_income, tax_base, ewt_rate, tax_withheld

  Summary:
    total_tax_withheld, tax_remitted_previous, tax_still_due,
    penalty_surcharge, penalty_interest, penalty_compromise, total_amount_payable

Bank_Statement:
  date, description, amount, debit, credit, reference, balance

=== Mapping Rules ===

For SALES sheets (SLS — Summary List of Sales):
- "Gross Sales" / "Gross Amount" / "Total Sales" → gross_sales
- "Vatable" / "Taxable Sales" / "Net of VAT" → vatable_sales
- "Zero Rated" / "Zero-Rated" → zero_rated_sales
- "Exempt" / "VAT Exempt" → exempt_sales
- "Output Tax" / "Output VAT" / "VAT Due" → output_tax
- "Gov Sales" / "Govt" → sales_to_government
- "Date" / "Invoice Date" / "Sales Date" → sales_date
- "SI No." / "Invoice No." / "OR No." / "Receipt No." → sales_invoice_number
- "Customer" / "Buyer" / "Client" → customer_name

For PURCHASE sheets (SLP — Summary List of Purchases):
- "Gross Purchase" / "Total Purchase" / "Gross Amount" → gross_purchase
- "Input Tax" / "Input VAT" / "VAT Input" → input_tax
- "Supplier" / "Vendor" / "Payee" → supplier_name
- "Date" / "Purchase Date" → purchase_date
- "Invoice No." / "OR No." / "Receipt No." → purchase_invoice_number
- "Capital Goods" → purchase_capital_goods_below_1m or purchase_capital_goods_above_1m
- "Domestic Goods" / "Local Purchase" → purchase_domestic_goods
- "Import" / "Importation" → purchase_importation
- "Services" / "Service Purchase" → purchase_domestic_services

For TIN columns (###-###-###-###): map to customer_tin (sales) or supplier_tin (purchases) or tin (general).

For EWT sheets (BIR 0619-E — Expanded Withholding):
- "Supplier" / "Vendor" / "Payee" / "Payee Name" → supplier_name
- "TIN" (of supplier/payee) → supplier_tin
- "Address" (of supplier/payee) → supplier_address
- "Date" / "Invoice Date" / "Payment Date" → invoice_date
- "Invoice No." / "OR No." / "Receipt No." → invoice_number
- "Description" / "Particulars" / "Remarks" → description
- "Category" / "Expense Type" / "Account" → expense_category
- "ATC" / "ATC Code" / "Tax Code" → atc_code
- "Nature of Income" / "Income Type" / "Nature of Payment" → nature_of_income
- "Tax Base" / "Gross Amount" / "Income Payment" → tax_base
- "Rate" / "Tax Rate" / "EWT Rate" / "WTax Rate" → ewt_rate
- "Tax Withheld" / "WTax" / "Withholding Tax" / "Tax Amount" → tax_withheld

IMPORTANT: Use field names EXACTLY as listed above. Do not invent new field names.

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
