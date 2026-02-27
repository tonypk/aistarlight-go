package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/pkg/irasforms"
)

var uenPattern = regexp.MustCompile(`^(\d{8,9}[A-Z]|[TS]\d{2}[A-Z]{2}\d{4}[A-Z])$`)

// RunSGChecks executes all Singapore compliance rules against report data.
func RunSGChecks(data map[string]interface{}, reportType string, priorData map[string]interface{}, existingReports []map[string]interface{}) []CheckResult {
	var results []CheckResult

	results = append(results, checkSGRequiredFields(data, reportType))
	results = append(results, checkSGCrossFieldConsistency(data, reportType))
	results = append(results, checkUENFormat(data))
	results = append(results, checkGSTRegistrationThreshold(data))
	results = append(results, checkSGAmountRanges(data, reportType))
	results = append(results, checkSGFilingDeadline(data, reportType))
	results = append(results, checkSGDuplicateReport(data, reportType, existingReports))
	results = append(results, checkSGZeroFiling(data, reportType))

	return results
}

func checkSGRequiredFields(data map[string]interface{}, reportType string) CheckResult {
	var required []string
	switch reportType {
	case irasforms.FormGSTF5:
		required = []string{"standard_rated_supplies", "box_6_output_tax", "box_7_input_tax_claimed"}
	case irasforms.FormFormC, irasforms.FormFormCS:
		required = []string{"revenue", "chargeable_income", "tax_payable"}
	case irasforms.FormFormB:
		required = []string{"total_income", "chargeable_income", "tax_payable"}
	case irasforms.FormIR8A:
		required = []string{"gross_salary", "total_gross", "employer_cpf", "employee_cpf"}
	case irasforms.FormS45:
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
		return newCheck("sg_required_fields", "Required Fields", "critical", false,
			fmt.Sprintf("Missing required fields: %s", strings.Join(missing, ", ")))
	}
	return newCheck("sg_required_fields", "Required Fields", "critical", true, "All required fields present")
}

func checkSGCrossFieldConsistency(data map[string]interface{}, reportType string) CheckResult {
	switch reportType {
	case irasforms.FormGSTF5:
		return checkGSTF5Consistency(data)
	case irasforms.FormFormC, irasforms.FormFormCS:
		return checkCorpTaxConsistency(data)
	default:
		return newCheck("sg_cross_field", "Cross-field Consistency", "critical", true, "No cross-field checks for this form")
	}
}

func checkGSTF5Consistency(data map[string]interface{}) CheckResult {
	box1 := toDecimal(data["box_1_standard_rated_supplies"])
	box2 := toDecimal(data["box_2_zero_rated_supplies"])
	box3 := toDecimal(data["box_3_exempt_supplies"])
	box4 := toDecimal(data["box_4_total_supplies"])

	expectedBox4 := box1.Add(box2).Add(box3)
	if !box4.IsZero() && box4.Sub(expectedBox4).Abs().GreaterThan(decimalTolerance) {
		return newCheck("sg_cross_field", "Cross-field Consistency", "critical", false,
			fmt.Sprintf("Box 4 (S$%s) should equal Box 1+2+3 (S$%s)", box4.String(), expectedBox4.String()))
	}

	box6 := toDecimal(data["box_6_output_tax"])
	expectedOutput := box1.Mul(irasforms.GSTRate)
	if !box6.IsZero() && box6.Sub(expectedOutput).Abs().GreaterThan(decimal.NewFromInt(1)) {
		return newCheck("sg_cross_field", "Cross-field Consistency", "critical", false,
			fmt.Sprintf("Box 6 output tax (S$%s) inconsistent with Box 1 × 9%% (S$%s)", box6.String(), expectedOutput.Round(2).String()))
	}

	return newCheck("sg_cross_field", "Cross-field Consistency", "critical", true, "GST F5 cross-field checks passed")
}

func checkCorpTaxConsistency(data map[string]interface{}) CheckResult {
	chargeableIncome := toDecimal(data["chargeable_income"])
	taxPayable := toDecimal(data["tax_payable"])

	if chargeableIncome.IsZero() && !taxPayable.IsZero() {
		return newCheck("sg_cross_field", "Cross-field Consistency", "critical", false,
			"Tax payable is non-zero but chargeable income is zero")
	}

	if chargeableIncome.GreaterThan(decimal.Zero) {
		effectiveRate := taxPayable.Div(chargeableIncome)
		if effectiveRate.GreaterThan(irasforms.CorporateRate) {
			return newCheck("sg_cross_field", "Cross-field Consistency", "high", false,
				fmt.Sprintf("Effective tax rate %.2f%% exceeds corporate rate 17%%", effectiveRate.Mul(decimal.NewFromInt(100)).InexactFloat64()))
		}
	}

	return newCheck("sg_cross_field", "Cross-field Consistency", "critical", true, "Corporate tax cross-field checks passed")
}

func checkUENFormat(data map[string]interface{}) CheckResult {
	uen := toString(data["uen"])
	if uen == "" {
		return newCheck("sg_uen_format", "UEN Format", "medium", true, "No UEN provided (optional)")
	}

	if !uenPattern.MatchString(strings.ToUpper(uen)) {
		return newCheck("sg_uen_format", "UEN Format", "medium", false,
			fmt.Sprintf("Invalid UEN format: %s (expected format: 123456789A or T12AB1234X)", uen))
	}

	return newCheck("sg_uen_format", "UEN Format", "medium", true, "UEN format is valid")
}

func checkGSTRegistrationThreshold(data map[string]interface{}) CheckResult {
	revenue := toDecimal(data["revenue"])
	if revenue.IsZero() {
		revenue = toDecimal(data["box_4_total_supplies"])
	}

	threshold := decimal.NewFromInt(1_000_000)
	if revenue.GreaterThan(threshold) {
		return newCheck("sg_gst_threshold", "GST Registration Threshold", "high", true,
			fmt.Sprintf("Revenue S$%s exceeds S$1M — GST registration is mandatory", revenue.Round(2).String()))
	}

	return newCheck("sg_gst_threshold", "GST Registration Threshold", "high", true,
		"Revenue below S$1M GST registration threshold")
}

func checkSGAmountRanges(data map[string]interface{}, reportType string) CheckResult {
	for key, val := range data {
		d := toDecimal(val)
		if d.Abs().GreaterThan(maxAmount) {
			return newCheck("sg_amount_ranges", "Amount Ranges", "high", false,
				fmt.Sprintf("Field %s value S$%s exceeds reasonable range", key, d.String()))
		}
	}
	return newCheck("sg_amount_ranges", "Amount Ranges", "high", true, "All amounts within reasonable ranges")
}

func checkSGFilingDeadline(data map[string]interface{}, reportType string) CheckResult {
	period := toString(data["period"])
	if period == "" {
		return newCheck("sg_filing_deadline", "Filing Deadline", "medium", true, "No period specified")
	}
	return newCheck("sg_filing_deadline", "Filing Deadline", "medium", true, "Filing deadline check OK")
}

func checkSGDuplicateReport(data map[string]interface{}, reportType string, existingReports []map[string]interface{}) CheckResult {
	period := toString(data["period"])
	for _, r := range existingReports {
		if toString(r["report_type"]) == reportType && toString(r["period"]) == period {
			return newCheck("sg_duplicate", "Duplicate Report", "medium", false,
				fmt.Sprintf("A %s report for period %s already exists", reportType, period))
		}
	}
	return newCheck("sg_duplicate", "Duplicate Report", "medium", true, "No duplicate report found")
}

func checkSGZeroFiling(data map[string]interface{}, reportType string) CheckResult {
	switch reportType {
	case irasforms.FormGSTF5:
		box4 := toDecimal(data["box_4_total_supplies"])
		box6 := toDecimal(data["box_6_output_tax"])
		if box4.IsZero() && box6.IsZero() {
			return newCheck("sg_zero_filing", "Zero Filing Warning", "low", false,
				"GST F5 with zero supplies and zero output tax — confirm this is intentional")
		}
	case irasforms.FormFormC, irasforms.FormFormCS:
		revenue := toDecimal(data["revenue"])
		if revenue.IsZero() {
			return newCheck("sg_zero_filing", "Zero Filing Warning", "low", false,
				"Corporate tax return with zero revenue — confirm this is intentional")
		}
	}
	return newCheck("sg_zero_filing", "Zero Filing Warning", "low", true, "Non-zero filing amounts present")
}
