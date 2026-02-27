package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/service/cleaning"
)

const columnMapperSystemPromptPH = `Expert Philippine CPA assistant for BIR tax return filing.
Map spreadsheet columns to official BIR form fields.
Supports: 2550M/2550Q, 1601C, 0619E, 1701, 1702, 2316.

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

BIR_1701 (Annual Income Tax Return — Individuals / Self-Employed / Professionals):

  Taxpayer Info:
    tin, registered_name, trade_name, rdo_code, address, zip_code,
    taxpayer_type, taxable_year

  Gross Income:
    income_date, income_source, or_si_number,
    gross_sales_receipts, sales_returns, net_sales,
    cost_of_sales, gross_income, other_taxable_income,
    compensation_income, total_gross_income

  Deductions:
    expense_date, expense_description, expense_category, expense_amount,
    deduction_method, optional_standard_deduction,
    salaries_wages, rent_expense, depreciation, utilities,
    taxes_licenses, insurance, professional_fees, repairs_maintenance,
    representation, transportation, communication, supplies,
    bad_debts, interest_expense, other_deductions, total_deductions

  Tax Computation:
    taxable_income, income_tax_due

  Tax Credits:
    prior_year_excess_credits, quarterly_tax_payments,
    creditable_withholding_tax, tax_withheld_per_2316,
    foreign_tax_credits, other_tax_credits, total_tax_credits,
    tax_payable, penalty_surcharge, penalty_interest,
    penalty_compromise, total_amount_payable

BIR_1702 (Annual Income Tax Return — Corporations / Partnerships):

  Corporate Info:
    tin, registered_name, trade_name, rdo_code, address, zip_code,
    sec_registration, industry_classification, taxable_year, tax_regime

  Revenue:
    revenue_date, revenue_source, or_si_number,
    gross_sales_receipts, sales_returns, net_sales,
    cost_of_sales, gross_income, other_income, total_gross_income

  Operating Expenses:
    expense_date, expense_description, expense_category, expense_amount,
    salaries_wages, rent_expense, depreciation, utilities,
    taxes_licenses, insurance, professional_fees, repairs_maintenance,
    representation, transportation, communication, supplies,
    bad_debts, interest_expense, charitable_contributions,
    research_development, other_deductions, total_operating_expenses

  Tax Computation:
    net_income_before_tax, taxable_income,
    regular_income_tax, mcit, income_tax_due

  Tax Credits:
    prior_year_excess_credits, excess_mcit, quarterly_tax_payments,
    creditable_withholding_tax, foreign_tax_credits,
    other_tax_credits, total_tax_credits, tax_payable,
    penalty_surcharge, penalty_interest, penalty_compromise,
    total_amount_payable

BIR_2316 (Certificate of Compensation Payment / Tax Withheld):

  Employer Info:
    employer_tin, employer_name, employer_address, employer_zip_code, rdo_code

  Employee Info:
    employee_tin, employee_name, employee_address, employee_zip_code,
    date_of_birth, nationality, civil_status, employment_status,
    date_hired, date_separated

  Compensation:
    basic_salary, overtime_pay, holiday_pay, night_differential,
    hazard_pay, thirteenth_month_pay, other_benefits, gross_compensation

  Non-Taxable:
    nontaxable_13th_month, nontaxable_deminimis,
    sss_gsis_contribution, philhealth_contribution, pagibig_contribution,
    union_dues, other_nontaxable, total_nontaxable

  Tax Computation:
    taxable_compensation, income_tax_due,
    tax_withheld_jan_nov, tax_withheld_december,
    total_tax_withheld, tax_adjustment

  Previous Employer:
    prev_employer_tin, prev_employer_name,
    prev_gross_compensation, prev_nontaxable,
    prev_taxable_compensation, prev_tax_withheld

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

For ITR sheets (BIR 1701/1702 — Income Tax Return):
- "Sales" / "Revenue" / "Receipts" / "Gross Sales" → gross_sales_receipts
- "Cost of Sales" / "COGS" / "Cost of Services" → cost_of_sales
- "Gross Income" / "Gross Profit" → gross_income
- "Net Sales" / "Net Revenue" → net_sales
- "Returns" / "Discounts" / "Allowances" → sales_returns
- "Other Income" / "Non-Operating Income" / "Miscellaneous" → other_taxable_income (1701) or other_income (1702)
- "Compensation" / "Salary Income" → compensation_income
- "Expense" / "Amount" (in expense sheets) → expense_amount
- "Category" / "Account" / "GL Account" → expense_category
- "Salaries" / "Wages" / "Payroll" → salaries_wages
- "Rent" / "Lease" → rent_expense
- "Depreciation" / "Amortization" → depreciation
- "Utilities" / "Light & Water" → utilities
- "Taxes" / "Licenses" / "Permits" → taxes_licenses
- "Insurance" / "Premiums" → insurance
- "Professional Fees" / "Consultant" → professional_fees
- "Repairs" / "Maintenance" → repairs_maintenance
- "Representation" / "Entertainment" → representation
- "Transportation" / "Travel" → transportation
- "Communication" / "Telephone" / "Internet" → communication
- "Supplies" / "Office Supplies" → supplies
- "Bad Debts" / "Doubtful Accounts" → bad_debts
- "Interest" / "Interest Expense" / "Finance Cost" → interest_expense
- "Taxable Income" / "Net Income" → taxable_income
- "Income Tax" / "Tax Due" → income_tax_due
- "CWT" / "Creditable Withholding" / "2307" → creditable_withholding_tax
- "Quarterly Payment" / "1701Q" / "1702Q" → quarterly_tax_payments

For PAYROLL/2316 sheets (BIR 2316 — Certificate of Compensation):
- "Employee" / "Name" / "Employee Name" → employee_name
- "Employee TIN" / "TIN" (of employee) → employee_tin
- "Basic Salary" / "Basic Pay" / "Monthly Salary" → basic_salary
- "Overtime" / "OT Pay" → overtime_pay
- "Holiday" / "Holiday Pay" → holiday_pay
- "Night Diff" / "Night Differential" / "NSD" → night_differential
- "Hazard" / "Hazard Pay" → hazard_pay
- "13th Month" / "13th Month Pay" / "Thirteenth" → thirteenth_month_pay
- "Bonus" / "Other Benefits" / "Allowances" → other_benefits
- "Gross Compensation" / "Gross Pay" / "Total Compensation" → gross_compensation
- "De Minimis" / "Deminimis" → nontaxable_deminimis
- "SSS" / "GSIS" → sss_gsis_contribution
- "PhilHealth" / "PHIC" → philhealth_contribution
- "Pag-IBIG" / "HDMF" / "PAGIBIG" → pagibig_contribution
- "Taxable Compensation" / "Taxable Pay" / "Net Taxable" → taxable_compensation
- "Tax Withheld" / "WHT" / "Withholding Tax" → total_tax_withheld
- "Employer" / "Employer Name" / "Company" → employer_name
- "Employer TIN" → employer_tin

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

When multiple source columns could map to the SAME target field, or a column has confidence < 0.85:
- Still pick the best mapping in "mappings"
- Add "candidates" with up to 3 options per ambiguous column:
  {"column_name": [{"target_field":"...", "confidence":0.8, "reason":"..."}]}
- Add "conflicts" when 2+ source columns compete for the same target:
  [{"target_field":"gross_sales", "columns":["Amount","Gross Amt"]}]

Respond ONLY with valid JSON:
{
  "mappings": {"source_column_name": "target_field_name", ...},
  "unmapped": ["column_names_that_dont_map"],
  "confidence": 0.95,
  "field_confidence": {"source_column_name": 0.95, ...},
  "candidates": {"ambiguous_column": [{"target_field":"...", "confidence":0.8, "reason":"header contains Gross"}]},
  "conflicts": [{"target_field":"gross_sales", "columns":["Amount","Gross Amt"]}]
}

field_confidence: per-column confidence (0.0-1.0) indicating how sure you are about each mapping.
candidates: per-column list of up to 3 possible target fields with confidence and reasoning (only for ambiguous columns).
conflicts: groups of source columns that compete for the same target field.`

const columnMapperSystemPromptSG = `Expert Singapore Chartered Accountant assistant for IRAS tax return filing.
Map spreadsheet columns to official IRAS form fields.
Supports: GST F5, Form C, Form C-S, Form B, IR8A, S45.

=== Target fields by report type ===

IRAS_GST_F5 (GST Return):

  Standard-Rated Supplies (Output):
    supply_date, invoice_number, customer_name, customer_uen,
    standard_rated_supplies, zero_rated_supplies, exempt_supplies,
    total_supplies, output_tax

  Taxable Purchases (Input):
    supplier_name, supplier_uen, purchase_date, purchase_invoice_number,
    taxable_purchases, input_tax_claimable

  GST Computation:
    total_value_of_supplies, total_value_of_gst_on_supplies,
    total_value_of_taxable_purchases, total_value_of_gst_on_purchases,
    net_gst_payable_refundable

  Details:
    uen, registered_name, gst_registration_number, accounting_period,
    filing_due_date

IRAS_FORM_C / IRAS_FORM_CS (Corporate Income Tax Return):

  Income:
    revenue, other_income, total_income, cost_of_goods_sold, gross_profit

  Expenses:
    directors_fees, employee_remuneration, cpf_contributions, rent,
    depreciation, professional_fees, transport, utilities,
    other_expenses, total_expenses

  Tax Computation:
    adjusted_profit, capital_allowances, trade_losses_brought_forward,
    donations, chargeable_income, tax_payable, tax_exempt_amount,
    net_tax_payable

  Details:
    uen, company_name, financial_year_start, financial_year_end,
    revenue_amount

IRAS_FORM_B (Individual Income Tax Return):

  Income:
    employment_income, trade_business_income, rental_income,
    interest_income, dividend_income, other_income, total_income

  Reliefs:
    earned_income_relief, cpf_relief, life_insurance_relief,
    course_fees_relief, parent_relief, spouse_relief, child_relief,
    total_reliefs

  Tax Computation:
    chargeable_income, tax_payable, tax_rebate, net_tax_payable

  Details:
    nric_fin, full_name, date_of_birth, marital_status,
    year_of_assessment

IRAS_IR8A (Return of Employee's Remuneration):

  Employee Info:
    employee_name, nric_fin, designation, date_joined, date_ceased

  Remuneration:
    gross_salary, bonus, director_fees, allowances, benefits_in_kind,
    stock_options, other_remuneration, total_remuneration

  CPF:
    employee_cpf, employer_cpf, total_cpf

  Details:
    employer_name, employer_uen

IRAS_S45 (Withholding Tax on Non-Resident Payments):

  Payee Info:
    payee_name, payee_country, payee_address

  Payment:
    income_type, description, payment_date, payment_amount,
    wht_rate, tax_withheld, net_payment

  Details:
    payer_name, payer_uen, period

Bank_Statement:
  date, description, amount, debit, credit, reference, balance

=== Mapping Rules ===

For SALES/SUPPLY sheets:
- "Revenue" / "Sales" / "Gross Revenue" / "Standard Rated" → standard_rated_supplies
- "Zero Rated" / "Zero-Rated" / "Export Sales" → zero_rated_supplies
- "Exempt" / "GST Exempt" → exempt_supplies
- "Output Tax" / "GST Collected" / "Output GST" → output_tax
- "Date" / "Invoice Date" / "Supply Date" → supply_date
- "Invoice No." / "Tax Invoice" → invoice_number
- "Customer" / "Buyer" / "Client" → customer_name
- "UEN" (of customer) → customer_uen

For PURCHASE sheets:
- "Purchase" / "Total Purchase" / "Taxable Purchases" → taxable_purchases
- "Input Tax" / "GST Paid" / "Input GST" → input_tax_claimable
- "Supplier" / "Vendor" / "Payee" → supplier_name
- "Date" / "Purchase Date" → purchase_date
- "Invoice No." / "Tax Invoice" → purchase_invoice_number
- "Supplier UEN" / "UEN" (of supplier) → supplier_uen

For UEN columns (format like 200012345A or T12LL1234F): map to customer_uen (sales) or supplier_uen (purchases) or uen (general).

For CORPORATE TAX sheets (Form C/C-S):
- "Revenue" / "Turnover" / "Sales" → revenue
- "COGS" / "Cost of Sales" / "Cost of Goods" → cost_of_goods_sold
- "Gross Profit" / "Gross Income" → gross_profit
- "Employee Cost" / "Staff Cost" / "Remuneration" → employee_remuneration
- "CPF" / "CPF Contributions" → cpf_contributions
- "Directors Fees" / "Director Fees" → directors_fees
- "Rent" / "Rental" → rent
- "Depreciation" / "Amortization" → depreciation
- "Capital Allowances" / "CA" → capital_allowances
- "Chargeable Income" / "Taxable Income" → chargeable_income

For PAYROLL/IR8A sheets:
- "Employee" / "Name" / "Employee Name" → employee_name
- "NRIC" / "FIN" / "NRIC/FIN" → nric_fin
- "Salary" / "Gross Salary" / "Basic Pay" → gross_salary
- "Bonus" / "Annual Bonus" → bonus
- "Allowances" / "Transport Allowance" / "Housing" → allowances
- "Benefits" / "BIK" / "Benefits in Kind" → benefits_in_kind
- "Employee CPF" / "CPF Employee" → employee_cpf
- "Employer CPF" / "CPF Employer" → employer_cpf
- "Total Remuneration" / "Gross Pay" → total_remuneration

For S45 WHT sheets:
- "Payee" / "Non-Resident" / "Recipient" → payee_name
- "Country" / "Tax Residence" → payee_country
- "Income Type" / "Nature of Payment" → income_type
- "Payment Amount" / "Gross Payment" → payment_amount
- "WHT Rate" / "Tax Rate" → wht_rate
- "Tax Withheld" / "WHT Amount" → tax_withheld

IMPORTANT: Use field names EXACTLY as listed above. Do not invent new field names.

When multiple source columns could map to the SAME target field, or a column has confidence < 0.85:
- Still pick the best mapping in "mappings"
- Add "candidates" with up to 3 options per ambiguous column:
  {"column_name": [{"target_field":"...", "confidence":0.8, "reason":"..."}]}
- Add "conflicts" when 2+ source columns compete for the same target:
  [{"target_field":"standard_rated_supplies", "columns":["Amount","Revenue"]}]

Respond ONLY with valid JSON:
{
  "mappings": {"source_column_name": "target_field_name", ...},
  "unmapped": ["column_names_that_dont_map"],
  "confidence": 0.95,
  "field_confidence": {"source_column_name": 0.95, ...},
  "candidates": {"ambiguous_column": [{"target_field":"...", "confidence":0.8, "reason":"header contains Revenue"}]},
  "conflicts": [{"target_field":"standard_rated_supplies", "columns":["Amount","Revenue"]}]
}

field_confidence: per-column confidence (0.0-1.0) indicating how sure you are about each mapping.
candidates: per-column list of up to 3 possible target fields with confidence and reasoning (only for ambiguous columns).
conflicts: groups of source columns that compete for the same target field.`

func columnMapperPrompt(jurisdiction string) string {
	if jurisdiction == "SG" {
		return columnMapperSystemPromptSG
	}
	return columnMapperSystemPromptPH
}

// ColumnMapperService handles AI-powered column mapping.
type ColumnMapperService struct {
	ai *openai.Client
}

// NewColumnMapperService creates a column mapper.
func NewColumnMapperService(ai *openai.Client) *ColumnMapperService {
	return &ColumnMapperService{ai: ai}
}

// FieldCandidate represents one possible target field for an ambiguous column.
type FieldCandidate struct {
	TargetField string  `json:"target_field"`
	Confidence  float64 `json:"confidence"`
	Reason      string  `json:"reason"`
}

// ConflictGroup represents multiple source columns competing for the same target field.
type ConflictGroup struct {
	TargetField string   `json:"target_field"`
	Columns     []string `json:"columns"`
}

// ColumnMapping holds the result of column mapping.
type ColumnMapping struct {
	Mappings        map[string]string              `json:"mappings"`
	Unmapped        []string                       `json:"unmapped"`
	Confidence      float64                        `json:"confidence"`
	FieldConfidence map[string]float64             `json:"field_confidence,omitempty"`
	Candidates      map[string][]FieldCandidate    `json:"candidates,omitempty"`
	Conflicts       []ConflictGroup                `json:"conflicts,omitempty"`
}

// AutoMapColumnsOpts holds optional parameters for AutoMapColumns.
type AutoMapColumnsOpts struct {
	DataCategory    string              // sales, purchases, general — helps AI disambiguate
	CorrectionHints []MappingCorrection // past user corrections
	Jurisdiction    string              // "PH" or "SG" — selects field dictionary
}

// AutoMapColumns maps spreadsheet columns to BIR form fields using AI.
func (s *ColumnMapperService) AutoMapColumns(
	ctx context.Context,
	columns []string,
	sampleRows []map[string]interface{},
	reportType string,
	existingMappings map[string]string,
	opts ...AutoMapColumnsOpts,
) (*ColumnMapping, error) {
	var opt AutoMapColumnsOpts
	if len(opts) > 0 {
		opt = opts[0]
	}
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

	// Phase 5: Try fuzzy matching against saved template before calling AI
	if len(existingMappings) > 0 {
		fuzzyMatched, fuzzyConf := FuzzyMatchSavedMappings(columns, existingMappings)
		if len(fuzzyMatched) > 0 {
			coverage := float64(len(fuzzyMatched)) / float64(len(columns))
			totalConf := 0.0
			for _, c := range fuzzyConf {
				totalConf += c
			}
			avgConf := totalConf / float64(len(fuzzyConf))

			// If fuzzy match covers >80% columns with avg confidence >0.85, use directly
			if coverage > 0.80 && avgConf > 0.85 {
				var unmapped []string
				for _, col := range columns {
					if _, ok := fuzzyMatched[col]; !ok {
						unmapped = append(unmapped, col)
					}
				}
				return &ColumnMapping{
					Mappings:        fuzzyMatched,
					Unmapped:        unmapped,
					Confidence:      avgConf,
					FieldConfidence: fuzzyConf,
				}, nil
			}
		}
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

	// Data category hint — critical for disambiguating Sales vs Purchases
	if opt.DataCategory != "" {
		switch strings.ToLower(opt.DataCategory) {
		case "purchases":
			userPrompt += "\n\nIMPORTANT: This data contains PURCHASE transactions (SLP — Summary List of Purchases). " +
				"Map amount columns to purchase fields (gross_purchase, input_tax, etc.), NOT to sales fields. " +
				"Map party columns to supplier fields (supplier_name, supplier_tin), NOT customer fields."
		case "sales":
			userPrompt += "\n\nIMPORTANT: This data contains SALES transactions (SLS — Summary List of Sales). " +
				"Map amount columns to sales fields (gross_sales, vatable_sales, output_tax, etc.), NOT to purchase fields. " +
				"Map party columns to customer fields (customer_name, customer_tin), NOT supplier fields."
		}
	}

	if len(existingMappings) > 0 {
		existingJSON, _ := json.Marshal(existingMappings)
		userPrompt += fmt.Sprintf("\n\nPrevious mappings (prefer reusing): %s", string(existingJSON))
	}

	// Phase 4: Append correction hints if available
	if len(opt.CorrectionHints) > 0 {
		userPrompt += buildCorrectionHint(opt.CorrectionHints)
	}

	resp, err := s.ai.ChatCompletion(ctx, []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: columnMapperPrompt(opt.Jurisdiction)},
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

	// Programmatic conflict detection: scan for duplicate target fields
	result.Conflicts = detectConflicts(result.Mappings, result.Conflicts)

	return &result, nil
}

// detectConflicts scans mappings for duplicate target fields and merges with
// any AI-reported conflicts. This ensures conflicts are always detected even
// if the AI fails to report them.
func detectConflicts(mappings map[string]string, existing []ConflictGroup) []ConflictGroup {
	// Build target → columns index
	targetCols := make(map[string][]string)
	for col, target := range mappings {
		if target == "" || target == "_skip" {
			continue
		}
		targetCols[target] = append(targetCols[target], col)
	}

	// Merge with existing conflict set (keyed by target field)
	conflictMap := make(map[string]ConflictGroup)
	for _, cg := range existing {
		conflictMap[cg.TargetField] = cg
	}

	for target, cols := range targetCols {
		if len(cols) < 2 {
			continue
		}
		if _, ok := conflictMap[target]; !ok {
			conflictMap[target] = ConflictGroup{TargetField: target, Columns: cols}
		} else {
			// Merge: union the column lists
			seen := make(map[string]bool)
			merged := make([]string, 0)
			for _, c := range conflictMap[target].Columns {
				if !seen[c] {
					seen[c] = true
					merged = append(merged, c)
				}
			}
			for _, c := range cols {
				if !seen[c] {
					seen[c] = true
					merged = append(merged, c)
				}
			}
			conflictMap[target] = ConflictGroup{TargetField: target, Columns: merged}
		}
	}

	if len(conflictMap) == 0 {
		return nil
	}

	conflicts := make([]ConflictGroup, 0, len(conflictMap))
	for _, cg := range conflictMap {
		conflicts = append(conflicts, cg)
	}
	return conflicts
}

func allColumnsMapped(columns []string, mappings map[string]string) bool {
	for _, col := range columns {
		if _, ok := mappings[col]; !ok {
			return false
		}
	}
	return true
}

// abbreviations maps common abbreviations to their full forms for fuzzy matching.
var abbreviations = map[string]string{
	"amt":   "amount",
	"inv":   "invoice",
	"no":    "number",
	"no.":   "number",
	"num":   "number",
	"addr":  "address",
	"desc":  "description",
	"qty":   "quantity",
	"dt":    "date",
	"val":   "value",
	"tot":   "total",
	"pmt":   "payment",
	"txn":   "transaction",
	"acct":  "account",
	"bal":   "balance",
	"ref":   "reference",
	// Filipino abbreviations
	"hal":   "halaga",   // Amount
	"pet":   "petsa",    // Date
	"bil":   "bilang",   // Count/Number
	"kab":   "kabuuan",  // Total
}

// normalizeColumn normalizes a column name for comparison: lowercase, trim, collapse whitespace.
func normalizeColumn(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Collapse multiple spaces/underscores to single space
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '_' || r == '-'
	})
	return strings.Join(parts, " ")
}

// expandAbbreviations replaces known abbreviations in a normalized string.
func expandAbbreviations(s string) string {
	words := strings.Fields(s)
	expanded := make([]string, len(words))
	for i, w := range words {
		if full, ok := abbreviations[w]; ok {
			expanded[i] = full
		} else {
			// Strip trailing period for abbreviation lookup
			trimmed := strings.TrimRight(w, ".")
			if full, ok := abbreviations[trimmed]; ok {
				expanded[i] = full
			} else {
				expanded[i] = w
			}
		}
	}
	return strings.Join(expanded, " ")
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 { return lb }
	if lb == 0 { return la }

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			min := del
			if ins < min { min = ins }
			if sub < min { min = sub }
			curr[j] = min
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// FuzzyMatchSavedMappings attempts to match current columns to saved template
// mappings using fuzzy matching. Returns matched mappings and per-field confidence.
func FuzzyMatchSavedMappings(
	currentColumns []string,
	savedMappings map[string]string,
) (matched map[string]string, confidence map[string]float64) {
	matched = make(map[string]string)
	confidence = make(map[string]float64)

	if len(savedMappings) == 0 {
		return
	}

	// Build normalized index of saved columns
	type savedEntry struct {
		original string
		norm     string
		expanded string
		target   string
	}
	saved := make([]savedEntry, 0, len(savedMappings))
	for col, target := range savedMappings {
		norm := normalizeColumn(col)
		saved = append(saved, savedEntry{
			original: col,
			norm:     norm,
			expanded: expandAbbreviations(norm),
			target:   target,
		})
	}

	for _, col := range currentColumns {
		norm := normalizeColumn(col)
		expanded := expandAbbreviations(norm)

		var bestTarget string
		var bestConf float64

		for _, s := range saved {
			// Level 1: Exact match (after normalization)
			if norm == s.norm {
				bestTarget = s.target
				bestConf = 1.0
				break
			}

			// Level 2: Case + whitespace normalized match
			if norm == s.norm {
				if 0.95 > bestConf {
					bestTarget = s.target
					bestConf = 0.95
				}
				continue
			}

			// Level 3: Abbreviation expansion match
			if expanded == s.expanded && expanded != "" {
				if 0.75 > bestConf {
					bestTarget = s.target
					bestConf = 0.75
				}
				continue
			}

			// Level 4: Levenshtein distance ≤ 3
			dist := levenshtein(norm, s.norm)
			if dist <= 3 && dist > 0 {
				conf := 0.80 - float64(dist)*0.05
				if conf > bestConf {
					bestTarget = s.target
					bestConf = conf
				}
			}
		}

		if bestTarget != "" && bestConf > 0 {
			matched[col] = bestTarget
			confidence[col] = bestConf
		}
	}

	return
}

// MappingCorrection represents a user correction to an AI column mapping.
type MappingCorrection struct {
	ColumnName   string        `json:"column_name"`
	OldTarget    string        `json:"old_target"`
	NewTarget    string        `json:"new_target"`
	SampleValues []interface{} `json:"sample_values,omitempty"`
}

// buildCorrectionHint formats past correction records into a prompt hint for the AI.
func buildCorrectionHint(corrections []MappingCorrection) string {
	if len(corrections) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\nUser previously corrected these column mappings (prefer these):\n")
	for _, c := range corrections {
		sb.WriteString(fmt.Sprintf("- \"%s\" should map to \"%s\"", c.ColumnName, c.NewTarget))
		if c.OldTarget != "" {
			sb.WriteString(fmt.Sprintf(" (not \"%s\")", c.OldTarget))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// AutoMapColumnsFromCleaning converts cleaning pipeline FieldMappings into a
// ColumnMapping result compatible with the existing API.
func AutoMapColumnsFromCleaning(cleaningMapping map[string]cleaning.FieldMapping, columns []string) *ColumnMapping {
	if len(cleaningMapping) == 0 {
		return &ColumnMapping{
			Mappings:   map[string]string{},
			Unmapped:   columns,
			Confidence: 0,
		}
	}

	mappings := make(map[string]string, len(cleaningMapping))
	fieldConf := make(map[string]float64, len(cleaningMapping))
	var unmapped []string
	totalConf := 0.0
	mappedCount := 0

	for _, col := range columns {
		fm, ok := cleaningMapping[col]
		if ok && fm.TargetField != "" {
			mappings[col] = fm.TargetField
			fieldConf[col] = fm.Confidence
			totalConf += fm.Confidence
			mappedCount++
		} else {
			unmapped = append(unmapped, col)
		}
	}

	confidence := 0.0
	if mappedCount > 0 {
		confidence = totalConf / float64(mappedCount)
	}

	return &ColumnMapping{
		Mappings:        mappings,
		Unmapped:        unmapped,
		Confidence:      confidence,
		FieldConfidence: fieldConf,
	}
}
