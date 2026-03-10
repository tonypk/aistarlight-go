package service

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/pkg/lkforms"
)

// RunLKChecks executes all Sri Lanka compliance rules against report data.
func RunLKChecks(data map[string]interface{}, reportType string, priorData map[string]interface{}, existingReports []map[string]interface{}) []CheckResult {
	var results []CheckResult

	results = append(results, checkLKRequiredFields(data, reportType))
	results = append(results, checkLKCrossFieldConsistency(data, reportType))
	results = append(results, checkLKAmountRanges(data, reportType))
	results = append(results, checkLKFilingDeadline(data, reportType))
	results = append(results, checkLKDuplicateReport(data, reportType, existingReports))
	results = append(results, checkLKVATRate(data, reportType))

	return results
}

func checkLKRequiredFields(data map[string]interface{}, reportType string) CheckResult {
	var required []string
	switch reportType {
	case lkforms.FormVATReturn:
		required = []string{"standard_rated_supplies", "output_vat", "input_vat_claimed"}
	case lkforms.FormCIT:
		required = []string{"revenue", "taxable_income", "tax_payable"}
	case lkforms.FormPAYE:
		required = []string{"gross_salary", "epf_employee", "epf_employer", "etf_employer"}
	case lkforms.FormWHT:
		required = []string{"payment_amount", "income_type", "wht_rate", "tax_withheld"}
	default:
		required = []string{}
	}

	var missing []string
	for _, f := range required {
		if _, ok := data[f]; !ok {
			missing = append(missing, f)
		} else if toString(data[f]) == "" || toString(data[f]) == "0" {
			missing = append(missing, f)
		}
	}

	if len(missing) > 0 {
		return newCheck("lk_required_fields", "Required Fields", "critical", false,
			fmt.Sprintf("Missing required fields: %s", strings.Join(missing, ", ")))
	}
	return newCheck("lk_required_fields", "Required Fields", "critical", true, "All required fields present")
}

func checkLKCrossFieldConsistency(data map[string]interface{}, reportType string) CheckResult {
	if reportType != lkforms.FormVATReturn {
		return newCheck("lk_cross_field", "Cross-Field Consistency", "high", true, "Not applicable")
	}

	stdRated := toDecimal(data["standard_rated_supplies"])
	outputVAT := toDecimal(data["output_vat"])
	expectedVAT := stdRated.Mul(lkforms.VATRate)

	diff := outputVAT.Sub(expectedVAT).Abs()
	if diff.GreaterThan(decimalTolerance) {
		return newCheck("lk_cross_field", "Cross-Field Consistency", "high", false,
			fmt.Sprintf("Output VAT (%s) does not match 18%% of standard-rated supplies (%s). Expected: %s",
				outputVAT.StringFixed(2), stdRated.StringFixed(2), expectedVAT.StringFixed(2)))
	}

	return newCheck("lk_cross_field", "Cross-Field Consistency", "high", true, "Output VAT consistent with supplies")
}

func checkLKAmountRanges(data map[string]interface{}, reportType string) CheckResult {
	for _, field := range []string{"standard_rated_supplies", "output_vat", "revenue", "gross_salary"} {
		if v, ok := data[field]; ok {
			amount := toDecimal(v)
			if amount.LessThan(decimal.Zero) {
				return newCheck("lk_amount_range", "Amount Ranges", "high", false,
					fmt.Sprintf("Field '%s' has negative value: %s", field, amount.StringFixed(2)))
			}
			if amount.GreaterThan(maxAmount) {
				return newCheck("lk_amount_range", "Amount Ranges", "medium", false,
					fmt.Sprintf("Field '%s' exceeds maximum (%s > 999,999,999)", field, amount.StringFixed(2)))
			}
		}
	}
	return newCheck("lk_amount_range", "Amount Ranges", "high", true, "All amounts within valid range")
}

func checkLKFilingDeadline(data map[string]interface{}, reportType string) CheckResult {
	period := toString(data["period"])
	if period == "" {
		return newCheck("lk_filing_deadline", "Filing Deadline", "medium", true, "No period specified")
	}
	// Filing deadlines checked at calendar level; basic presence check here.
	return newCheck("lk_filing_deadline", "Filing Deadline", "medium", true, "Period specified")
}

func checkLKDuplicateReport(data map[string]interface{}, reportType string, existing []map[string]interface{}) CheckResult {
	period := toString(data["period"])
	for _, r := range existing {
		if toString(r["report_type"]) == reportType && toString(r["period"]) == period {
			return newCheck("lk_duplicate", "Duplicate Report", "high", false,
				fmt.Sprintf("A %s report already exists for period %s", reportType, period))
		}
	}
	return newCheck("lk_duplicate", "Duplicate Report", "high", true, "No duplicate report found")
}

func checkLKVATRate(data map[string]interface{}, reportType string) CheckResult {
	if reportType != lkforms.FormVATReturn {
		return newCheck("lk_vat_rate", "VAT Rate", "medium", true, "Not applicable")
	}

	stdRated := toDecimal(data["standard_rated_supplies"])
	if stdRated.IsZero() {
		return newCheck("lk_vat_rate", "VAT Rate", "medium", true, "No standard-rated supplies")
	}

	outputVAT := toDecimal(data["output_vat"])
	effectiveRate := outputVAT.Div(stdRated)
	expectedRate := lkforms.VATRate

	diff := effectiveRate.Sub(expectedRate).Abs()
	if diff.GreaterThan(decimal.NewFromFloat(0.005)) {
		return newCheck("lk_vat_rate", "VAT Rate", "medium", false,
			fmt.Sprintf("Effective VAT rate (%.2f%%) differs from expected 18%%", effectiveRate.Mul(decimal.NewFromInt(100)).InexactFloat64()))
	}
	return newCheck("lk_vat_rate", "VAT Rate", "medium", true, "VAT rate is correct (18%)")
}
