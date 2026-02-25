package service

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/pkg/birforms"
)

// SalesRow represents a single sales record for VAT calculation.
type SalesRow struct {
	Amount  decimal.Decimal `json:"amount"`
	VATType string          `json:"vat_type"` // vatable, government, zero_rated, exempt
}

// PurchaseRow represents a single purchase record for VAT calculation.
type PurchaseRow struct {
	Amount    decimal.Decimal `json:"amount"`
	VATAmount decimal.Decimal `json:"vat_amount"`
	Category  string          `json:"category"` // goods, capital, services, imports
}

// TaxResult holds key-value calculation results. All values are decimal strings.
type TaxResult map[string]string

// CalculateReport dispatches to the appropriate BIR form calculator.
func CalculateReport(formType string, input map[string]interface{}) (TaxResult, error) {
	switch formType {
	case birforms.FormBIR2550M:
		return CalculateBIR2550M(input)
	case birforms.FormBIR2550Q:
		return CalculateBIR2550Q(input)
	case birforms.FormBIR1601C:
		return CalculateBIR1601C(input)
	case birforms.FormBIR0619E:
		return CalculateBIR0619E(input)
	case birforms.FormBIR1701:
		return CalculateBIR1701(input)
	case birforms.FormBIR1702:
		return CalculateBIR1702(input)
	case birforms.FormBIR2316:
		return CalculateBIR2316(input)
	default:
		return nil, fmt.Errorf("no calculator available for %s", formType)
	}
}

// CalculateBIR2550M computes Monthly VAT (BIR 2550M).
//
// Follows the actual BIR 2550M form structure:
//
//	Part II  - Sales/Receipts (Lines 1-5)
//	Part III - Output Tax (Lines 6, 6A, 6B)
//	Part IV  - Input Tax (Lines 7-11)
//	Part V   - Tax Due (Lines 12-16)
func CalculateBIR2550M(input map[string]interface{}) (TaxResult, error) {
	salesData := toMapSlice(input["sales_data"])
	purchasesData := toMapSlice(input["purchases_data"])
	taxCredits := toDecimal(input["tax_credits"])
	penalties := toDecimal(input["penalties"])

	// --- Check if input is a pre-aggregated VATSummary (from session) ---
	// If sales_data is empty but we have VATSummary fields, use those directly.
	hasSummaryFields := !toDecimal(input["vatable_sales"]).IsZero() ||
		!toDecimal(input["total_sales"]).IsZero() ||
		!toDecimal(input["output_vat"]).IsZero() ||
		!toDecimal(input["total_output_vat"]).IsZero() ||
		!toDecimal(input["total_input_vat"]).IsZero()

	var vatableSales, salesToGovernment, zeroRatedSales, vatExemptSales decimal.Decimal
	var totalSales decimal.Decimal
	var outputVAT, outputVATGovt, totalOutputVAT decimal.Decimal
	var inputVATGoods, inputVATCapital, inputVATServices, inputVATImports decimal.Decimal
	var totalInputVAT decimal.Decimal

	if len(salesData) == 0 && len(purchasesData) == 0 && hasSummaryFields {
		// Use pre-aggregated VATSummary fields directly
		vatableSales = toDecimal(input["vatable_sales"])
		salesToGovernment = toDecimal(input["sales_to_government"])
		zeroRatedSales = toDecimal(input["zero_rated_sales"])
		vatExemptSales = toDecimal(input["vat_exempt_sales"])
		totalSales = toDecimal(input["total_sales"])
		if totalSales.IsZero() {
			totalSales = vatableSales.Add(salesToGovernment).Add(zeroRatedSales).Add(vatExemptSales)
		}

		outputVAT = toDecimal(input["output_vat"])
		outputVATGovt = toDecimal(input["output_vat_government"])
		totalOutputVAT = toDecimal(input["total_output_vat"])
		if totalOutputVAT.IsZero() {
			totalOutputVAT = outputVAT.Add(outputVATGovt)
		}

		inputVATGoods = toDecimal(input["input_vat_goods"])
		inputVATCapital = toDecimal(input["input_vat_capital"])
		inputVATServices = toDecimal(input["input_vat_services"])
		inputVATImports = toDecimal(input["input_vat_imports"])
		totalInputVAT = toDecimal(input["total_input_vat"])
		if totalInputVAT.IsZero() {
			totalInputVAT = inputVATGoods.Add(inputVATCapital).Add(inputVATServices).Add(inputVATImports)
		}
	} else {
		// Calculate from raw sales_data / purchases_data rows

		// --- Part II: Sales Classification ---
		for _, row := range salesData {
			amount := toDecimal(row["amount"])
			vatType := strings.ToLower(strings.TrimSpace(toString(row["vat_type"])))
			if vatType == "" {
				vatType = "vatable"
			}
			switch vatType {
			case "government":
				salesToGovernment = salesToGovernment.Add(amount)
			case "zero_rated":
				zeroRatedSales = zeroRatedSales.Add(amount)
			case "exempt":
				vatExemptSales = vatExemptSales.Add(amount)
			default:
				vatableSales = vatableSales.Add(amount)
			}
		}

		totalSales = vatableSales.Add(salesToGovernment).Add(zeroRatedSales).Add(vatExemptSales)

		// --- Part III: Output Tax ---
		outputVAT = vatableSales.Mul(birforms.VATRate)
		outputVATGovt = salesToGovernment.Mul(birforms.GovtVATRate)
		totalOutputVAT = outputVAT.Add(outputVATGovt)

		// --- Part IV: Input Tax ---
		for _, row := range purchasesData {
			amount := toDecimal(row["amount"])
			vatAmount := toDecimal(row["vat_amount"])
			category := strings.ToLower(strings.TrimSpace(toString(row["category"])))
			if category == "" {
				category = "goods"
			}

			inputVAT := vatAmount
			if inputVAT.IsZero() {
				inputVAT = amount.Mul(birforms.VATRate)
			}

			switch category {
			case "capital":
				inputVATCapital = inputVATCapital.Add(inputVAT)
			case "services":
				inputVATServices = inputVATServices.Add(inputVAT)
			case "imports":
				inputVATImports = inputVATImports.Add(inputVAT)
			default:
				inputVATGoods = inputVATGoods.Add(inputVAT)
			}
		}

		totalInputVAT = inputVATGoods.Add(inputVATCapital).Add(inputVATServices).Add(inputVATImports)
	}

	// --- Part V: Tax Due ---
	vatPayable := totalOutputVAT.Sub(totalInputVAT)
	netVATPayableRaw := vatPayable.Sub(taxCredits)
	netVATPayable := decMax(netVATPayableRaw, decimal.Zero)
	taxCreditCarriedForward := decMax(netVATPayableRaw.Neg(), decimal.Zero)
	totalAmountDue := netVATPayable.Add(penalties)

	return TaxResult{
		// Part II - Sales
		"line_1_vatable_sales":        vatableSales.String(),
		"line_2_sales_to_government":  salesToGovernment.String(),
		"line_3_zero_rated_sales":     zeroRatedSales.String(),
		"line_4_exempt_sales":         vatExemptSales.String(),
		"line_5_total_sales":          totalSales.String(),
		// Part III - Output Tax
		"line_6_output_vat":            outputVAT.String(),
		"line_6a_output_vat_government": outputVATGovt.String(),
		"line_6b_total_output_vat":     totalOutputVAT.String(),
		// Part IV - Input Tax
		"line_7_input_vat_goods":    inputVATGoods.String(),
		"line_8_input_vat_capital":  inputVATCapital.String(),
		"line_9_input_vat_services": inputVATServices.String(),
		"line_10_input_vat_imports": inputVATImports.String(),
		"line_11_total_input_vat":   totalInputVAT.String(),
		// Part V - Tax Due
		"line_12_vat_payable":       vatPayable.String(),
		"line_13_less_tax_credits":  taxCredits.String(),
		"line_14_net_vat_payable":   netVATPayable.String(),
		"line_15_add_penalties":     penalties.String(),
		"line_16_total_amount_due":  totalAmountDue.String(),
		"tax_credit_carried_forward": taxCreditCarriedForward.String(),
		// Legacy compatibility keys
		"vatable_sales":     vatableSales.String(),
		"vat_exempt_sales":  vatExemptSales.String(),
		"zero_rated_sales":  zeroRatedSales.String(),
		"total_sales":       totalSales.String(),
		"output_vat":        outputVAT.String(),
		"input_vat_goods":   inputVATGoods.String(),
		"input_vat_services": inputVATServices.String(),
		"input_vat_capital": inputVATCapital.String(),
		"total_input_vat":   totalInputVAT.String(),
		"vat_payable":       vatPayable.String(),
		"net_vat_payable":   netVATPayable.String(),
	}, nil
}

// CalculateBIR2550Q computes Quarterly VAT (BIR 2550Q).
// Same calculation as 2550M; only form metadata differs.
func CalculateBIR2550Q(input map[string]interface{}) (TaxResult, error) {
	result, err := CalculateBIR2550M(input)
	if err != nil {
		return nil, err
	}
	result["form_type"] = birforms.FormBIR2550Q
	return result, nil
}

// CalculateBIR1601C computes Monthly Withholding Tax on Compensation (BIR 1601-C).
func CalculateBIR1601C(input map[string]interface{}) (TaxResult, error) {
	compData := toMap(input["compensation_data"])
	if compData == nil {
		compData = input
	}

	totalComp := toDecimal(compData["total_compensation"])
	minWage := toDecimal(compData["statutory_minimum_wage"])
	thirteenth := toDecimal(compData["nontaxable_13th_month"])
	deminimis := toDecimal(compData["nontaxable_deminimis"])
	sssGSIS := toDecimal(compData["sss_gsis_phic_hdmf"])
	otherNT := toDecimal(compData["other_nontaxable"])
	taxWithheld := toDecimal(compData["tax_withheld"])
	adjustment := toDecimal(compData["adjustment"])
	surcharge := toDecimal(compData["surcharge"])
	interest := toDecimal(compData["interest"])
	compromise := toDecimal(compData["compromise"])

	totalNontaxable := minWage.Add(thirteenth).Add(deminimis).Add(sssGSIS).Add(otherNT)
	taxableComp := decMax(totalComp.Sub(totalNontaxable), decimal.Zero)
	totalTaxRemitted := taxWithheld.Add(adjustment)
	totalPenalties := surcharge.Add(interest).Add(compromise)
	totalDue := totalTaxRemitted.Add(totalPenalties)

	return TaxResult{
		"line_1_total_compensation":   totalComp.String(),
		"line_2_statutory_minimum_wage": minWage.String(),
		"line_3_nontaxable_13th_month": thirteenth.String(),
		"line_4_nontaxable_deminimis": deminimis.String(),
		"line_5_sss_gsis_phic_hdmf":   sssGSIS.String(),
		"line_6_other_nontaxable":     otherNT.String(),
		"line_7_total_nontaxable":     totalNontaxable.String(),
		"line_8_taxable_compensation": taxableComp.String(),
		"line_9_tax_withheld":         taxWithheld.String(),
		"line_10_adjustment":          adjustment.String(),
		"line_11_total_tax_remitted":  totalTaxRemitted.String(),
		"line_12_surcharge":           surcharge.String(),
		"line_13_interest":            interest.String(),
		"line_14_compromise":          compromise.String(),
		"line_15_total_penalties":     totalPenalties.String(),
		"line_16_total_amount_due":    totalDue.String(),
	}, nil
}

// CalculateBIR0619E computes Monthly Expanded Withholding Tax (BIR 0619-E).
func CalculateBIR0619E(input map[string]interface{}) (TaxResult, error) {
	ewtData := toMap(input["ewt_data"])
	if ewtData == nil {
		ewtData = input
	}

	totalIncome := toDecimal(ewtData["total_income_payments"])
	totalWithheld := toDecimal(ewtData["total_taxes_withheld"])
	adjustment := toDecimal(ewtData["adjustment"])
	surcharge := toDecimal(ewtData["surcharge"])
	interest := toDecimal(ewtData["interest"])
	compromise := toDecimal(ewtData["compromise"])

	taxStillDue := decMax(totalWithheld.Sub(adjustment), decimal.Zero)
	totalPenalties := surcharge.Add(interest).Add(compromise)
	totalDue := taxStillDue.Add(totalPenalties)

	return TaxResult{
		"line_1_total_amount_of_income_payments": totalIncome.String(),
		"line_2_total_taxes_withheld":            totalWithheld.String(),
		"line_3_adjustment":                      adjustment.String(),
		"line_4_tax_still_due":                   taxStillDue.String(),
		"line_5_surcharge":                       surcharge.String(),
		"line_6_interest":                        interest.String(),
		"line_7_compromise":                      compromise.String(),
		"line_8_total_penalties":                  totalPenalties.String(),
		"line_9_total_amount_due":                totalDue.String(),
	}, nil
}

// CalculateBIR1701 computes Annual Individual Income Tax (BIR 1701).
// Supports business/professional income, OSD or itemized deductions, TRAIN Law graduated rates.
func CalculateBIR1701(input map[string]interface{}) (TaxResult, error) {
	incomeData := toMap(input["income_data"])
	if incomeData == nil {
		incomeData = input
	}

	grossSales := toDecimal(incomeData["gross_sales_receipts"])
	costOfSales := toDecimal(incomeData["cost_of_sales"])
	otherIncome := toDecimal(incomeData["other_taxable_income"])
	deductionMethod := strings.ToLower(toString(incomeData["deduction_method"]))
	if deductionMethod == "" {
		deductionMethod = "osd"
	}
	itemized := toDecimal(incomeData["itemized_deductions"])
	cwt := toDecimal(incomeData["creditable_withholding_tax"])
	quarterlyPayments := toDecimal(incomeData["quarterly_payments"])
	otherCredits := toDecimal(incomeData["other_credits"])
	surcharge := toDecimal(incomeData["surcharge"])
	interest := toDecimal(incomeData["interest"])
	compromise := toDecimal(incomeData["compromise"])

	grossIncomeBiz := decMax(grossSales.Sub(costOfSales), decimal.Zero)
	totalGross := grossIncomeBiz.Add(otherIncome)

	var totalDeductions, osdAmount decimal.Decimal
	if deductionMethod == "itemized" {
		totalDeductions = itemized
		osdAmount = decimal.Zero
	} else {
		// OSD = 40% of gross sales/receipts
		osdAmount = grossSales.Mul(decimal.NewFromFloat(0.40))
		totalDeductions = osdAmount
	}

	netTaxable := decMax(totalGross.Sub(totalDeductions), decimal.Zero)
	taxDue := computeGraduatedTax(netTaxable)

	totalCredits := cwt.Add(quarterlyPayments).Add(otherCredits)
	taxPayable := decMax(taxDue.Sub(totalCredits), decimal.Zero)
	totalPenalties := surcharge.Add(interest).Add(compromise)
	totalDue := taxPayable.Add(totalPenalties)

	return TaxResult{
		"gross_sales_receipts":       grossSales.String(),
		"cost_of_sales":              costOfSales.String(),
		"gross_income_from_business": grossIncomeBiz.String(),
		"other_taxable_income":       otherIncome.String(),
		"total_gross_income":         totalGross.String(),
		"deduction_method":           deductionMethod,
		"itemized_deductions":        itemized.String(),
		"osd_amount":                 osdAmount.String(),
		"total_deductions":           totalDeductions.String(),
		"net_taxable_income":         netTaxable.String(),
		"income_tax_due":             taxDue.String(),
		"creditable_withholding_tax": cwt.String(),
		"quarterly_payments":         quarterlyPayments.String(),
		"other_credits":              otherCredits.String(),
		"total_tax_credits":          totalCredits.String(),
		"tax_payable":                taxPayable.String(),
		"surcharge":                  surcharge.String(),
		"interest":                   interest.String(),
		"compromise":                 compromise.String(),
		"total_penalties":            totalPenalties.String(),
		"total_amount_due":           totalDue.String(),
	}, nil
}

// CalculateBIR1702 computes Annual Corporate Income Tax (BIR 1702).
// Supports RCIT (25%/20%) vs MCIT (1%) comparison, OSD or itemized, excess MCIT carryforward.
func CalculateBIR1702(input map[string]interface{}) (TaxResult, error) {
	incomeData := toMap(input["income_data"])
	if incomeData == nil {
		incomeData = input
	}

	grossIncome := toDecimal(incomeData["gross_income"])
	costOfSales := toDecimal(incomeData["cost_of_sales"])
	otherIncome := toDecimal(incomeData["other_income"])
	deductionMethod := strings.ToLower(toString(incomeData["deduction_method"]))
	if deductionMethod == "" {
		deductionMethod = "itemized"
	}
	itemized := toDecimal(incomeData["itemized_deductions"])
	excessMCITPrior := toDecimal(incomeData["excess_mcit_prior"])
	cwt := toDecimal(incomeData["creditable_withholding_tax"])
	quarterlyPayments := toDecimal(incomeData["quarterly_payments"])
	otherCredits := toDecimal(incomeData["other_credits"])
	surcharge := toDecimal(incomeData["surcharge"])
	interest := toDecimal(incomeData["interest"])
	compromise := toDecimal(incomeData["compromise"])
	isSME := toBool(incomeData["is_sme"])

	grossProfit := decMax(grossIncome.Sub(costOfSales), decimal.Zero)
	totalGross := grossProfit.Add(otherIncome)

	var totalDeductions, osdAmount decimal.Decimal
	if deductionMethod == "osd" {
		osdAmount = grossIncome.Mul(decimal.NewFromFloat(0.40))
		totalDeductions = osdAmount
	} else {
		osdAmount = decimal.Zero
		totalDeductions = itemized
	}

	netTaxable := decMax(totalGross.Sub(totalDeductions), decimal.Zero)

	// RCIT
	rcitRate := birforms.RCIT
	if isSME {
		rcitRate = birforms.RCITReduced
	}
	rcitAmount := netTaxable.Mul(rcitRate)

	// MCIT (1% of gross income)
	mcitBase := grossIncome
	mcitAmount := mcitBase.Mul(birforms.MCIT)

	// Tax due = higher of RCIT or MCIT
	var taxDue decimal.Decimal
	if mcitAmount.GreaterThan(rcitAmount) {
		taxDue = mcitAmount
	} else {
		taxDue = rcitAmount
		// Can apply excess MCIT from prior years against RCIT
		taxDue = decMax(taxDue.Sub(excessMCITPrior), decimal.Zero)
	}

	totalCredits := cwt.Add(quarterlyPayments).Add(otherCredits)
	taxPayable := decMax(taxDue.Sub(totalCredits), decimal.Zero)
	totalPenalties := surcharge.Add(interest).Add(compromise)
	totalDue := taxPayable.Add(totalPenalties)

	return TaxResult{
		"gross_income":               grossIncome.String(),
		"cost_of_sales":              costOfSales.String(),
		"gross_profit":               grossProfit.String(),
		"other_income":               otherIncome.String(),
		"total_gross_income":         totalGross.String(),
		"deduction_method":           deductionMethod,
		"itemized_deductions":        itemized.String(),
		"osd_amount":                 osdAmount.String(),
		"total_deductions":           totalDeductions.String(),
		"net_taxable_income":         netTaxable.String(),
		"rcit_rate":                  rcitRate.String(),
		"rcit_amount":                rcitAmount.String(),
		"mcit_base":                  mcitBase.String(),
		"mcit_amount":                mcitAmount.String(),
		"income_tax_due":             taxDue.String(),
		"excess_mcit_prior":          excessMCITPrior.String(),
		"creditable_withholding_tax": cwt.String(),
		"quarterly_payments":         quarterlyPayments.String(),
		"other_credits":              otherCredits.String(),
		"total_tax_credits":          totalCredits.String(),
		"tax_payable":                taxPayable.String(),
		"surcharge":                  surcharge.String(),
		"interest":                   interest.String(),
		"compromise":                 compromise.String(),
		"total_penalties":            totalPenalties.String(),
		"total_amount_due":           totalDue.String(),
	}, nil
}

// CalculateBIR2316 computes the Certificate of Compensation Payment/Tax Withheld (BIR 2316).
func CalculateBIR2316(input map[string]interface{}) (TaxResult, error) {
	compData := toMap(input["compensation_data"])
	if compData == nil {
		compData = input
	}

	presentComp := toDecimal(compData["present_employer_compensation"])
	presentNT := toDecimal(compData["present_employer_nontaxable"])
	presentTaxable := decMax(presentComp.Sub(presentNT), decimal.Zero)

	prevComp := toDecimal(compData["previous_employer_compensation"])
	prevNT := toDecimal(compData["previous_employer_nontaxable"])
	prevTaxable := decMax(prevComp.Sub(prevNT), decimal.Zero)

	totalComp := presentComp.Add(prevComp)
	totalNT := presentNT.Add(prevNT)
	totalTaxable := presentTaxable.Add(prevTaxable)

	taxDue := computeGraduatedTax(totalTaxable)

	taxWithheldPresent := toDecimal(compData["tax_withheld_present"])
	taxWithheldPrevious := toDecimal(compData["tax_withheld_previous"])
	totalTaxWithheld := taxWithheldPresent.Add(taxWithheldPrevious)

	diff := totalTaxWithheld.Sub(taxDue)
	amountRefunded := decMax(diff, decimal.Zero)
	amountStillDue := decMax(diff.Neg(), decimal.Zero)

	return TaxResult{
		"employee_name":                  toString(compData["employee_name"]),
		"employee_tin":                   toString(compData["employee_tin"]),
		"employer_name":                  toString(compData["employer_name"]),
		"employer_tin":                   toString(compData["employer_tin"]),
		"present_employer_compensation":  presentComp.String(),
		"present_employer_nontaxable":    presentNT.String(),
		"present_employer_taxable":       presentTaxable.String(),
		"previous_employer_compensation": prevComp.String(),
		"previous_employer_nontaxable":   prevNT.String(),
		"previous_employer_taxable":      prevTaxable.String(),
		"total_compensation":             totalComp.String(),
		"total_nontaxable_compensation":  totalNT.String(),
		"total_taxable_compensation":     totalTaxable.String(),
		"tax_due":                        taxDue.String(),
		"tax_withheld_present":           taxWithheldPresent.String(),
		"tax_withheld_previous":          taxWithheldPrevious.String(),
		"total_tax_withheld":             totalTaxWithheld.String(),
		"amount_refunded":                amountRefunded.String(),
		"amount_still_due":               amountStillDue.String(),
	}, nil
}

// computeGraduatedTax calculates income tax using TRAIN Law graduated brackets.
func computeGraduatedTax(taxableIncome decimal.Decimal) decimal.Decimal {
	if taxableIncome.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}

	prevLimit := decimal.Zero
	for _, b := range birforms.TRAINBrackets {
		// NotOver == 0 means no upper limit (last bracket)
		if b.NotOver.IsZero() || taxableIncome.LessThanOrEqual(b.NotOver) {
			excess := taxableIncome.Sub(prevLimit)
			return b.BaseTax.Add(excess.Mul(b.Rate))
		}
		prevLimit = b.NotOver
	}
	return decimal.Zero
}

// --- Helper functions for safe type conversion ---

func toDecimal(v interface{}) decimal.Decimal {
	if v == nil {
		return decimal.Zero
	}
	switch val := v.(type) {
	case decimal.Decimal:
		return val
	case float64:
		return decimal.NewFromFloat(val)
	case float32:
		return decimal.NewFromFloat32(val)
	case int:
		return decimal.NewFromInt(int64(val))
	case int64:
		return decimal.NewFromInt(val)
	case string:
		if val == "" {
			return decimal.Zero
		}
		d, err := decimal.NewFromString(val)
		if err != nil {
			return decimal.Zero
		}
		return d
	default:
		d, err := decimal.NewFromString(fmt.Sprintf("%v", val))
		if err != nil {
			return decimal.Zero
		}
		return d
	}
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func toBool(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		lower := strings.ToLower(val)
		return lower == "true" || lower == "1" || lower == "yes"
	case float64:
		return val != 0
	case int:
		return val != 0
	default:
		return false
	}
}

func toMap(v interface{}) map[string]interface{} {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]interface{})
	if ok {
		return m
	}
	return nil
}

func toMapSlice(v interface{}) []map[string]interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []map[string]interface{}:
		return val
	case []interface{}:
		result := make([]map[string]interface{}, 0, len(val))
		for _, item := range val {
			if m, ok := item.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result
	default:
		return nil
	}
}

func decMax(a, b decimal.Decimal) decimal.Decimal {
	if a.GreaterThan(b) {
		return a
	}
	return b
}
