package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/birforms"
)

// GLTaxBridge auto-populates BIR tax calculators from GL balances.
type GLTaxBridge struct {
	q   *sqlc.Queries
	fs  *FinancialStatementService
}

// NewGLTaxBridge creates a GLTaxBridge.
func NewGLTaxBridge(q *sqlc.Queries, fs *FinancialStatementService) *GLTaxBridge {
	return &GLTaxBridge{q: q, fs: fs}
}

// TaxCalculationResult wraps the tax result with metadata.
type TaxCalculationResult struct {
	FormType    string    `json:"form_type"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	Result      TaxResult `json:"result"`
}

// Calculate2550M computes Monthly VAT from GL balances.
func (b *GLTaxBridge) Calculate2550M(ctx context.Context, companyID uuid.UUID, periodStart, periodEnd time.Time) (*TaxCalculationResult, error) {
	// Revenue accounts by VAT classification
	_, vatableSales, err := b.fs.PeriodBalancesByPrefix(ctx, companyID, "4000", periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("vatable sales: %w", err)
	}
	// Also include service revenue vatable
	_, vatableSvcSales, err := b.fs.PeriodBalancesByPrefix(ctx, companyID, "4100", periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("vatable service sales: %w", err)
	}
	vatableSales = vatableSales.Add(vatableSvcSales)

	_, zeroRatedSales, err := b.fs.PeriodBalancesByPrefix(ctx, companyID, "4010", periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("zero-rated sales: %w", err)
	}
	// Also include zero-rated service
	_, zeroRatedSvcSales, err := b.fs.PeriodBalancesByPrefix(ctx, companyID, "4110", periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("zero-rated service sales: %w", err)
	}
	zeroRatedSales = zeroRatedSales.Add(zeroRatedSvcSales)

	_, exemptSales, err := b.fs.PeriodBalancesByPrefix(ctx, companyID, "4020", periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("exempt sales: %w", err)
	}

	// Output VAT (liability 2200)
	_, outputVAT, err := b.fs.PeriodBalancesByPrefix(ctx, companyID, "2200", periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("output VAT: %w", err)
	}

	// Input VAT (asset 1400)
	_, inputVAT, err := b.fs.PeriodBalancesByPrefix(ctx, companyID, "1400", periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("input VAT: %w", err)
	}

	// Purchases by category (COGS 5xxx)
	_, domesticPurchases, err := b.fs.PeriodBalancesByPrefix(ctx, companyID, "5", periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("domestic purchases: %w", err)
	}

	// Build tax engine input
	input := map[string]interface{}{
		"sales_data": []map[string]interface{}{
			{"amount": vatableSales.String(), "vat_type": "vatable"},
			{"amount": zeroRatedSales.String(), "vat_type": "zero_rated"},
			{"amount": exemptSales.String(), "vat_type": "exempt"},
		},
		"purchases_data": []map[string]interface{}{
			{"amount": domesticPurchases.String(), "vat_amount": inputVAT.String(), "category": "goods"},
		},
		"tax_credits": "0",
		"penalties":   "0",
	}

	result, err := CalculateReport(birforms.FormBIR2550M, input)
	if err != nil {
		return nil, fmt.Errorf("calculate 2550M: %w", err)
	}

	// Override with actual GL values for precision
	result["gl_output_vat"] = outputVAT.String()
	result["gl_input_vat"] = inputVAT.String()
	result["gl_source"] = "true"

	return &TaxCalculationResult{
		FormType:    birforms.FormBIR2550M,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Result:      result,
	}, nil
}

// Calculate0619E computes Monthly Expanded Withholding Tax from GL.
func (b *GLTaxBridge) Calculate0619E(ctx context.Context, companyID uuid.UUID, periodStart, periodEnd time.Time) (*TaxCalculationResult, error) {
	// EWT Payable (liability 2210)
	_, totalEWT, err := b.fs.PeriodBalancesByPrefix(ctx, companyID, "2210", periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("EWT payable: %w", err)
	}

	// Related expense accounts (6xxx)
	_, totalExpenses, err := b.fs.PeriodBalancesByPrefix(ctx, companyID, "6", periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("expenses: %w", err)
	}

	input := map[string]interface{}{
		"ewt_data": map[string]interface{}{
			"total_income_payments": totalExpenses.String(),
			"total_taxes_withheld":  totalEWT.String(),
			"adjustment":            "0",
			"surcharge":             "0",
			"interest":              "0",
			"compromise":            "0",
		},
	}

	result, err := CalculateReport(birforms.FormBIR0619E, input)
	if err != nil {
		return nil, fmt.Errorf("calculate 0619E: %w", err)
	}

	result["gl_source"] = "true"

	return &TaxCalculationResult{
		FormType:    birforms.FormBIR0619E,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Result:      result,
	}, nil
}

// Calculate1701 computes Annual Individual Income Tax from GL.
func (b *GLTaxBridge) Calculate1701(ctx context.Context, companyID uuid.UUID, year int) (*TaxCalculationResult, error) {
	startDate := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)

	is, err := b.fs.IncomeStatement(ctx, companyID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("income statement: %w", err)
	}

	// CWT from GL (asset 1410)
	_, cwt, err := b.fs.AccountBalancesByPrefix(ctx, companyID, "1410", endDate)
	if err != nil {
		return nil, fmt.Errorf("CWT: %w", err)
	}

	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts":       is.TotalRevenue.String(),
			"cost_of_sales":              is.TotalCOGS.String(),
			"other_taxable_income":       "0",
			"deduction_method":           "itemized",
			"itemized_deductions":        is.TotalExpenses.String(),
			"creditable_withholding_tax": cwt.String(),
			"quarterly_payments":         "0",
			"other_credits":              "0",
			"surcharge":                  "0",
			"interest":                   "0",
			"compromise":                 "0",
		},
	}

	result, err := CalculateReport(birforms.FormBIR1701, input)
	if err != nil {
		return nil, fmt.Errorf("calculate 1701: %w", err)
	}

	result["gl_source"] = "true"

	return &TaxCalculationResult{
		FormType:    birforms.FormBIR1701,
		PeriodStart: startDate,
		PeriodEnd:   endDate,
		Result:      result,
	}, nil
}

// Calculate1702 computes Annual Corporate Income Tax from GL.
func (b *GLTaxBridge) Calculate1702(ctx context.Context, companyID uuid.UUID, year int) (*TaxCalculationResult, error) {
	startDate := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)

	is, err := b.fs.IncomeStatement(ctx, companyID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("income statement: %w", err)
	}

	// CWT from GL (asset 1410)
	_, cwt, err := b.fs.AccountBalancesByPrefix(ctx, companyID, "1410", endDate)
	if err != nil {
		return nil, fmt.Errorf("CWT: %w", err)
	}

	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":               is.TotalRevenue.String(),
			"cost_of_sales":              is.TotalCOGS.String(),
			"other_income":               "0",
			"deduction_method":           "itemized",
			"itemized_deductions":        is.TotalExpenses.String(),
			"excess_mcit_prior":          "0",
			"creditable_withholding_tax": cwt.String(),
			"quarterly_payments":         "0",
			"other_credits":              "0",
			"surcharge":                  "0",
			"interest":                   "0",
			"compromise":                 "0",
			"is_sme":                     false,
		},
	}

	result, err := CalculateReport(birforms.FormBIR1702, input)
	if err != nil {
		return nil, fmt.Errorf("calculate 1702: %w", err)
	}

	result["gl_source"] = "true"

	return &TaxCalculationResult{
		FormType:    birforms.FormBIR1702,
		PeriodStart: startDate,
		PeriodEnd:   endDate,
		Result:      result,
	}, nil
}

// CalculateFromGL dispatches to the appropriate tax bridge method.
func (b *GLTaxBridge) CalculateFromGL(ctx context.Context, companyID uuid.UUID, formType string, periodStart, periodEnd time.Time) (*TaxCalculationResult, error) {
	switch formType {
	case birforms.FormBIR2550M, birforms.FormBIR2550Q:
		return b.Calculate2550M(ctx, companyID, periodStart, periodEnd)
	case birforms.FormBIR0619E:
		return b.Calculate0619E(ctx, companyID, periodStart, periodEnd)
	case birforms.FormBIR1701:
		return b.Calculate1701(ctx, companyID, periodStart.Year())
	case birforms.FormBIR1702:
		return b.Calculate1702(ctx, companyID, periodStart.Year())
	default:
		return nil, fmt.Errorf("GL-to-tax bridge not available for form type: %s", formType)
	}
}

// Ensure decimal is used
var _ = decimal.Zero
