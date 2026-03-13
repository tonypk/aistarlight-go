package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/pkg/birforms"
)

var decimalTolerance = decimal.NewFromFloat(0.50)
var tinPattern = regexp.MustCompile(`^\d{3}-\d{3}-\d{3}-\d{3}$`)
var maxAmount = decimal.NewFromInt(999_999_999)

// CheckResult represents a single compliance check result.
type CheckResult struct {
	CheckID   string `json:"check_id"`
	CheckName string `json:"check_name"`
	Severity  string `json:"severity"` // critical, high, medium, low
	Passed    bool   `json:"passed"`
	Message   string `json:"message"`
}

func newCheck(id, name, severity string, passed bool, msg string) CheckResult {
	return CheckResult{CheckID: id, CheckName: name, Severity: severity, Passed: passed, Message: msg}
}

// normalizeBIRFields ensures all expected compliance check fields exist in the data map.
// It maps line_* prefixed keys to short names and defaults missing numeric fields to "0".
func normalizeBIRFields(data map[string]interface{}) {
	// Map from line_* keys to legacy short keys (compliance checks use short keys)
	lineToShort := map[string]string{
		"line_1_vatable_sales":        "vatable_sales",
		"line_2_sales_to_government":  "sales_to_government",
		"line_3_zero_rated_sales":     "zero_rated_sales",
		"line_4_exempt_sales":         "vat_exempt_sales",
		"line_5_total_sales":          "total_sales",
		"line_6_output_vat":           "output_vat",
		"line_6a_output_vat_government": "output_vat_government",
		"line_6b_total_output_vat":    "total_output_vat",
		"line_7_input_vat_goods":      "input_vat_goods",
		"line_8_input_vat_capital":    "input_vat_capital",
		"line_9_input_vat_services":   "input_vat_services",
		"line_10_input_vat_imports":   "input_vat_imports",
		"line_11_total_input_vat":     "total_input_vat",
		"line_12_vat_payable":         "vat_payable",
		"line_16_total_amount_due":    "total_amount_due",
	}

	// Fill short keys from line_* keys if missing
	for lineKey, shortKey := range lineToShort {
		if _, ok := data[shortKey]; !ok {
			if v, ok := data[lineKey]; ok {
				data[shortKey] = v
			}
		}
	}

	// Default fields that can legitimately be zero
	zeroDefaults := []string{
		"sales_to_government", "zero_rated_sales", "vat_exempt_sales",
		"output_vat_government", "input_vat_goods", "input_vat_capital",
		"input_vat_services", "input_vat_imports",
	}
	for _, f := range zeroDefaults {
		if _, ok := data[f]; !ok {
			data[f] = "0"
		}
	}
}

// RunAllChecks executes all compliance rules against report data.
// Routes to jurisdiction-specific checks based on report type prefix.
func RunAllChecks(data map[string]interface{}, reportType string, priorData map[string]interface{}, existingReports []map[string]interface{}) []CheckResult {
	// Route IRAS_ prefixed forms to Singapore checks
	if strings.HasPrefix(reportType, "IRAS_") {
		return RunSGChecks(data, reportType, priorData, existingReports)
	}
	// Route IRDSL_ prefixed forms to Sri Lanka checks
	if strings.HasPrefix(reportType, "IRDSL_") {
		return RunLKChecks(data, reportType, priorData, existingReports)
	}

	// Normalize BIR field names: fill short keys from line_* keys, default zeros
	normalizeBIRFields(data)

	// Default: Philippine (BIR) checks
	var results []CheckResult

	results = append(results, checkRequiredFields(data, reportType))
	results = append(results, checkCrossFieldConsistency(data, reportType))
	results = append(results, checkOutputVATAccuracy(data, reportType))
	results = append(results, checkGovernmentVATRate(data, reportType))
	results = append(results, checkAmountRanges(data, reportType))
	results = append(results, checkTINFormat(data, reportType))
	results = append(results, checkFilingDeadline(data, reportType))
	results = append(results, checkPeriodOverPeriodAnomaly(data, reportType, priorData))
	results = append(results, checkDuplicateReport(data, reportType, existingReports))
	results = append(results, checkCapitalGoodsThreshold(data, reportType))
	results = append(results, checkZeroFilingWarning(data, reportType))

	return results
}

func checkRequiredFields(data map[string]interface{}, reportType string) CheckResult {
	var required []string
	switch reportType {
	case birforms.FormBIR2550M, birforms.FormBIR2550Q:
		required = []string{"vatable_sales", "sales_to_government", "zero_rated_sales", "vat_exempt_sales", "output_vat", "total_input_vat"}
	case birforms.FormBIR1601C:
		required = []string{"line_1_total_compensation", "line_9_tax_withheld", "line_11_total_tax_remitted"}
	case birforms.FormBIR0619E:
		required = []string{"line_1_total_amount_of_income_payments", "line_2_total_taxes_withheld", "line_9_total_amount_due"}
	default:
		required = []string{}
	}

	var missing []string
	for _, f := range required {
		if v, ok := data[f]; !ok || v == nil {
			missing = append(missing, f)
		}
	}

	if len(missing) > 0 {
		return newCheck("required_fields", "Required Fields", "critical", false,
			fmt.Sprintf("Missing required fields: %s", strings.Join(missing, ", ")))
	}
	return newCheck("required_fields", "Required Fields", "critical", true, "All required fields present")
}

func checkCrossFieldConsistency(data map[string]interface{}, reportType string) CheckResult {
	if reportType != birforms.FormBIR2550M && reportType != birforms.FormBIR2550Q {
		return newCheck("cross_field", "Cross-field Consistency", "critical", true, "Not applicable for this form type")
	}

	vatable := toDecimal(data["vatable_sales"])
	govt := toDecimal(data["sales_to_government"])
	zeroRated := toDecimal(data["zero_rated_sales"])
	exempt := toDecimal(data["vat_exempt_sales"])
	totalSales := toDecimal(data["total_sales"])

	computed := vatable.Add(govt).Add(zeroRated).Add(exempt)
	diff := computed.Sub(totalSales).Abs()

	if diff.GreaterThan(decimalTolerance) {
		return newCheck("cross_field", "Cross-field Consistency", "critical", false,
			fmt.Sprintf("Total sales (%.2f) != sum of components (%.2f), diff=%.2f",
				totalSales.InexactFloat64(), computed.InexactFloat64(), diff.InexactFloat64()))
	}
	return newCheck("cross_field", "Cross-field Consistency", "critical", true, "Sales components sum correctly")
}

func checkOutputVATAccuracy(data map[string]interface{}, reportType string) CheckResult {
	if reportType != birforms.FormBIR2550M && reportType != birforms.FormBIR2550Q {
		return newCheck("output_vat", "Output VAT Accuracy", "high", true, "Not applicable")
	}

	vatable := toDecimal(data["vatable_sales"])
	outputVAT := toDecimal(data["output_vat"])
	expected := vatable.Mul(birforms.VATRate)
	diff := outputVAT.Sub(expected).Abs()

	if diff.GreaterThan(decimalTolerance) {
		return newCheck("output_vat", "Output VAT Accuracy", "high", false,
			fmt.Sprintf("Output VAT (%.2f) != vatable_sales * 12%% (%.2f)",
				outputVAT.InexactFloat64(), expected.InexactFloat64()))
	}
	return newCheck("output_vat", "Output VAT Accuracy", "high", true, "Output VAT matches 12% of vatable sales")
}

func checkGovernmentVATRate(data map[string]interface{}, reportType string) CheckResult {
	if reportType != birforms.FormBIR2550M && reportType != birforms.FormBIR2550Q {
		return newCheck("govt_vat", "Government VAT Rate", "high", true, "Not applicable")
	}

	govtSales := toDecimal(data["sales_to_government"])
	govtVAT := toDecimal(data["output_vat_government"])
	if govtSales.IsZero() {
		return newCheck("govt_vat", "Government VAT Rate", "high", true, "No government sales")
	}

	expected := govtSales.Mul(birforms.GovtVATRate)
	diff := govtVAT.Sub(expected).Abs()

	if diff.GreaterThan(decimalTolerance) {
		return newCheck("govt_vat", "Government VAT Rate", "high", false,
			fmt.Sprintf("Government VAT (%.2f) != govt_sales * 5%% (%.2f)",
				govtVAT.InexactFloat64(), expected.InexactFloat64()))
	}
	return newCheck("govt_vat", "Government VAT Rate", "high", true, "Government VAT matches 5% rate")
}

func checkAmountRanges(data map[string]interface{}, _ string) CheckResult {
	amountFields := []string{
		"vatable_sales", "sales_to_government", "zero_rated_sales", "vat_exempt_sales",
		"total_sales", "output_vat", "total_input_vat",
		"line_1_total_compensation", "line_9_tax_withheld", "line_11_total_tax_remitted",
		"line_1_total_amount_of_income_payments", "line_2_total_taxes_withheld",
	}

	for _, f := range amountFields {
		v, ok := data[f]
		if !ok {
			continue
		}
		amt := toDecimal(v)
		if amt.LessThan(decimal.Zero) {
			return newCheck("amount_ranges", "Amount Ranges", "high", false,
				fmt.Sprintf("Field %s has negative value: %.2f", f, amt.InexactFloat64()))
		}
		if amt.GreaterThan(maxAmount) {
			return newCheck("amount_ranges", "Amount Ranges", "high", false,
				fmt.Sprintf("Field %s exceeds maximum (999,999,999): %.2f", f, amt.InexactFloat64()))
		}
	}
	return newCheck("amount_ranges", "Amount Ranges", "high", true, "All amounts within valid range")
}

func checkTINFormat(data map[string]interface{}, _ string) CheckResult {
	tin := toString(data["tin_number"])
	if tin == "" {
		return newCheck("tin_format", "TIN Format", "medium", true, "No TIN to validate")
	}
	if !tinPattern.MatchString(tin) {
		return newCheck("tin_format", "TIN Format", "medium", false,
			fmt.Sprintf("TIN format invalid: %s (expected ###-###-###-###)", tin))
	}
	return newCheck("tin_format", "TIN Format", "medium", true, "TIN format valid")
}

func checkFilingDeadline(data map[string]interface{}, reportType string) CheckResult {
	period := toString(data["period"])
	if period == "" {
		return newCheck("filing_deadline", "Filing Deadline", "medium", true, "No period specified")
	}

	// Parse period (YYYY-MM format)
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return newCheck("filing_deadline", "Filing Deadline", "medium", true, "Cannot parse period")
	}

	var deadlineDay int
	switch reportType {
	case birforms.FormBIR2550M:
		deadlineDay = 20 // Monthly VAT due 20th of following month
	case birforms.FormBIR0619E, birforms.FormBIR1601C:
		deadlineDay = 10 // EWT/WHT due 10th of following month
	case birforms.FormBIR2550Q:
		deadlineDay = 25 // Quarterly VAT due 25th of month after quarter
	default:
		return newCheck("filing_deadline", "Filing Deadline", "medium", true, "No deadline rule for this form")
	}

	// Deadline is in the following month
	nextMonth := t.AddDate(0, 1, 0)
	deadline := time.Date(nextMonth.Year(), nextMonth.Month(), deadlineDay, 0, 0, 0, 0, time.Local)

	if time.Now().After(deadline) {
		return newCheck("filing_deadline", "Filing Deadline", "medium", false,
			fmt.Sprintf("Filing deadline passed: %s", deadline.Format("2006-01-02")))
	}
	return newCheck("filing_deadline", "Filing Deadline", "medium", true,
		fmt.Sprintf("Filing deadline: %s", deadline.Format("2006-01-02")))
}

func checkPeriodOverPeriodAnomaly(data map[string]interface{}, reportType string, priorData map[string]interface{}) CheckResult {
	if priorData == nil {
		return newCheck("period_anomaly", "Period-over-Period", "medium", true, "No prior period data for comparison")
	}

	var field string
	switch reportType {
	case birforms.FormBIR2550M, birforms.FormBIR2550Q:
		field = "total_sales"
	case birforms.FormBIR1601C:
		field = "line_1_total_compensation"
	case birforms.FormBIR0619E:
		field = "line_9_total_amount_due"
	default:
		field = "total_amount_due"
	}

	current := toDecimal(data[field])
	prior := toDecimal(priorData[field])

	if prior.IsZero() {
		return newCheck("period_anomaly", "Period-over-Period", "medium", true, "Prior period value is zero")
	}

	change := current.Sub(prior).Div(prior).Abs()
	threshold := decimal.NewFromFloat(0.50) // 50% change

	if change.GreaterThan(threshold) {
		pct := change.Mul(decimal.NewFromInt(100))
		return newCheck("period_anomaly", "Period-over-Period", "medium", false,
			fmt.Sprintf("%s changed by %.1f%% vs prior period (%.2f → %.2f)",
				field, pct.InexactFloat64(), prior.InexactFloat64(), current.InexactFloat64()))
	}
	return newCheck("period_anomaly", "Period-over-Period", "medium", true, "No significant period-over-period changes")
}

func checkDuplicateReport(data map[string]interface{}, reportType string, existingReports []map[string]interface{}) CheckResult {
	if len(existingReports) == 0 {
		return newCheck("duplicate", "Duplicate Report", "medium", true, "No existing reports to check")
	}

	period := toString(data["period"])
	count := 0
	for _, r := range existingReports {
		if toString(r["report_type"]) == reportType && toString(r["period"]) == period {
			status := toString(r["status"])
			if status != "archived" && status != "rejected" {
				count++
			}
		}
	}

	if count > 1 {
		return newCheck("duplicate", "Duplicate Report", "medium", false,
			fmt.Sprintf("Found %d active reports for %s period %s", count, reportType, period))
	}
	return newCheck("duplicate", "Duplicate Report", "medium", true, "No duplicate reports found")
}

func checkCapitalGoodsThreshold(data map[string]interface{}, reportType string) CheckResult {
	if reportType != birforms.FormBIR2550M && reportType != birforms.FormBIR2550Q {
		return newCheck("capital_goods", "Capital Goods Threshold", "low", true, "Not applicable")
	}

	inputVATCapital := toDecimal(data["input_vat_capital"])
	threshold := decimal.NewFromInt(1_000_000) // PHP 1M per RR 16-2005

	if inputVATCapital.GreaterThan(threshold) {
		return newCheck("capital_goods", "Capital Goods Threshold", "low", false,
			fmt.Sprintf("Input VAT on capital goods (%.2f) exceeds PHP 1M — requires amortization per RR 16-2005",
				inputVATCapital.InexactFloat64()))
	}
	return newCheck("capital_goods", "Capital Goods Threshold", "low", true, "Capital goods within threshold")
}

func checkZeroFilingWarning(data map[string]interface{}, _ string) CheckResult {
	// Check common amount_due field names across forms
	amountDue := toDecimal(data["total_amount_due"])
	if amountDue.IsZero() {
		amountDue = toDecimal(data["line_16_total_amount_due"])
	}
	if amountDue.IsZero() {
		amountDue = toDecimal(data["line_9_total_amount_due"])
	}
	if amountDue.IsZero() {
		taxDue := toDecimal(data["income_tax_due"])
		if taxDue.IsZero() {
			return newCheck("zero_filing", "Zero Filing", "low", false,
				"Total amount due is zero — this is a nil return filing")
		}
	}
	return newCheck("zero_filing", "Zero Filing", "low", true, "Non-zero filing")
}
