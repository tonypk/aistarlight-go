package cleaning

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
)

const mergedSemanticPrompt = `You are a spreadsheet data analysis expert for Philippine BIR tax forms and financial documents.

Given the rows of a spreadsheet, analyze and return a SINGLE JSON object with ALL of the following:

1. **table_type**: What kind of data is this? One of: "sales", "purchase", "bank", "payroll", "ewt", "itr", "unknown"

2. **header_rows**: Array of 0-indexed row indices that form the column headers (may be multi-row).

3. **data_start_row**: 0-indexed first row of actual transaction/record data.

4. **data_end_row**: 0-indexed last row of actual data (INCLUSIVE). Exclude:
   - Summary/total rows ("Grand Total", "Subtotal", etc.)
   - Notes, signatures, "Prepared by", "Certified correct"
   - Empty separator rows at bottom

5. **columns**: Merged column header names (if multi-row headers, concatenate with spaces).
   - Remove trailing empty columns
   - Use "Column_N" for unnamed columns (1-based)

6. **drop_columns**: Array of 0-indexed column indices that should be removed (entirely empty, or row numbering columns like "(1)", "(2)", "(3)").

7. **column_mapping**: Map each column name to the appropriate BIR field:
   - For SALES: sales_date, sales_invoice_number, customer_name, customer_tin, customer_address, gross_sales, vatable_sales, sales_to_government, zero_rated_sales, exempt_sales, total_sales, output_tax
   - For PURCHASES: supplier_name, supplier_tin, supplier_address, purchase_date, purchase_invoice_number, gross_purchase, purchase_capital_goods_below_1m, purchase_capital_goods_above_1m, purchase_domestic_goods, purchase_importation, purchase_domestic_services, input_tax, input_tax_capital_goods, input_tax_domestic_goods
   - For PAYROLL/2316: employee_name, employee_tin, basic_salary, overtime_pay, holiday_pay, thirteenth_month_pay, gross_compensation, taxable_compensation, total_tax_withheld, sss_gsis_contribution, philhealth_contribution, pagibig_contribution
   - For EWT/0619E: supplier_name, supplier_tin, supplier_address, invoice_date, invoice_number, description, atc_code, nature_of_income, tax_base, ewt_rate, tax_withheld
   - For BANK: date, description, amount, debit, credit, reference, balance
   - For ITR: gross_sales_receipts, cost_of_sales, gross_income, net_sales, expense_amount, expense_category, taxable_income

   Format: {"column_name": {"target_field": "field_name", "confidence": 0.95}}
   Set confidence 0.0 for unmappable columns.

8. **warnings**: Array of issues found (e.g., "mixed date formats", "possible duplicate columns").

Return ONLY valid JSON with this exact structure:
{
  "table_type": "purchase",
  "header_rows": [3, 4],
  "data_start_row": 6,
  "data_end_row": 150,
  "columns": ["TAXABLE MONTH", "TIN", "REGISTERED NAME", ...],
  "drop_columns": [0],
  "column_mapping": {"TAXABLE MONTH": {"target_field": "purchase_date", "confidence": 0.95}, ...},
  "warnings": []
}`

// AISemanticService handles the merged AI call for header detection + column mapping.
type AISemanticService struct {
	ai *openai.Client
}

// NewAISemanticService creates the service.
func NewAISemanticService(ai *openai.Client) *AISemanticService {
	return &AISemanticService{ai: ai}
}

// Analyze performs a single AI call that detects headers, maps columns, and
// identifies data boundaries. Returns nil if AI is unavailable.
func (s *AISemanticService) Analyze(ctx context.Context, rows CellGrid) (*AISemanticResult, error) {
	if s == nil || s.ai == nil {
		return nil, fmt.Errorf("AI client not available")
	}

	// Build the prompt with first 30 + last 20 rows
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Spreadsheet has %d total rows.\n\n", len(rows)))

	headLimit := 30
	if headLimit > len(rows) {
		headLimit = len(rows)
	}
	tailStart := len(rows) - 20
	if tailStart < headLimit {
		// Small sheet — send all rows
		sb.WriteString("ALL ROWS:\n\n")
		for i := 0; i < len(rows); i++ {
			sb.WriteString(fmt.Sprintf("Row %d: %s\n", i, FormatRowForAI(rows[i])))
		}
	} else {
		sb.WriteString(fmt.Sprintf("FIRST %d ROWS (for header detection):\n\n", headLimit))
		for i := 0; i < headLimit; i++ {
			sb.WriteString(fmt.Sprintf("Row %d: %s\n", i, FormatRowForAI(rows[i])))
		}
		sb.WriteString(fmt.Sprintf("\n... (rows %d-%d omitted) ...\n\n", headLimit, tailStart-1))
		sb.WriteString(fmt.Sprintf("LAST %d ROWS (for footer/total detection):\n\n", len(rows)-tailStart))
		for i := tailStart; i < len(rows); i++ {
			sb.WriteString(fmt.Sprintf("Row %d: %s\n", i, FormatRowForAI(rows[i])))
		}
	}

	resp, err := s.ai.ChatCompletion(ctx, []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: mergedSemanticPrompt},
		{Role: oai.ChatMessageRoleUser, Content: sb.String()},
	},
		openai.WithTemperature(0),
		openai.WithMaxTokens(4000),
		openai.WithJSONResponse(),
		openai.WithModel("gpt-4.1-mini"),
	)
	if err != nil {
		return nil, fmt.Errorf("AI semantic analysis API call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("AI returned empty response")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)

	// Strip markdown code fences if present
	if strings.HasPrefix(content, "```") {
		if idx := strings.Index(content, "\n"); idx != -1 {
			content = content[idx+1:]
		}
		if strings.HasSuffix(content, "```") {
			content = content[:len(content)-3]
		}
		content = strings.TrimSpace(content)
	}

	var result AISemanticResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse AI response JSON: %w (raw: %.500s)", err, content)
	}

	// Validate
	if err := validateAIResult(&result, len(rows)); err != nil {
		return nil, fmt.Errorf("AI result validation failed: %w", err)
	}

	slog.Info("AI semantic analysis succeeded",
		"table_type", result.TableType,
		"columns", len(result.Columns),
		"data_range", fmt.Sprintf("[%d, %d]", result.DataStartRow, result.DataEndRow),
		"warnings", result.Warnings,
	)

	return &result, nil
}

// validateAIResult checks the AI output for basic validity.
func validateAIResult(r *AISemanticResult, totalRows int) error {
	if len(r.Columns) == 0 {
		return fmt.Errorf("AI returned zero columns")
	}

	if r.DataStartRow < 0 || r.DataStartRow >= totalRows {
		return fmt.Errorf("invalid data_start_row: %d (total rows: %d)", r.DataStartRow, totalRows)
	}

	if r.DataEndRow <= 0 || r.DataEndRow >= totalRows {
		r.DataEndRow = totalRows - 1
	}
	if r.DataEndRow < r.DataStartRow {
		r.DataEndRow = totalRows - 1
	}

	// Validate table_type
	validTypes := map[string]bool{
		"sales": true, "purchase": true, "bank": true,
		"payroll": true, "ewt": true, "itr": true, "unknown": true,
	}
	if !validTypes[r.TableType] {
		r.TableType = "unknown"
	}

	// Clean up columns
	for i := range r.Columns {
		r.Columns[i] = strings.TrimSpace(r.Columns[i])
	}
	for len(r.Columns) > 0 && r.Columns[len(r.Columns)-1] == "" {
		r.Columns = r.Columns[:len(r.Columns)-1]
	}
	for i, c := range r.Columns {
		if c == "" {
			r.Columns[i] = "Column_" + itoa(i+1)
		}
	}

	// Ensure column_mapping is initialized
	if r.ColumnMapping == nil {
		r.ColumnMapping = make(map[string]FieldMapping)
	}

	return nil
}
