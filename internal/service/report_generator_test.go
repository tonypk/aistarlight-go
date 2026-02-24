package service

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatPDFAmount(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"0", "0.00"},
		{"100", "100.00"},
		{"1234.56", "1,234.56"},
		{"1000000", "1,000,000.00"},
		{"99999.99", "99,999.99"},
		{"-500", "(500.00)"},
		{"-1234567.89", "(1,234,567.89)"},
		{"0.5", "0.50"},
		{"abc", "abc"}, // non-numeric returns as-is
	}

	for _, tt := range tests {
		got := formatPDFAmount(tt.input)
		if got != tt.want {
			t.Errorf("formatPDFAmount(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetD(t *testing.T) {
	data := map[string]string{
		"line_1_vatable_sales": "1000",
		"vatable_sales":        "500",
	}

	// First key found
	if got := getD(data, "line_1_vatable_sales"); got != "1000" {
		t.Errorf("expected 1000, got %s", got)
	}

	// Falls to second key
	if got := getD(data, "missing_key", "vatable_sales"); got != "500" {
		t.Errorf("expected 500, got %s", got)
	}

	// No key found → "0"
	if got := getD(data, "no_such_key"); got != "0" {
		t.Errorf("expected 0, got %s", got)
	}

	// Empty value skipped
	data["empty"] = ""
	if got := getD(data, "empty", "line_1_vatable_sales"); got != "1000" {
		t.Errorf("expected 1000 (skip empty), got %s", got)
	}
}

func sampleVATData() map[string]string {
	return map[string]string{
		"period":                        "2025-01",
		"line_1_vatable_sales":          "100000",
		"line_2_sales_to_government":    "20000",
		"line_3_zero_rated_sales":       "5000",
		"line_4_exempt_sales":           "3000",
		"line_5_total_sales":            "128000",
		"line_6_output_vat":             "12000",
		"line_6a_output_vat_government": "1000",
		"line_6b_total_output_vat":      "13000",
		"line_7_input_vat_goods":        "5000",
		"line_8_input_vat_capital":      "2000",
		"line_9_input_vat_services":     "1000",
		"line_10_input_vat_imports":     "500",
		"line_11_total_input_vat":       "8500",
		"line_12_vat_payable":           "4500",
		"line_13_less_tax_credits":      "1000",
		"line_14_net_vat_payable":       "3500",
		"line_15_add_penalties":         "0",
		"line_16_total_amount_due":      "3500",
		"tax_credit_carried_forward":    "0",
	}
}

func sampleCompany() CompanyInfo {
	return CompanyInfo{
		CompanyName: "Test Corp, Inc.",
		TINNumber:   "123-456-789-000",
		RDOCode:     "044",
	}
}

func TestGeneratePDFReport_BIR2550M(t *testing.T) {
	var buf bytes.Buffer
	err := GeneratePDFReport(&buf, "BIR_2550M", sampleVATData(), sampleCompany())
	if err != nil {
		t.Fatalf("GeneratePDFReport BIR_2550M: %v", err)
	}
	if buf.Len() < 1000 {
		t.Errorf("PDF too small: %d bytes", buf.Len())
	}
	// Verify it's a valid PDF (starts with %PDF)
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Error("Output does not start with %PDF header")
	}
	t.Logf("BIR 2550M PDF: %d bytes", buf.Len())
}

func TestGeneratePDFReport_BIR2550Q(t *testing.T) {
	var buf bytes.Buffer
	err := GeneratePDFReport(&buf, "BIR_2550Q", sampleVATData(), sampleCompany())
	if err != nil {
		t.Fatalf("GeneratePDFReport BIR_2550Q: %v", err)
	}
	if buf.Len() < 1000 {
		t.Errorf("PDF too small: %d bytes", buf.Len())
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Error("Output does not start with %PDF header")
	}
}

func TestGeneratePDFReport_BIR1601C(t *testing.T) {
	data := map[string]string{
		"period":                         "2025-01",
		"line_1_total_compensation":      "500000",
		"line_2_statutory_minimum_wage":  "100000",
		"line_3_nontaxable_13th_month":  "90000",
		"line_4_nontaxable_deminimis":   "10000",
		"line_5_sss_gsis_phic_hdmf":     "30000",
		"line_6_other_nontaxable":       "5000",
		"line_7_total_nontaxable":       "235000",
		"line_8_taxable_compensation":   "265000",
		"line_9_tax_withheld":           "15000",
		"line_10_adjustment":            "0",
		"line_11_total_tax_remitted":    "15000",
		"line_12_surcharge":             "0",
		"line_13_interest":              "0",
		"line_14_compromise":            "0",
		"line_15_total_penalties":       "0",
		"line_16_total_amount_due":      "15000",
	}

	var buf bytes.Buffer
	err := GeneratePDFReport(&buf, "BIR_1601C", data, sampleCompany())
	if err != nil {
		t.Fatalf("BIR_1601C: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Error("Not a valid PDF")
	}
	t.Logf("BIR 1601-C PDF: %d bytes", buf.Len())
}

func TestGeneratePDFReport_BIR0619E(t *testing.T) {
	data := map[string]string{
		"period":                                 "2025-01",
		"line_1_total_amount_of_income_payments": "800000",
		"line_2_total_taxes_withheld":            "40000",
		"line_3_adjustment":                      "0",
		"line_4_tax_still_due":                   "40000",
		"line_5_surcharge":                       "0",
		"line_6_interest":                        "0",
		"line_7_compromise":                      "0",
		"line_8_total_penalties":                  "0",
		"line_9_total_amount_due":                "40000",
	}

	var buf bytes.Buffer
	err := GeneratePDFReport(&buf, "BIR_0619E", data, sampleCompany())
	if err != nil {
		t.Fatalf("BIR_0619E: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Error("Not a valid PDF")
	}
}

func TestGeneratePDFReport_BIR1701(t *testing.T) {
	data := map[string]string{
		"period":                     "2024",
		"deduction_method":           "osd",
		"gross_sales_receipts":       "2000000",
		"cost_of_sales":              "800000",
		"gross_income_from_business": "1200000",
		"other_taxable_income":       "50000",
		"total_gross_income":         "1250000",
		"osd_amount":                 "800000",
		"total_deductions":           "800000",
		"net_taxable_income":         "450000",
		"income_tax_due":             "32500",
		"creditable_withholding_tax": "10000",
		"quarterly_payments":         "15000",
		"other_credits":              "0",
		"total_tax_credits":          "25000",
		"tax_payable":                "7500",
		"surcharge":                  "0",
		"interest":                   "0",
		"compromise":                 "0",
		"total_penalties":            "0",
		"total_amount_due":           "7500",
	}

	var buf bytes.Buffer
	err := GeneratePDFReport(&buf, "BIR_1701", data, sampleCompany())
	if err != nil {
		t.Fatalf("BIR_1701: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Error("Not a valid PDF")
	}
	t.Logf("BIR 1701 PDF: %d bytes", buf.Len())
}

func TestGeneratePDFReport_BIR1702(t *testing.T) {
	data := map[string]string{
		"period":                     "2024",
		"deduction_method":           "itemized",
		"gross_income":               "5000000",
		"cost_of_sales":              "2000000",
		"gross_profit":               "3000000",
		"other_income":               "100000",
		"total_gross_income":         "3100000",
		"itemized_deductions":        "800000",
		"total_deductions":           "800000",
		"net_taxable_income":         "2300000",
		"rcit_rate":                  "0.25",
		"rcit_amount":                "575000",
		"mcit_base":                  "5000000",
		"mcit_amount":                "50000",
		"income_tax_due":             "575000",
		"excess_mcit_prior":          "0",
		"creditable_withholding_tax": "100000",
		"quarterly_payments":         "200000",
		"other_credits":              "0",
		"total_tax_credits":          "300000",
		"tax_payable":                "275000",
		"surcharge":                  "0",
		"interest":                   "0",
		"compromise":                 "0",
		"total_penalties":            "0",
		"total_amount_due":           "275000",
	}

	var buf bytes.Buffer
	err := GeneratePDFReport(&buf, "BIR_1702", data, sampleCompany())
	if err != nil {
		t.Fatalf("BIR_1702: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Error("Not a valid PDF")
	}
}

func TestGeneratePDFReport_BIR2316(t *testing.T) {
	data := map[string]string{
		"period":                         "2024",
		"employee_name":                  "Juan Dela Cruz",
		"employee_tin":                   "987-654-321-000",
		"employer_name":                  "Test Corp, Inc.",
		"employer_tin":                   "123-456-789-000",
		"present_employer_compensation":  "600000",
		"present_employer_nontaxable":    "150000",
		"present_employer_taxable":       "450000",
		"previous_employer_compensation": "200000",
		"previous_employer_nontaxable":   "50000",
		"previous_employer_taxable":      "150000",
		"total_compensation":             "800000",
		"total_nontaxable_compensation":  "200000",
		"total_taxable_compensation":     "600000",
		"tax_due":                        "62500",
		"tax_withheld_present":           "50000",
		"tax_withheld_previous":          "15000",
		"total_tax_withheld":             "65000",
		"amount_refunded":                "2500",
		"amount_still_due":               "0",
	}

	var buf bytes.Buffer
	err := GeneratePDFReport(&buf, "BIR_2316", data, sampleCompany())
	if err != nil {
		t.Fatalf("BIR_2316: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Error("Not a valid PDF")
	}
	t.Logf("BIR 2316 PDF: %d bytes", buf.Len())
}

func TestGeneratePDFReport_BIR2316_StillDue(t *testing.T) {
	data := map[string]string{
		"period":                         "2024",
		"employee_name":                  "Maria Santos",
		"present_employer_compensation":  "1000000",
		"present_employer_nontaxable":    "200000",
		"present_employer_taxable":       "800000",
		"previous_employer_compensation": "0",
		"previous_employer_nontaxable":   "0",
		"previous_employer_taxable":      "0",
		"total_compensation":             "1000000",
		"total_nontaxable_compensation":  "200000",
		"total_taxable_compensation":     "800000",
		"tax_due":                        "102500",
		"tax_withheld_present":           "90000",
		"tax_withheld_previous":          "0",
		"total_tax_withheld":             "90000",
		"amount_refunded":                "0",
		"amount_still_due":               "12500",
	}

	var buf bytes.Buffer
	err := GeneratePDFReport(&buf, "BIR_2316", data, sampleCompany())
	if err != nil {
		t.Fatalf("BIR_2316 StillDue: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Error("Not a valid PDF")
	}
}

func TestGeneratePDFReport_Generic(t *testing.T) {
	data := map[string]string{
		"period":  "2025-01",
		"field_1": "value_1",
		"field_2": "value_2",
	}

	var buf bytes.Buffer
	err := GeneratePDFReport(&buf, "CUSTOM_REPORT", data, sampleCompany())
	if err != nil {
		t.Fatalf("Generic: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Error("Not a valid PDF")
	}
}

func TestGeneratePDFReport_WithComplianceScore(t *testing.T) {
	data := sampleVATData()
	data["compliance_score"] = "85"

	var buf bytes.Buffer
	err := GeneratePDFReport(&buf, "BIR_2550M", data, sampleCompany())
	if err != nil {
		t.Fatalf("With compliance score: %v", err)
	}
	if buf.Len() < 1000 {
		t.Error("PDF too small")
	}
}

func TestGeneratePDFReport_WithTaxCredit(t *testing.T) {
	data := sampleVATData()
	data["tax_credit_carried_forward"] = "5000"

	var buf bytes.Buffer
	err := GeneratePDFReport(&buf, "BIR_2550M", data, sampleCompany())
	if err != nil {
		t.Fatalf("With tax credit: %v", err)
	}
	if buf.Len() < 1000 {
		t.Error("PDF too small")
	}
}

func TestGenerateReconciliationPDF(t *testing.T) {
	input := ReconciliationPDFInput{
		SessionID: "test-session-123",
		Period:    "2025-01",
		Status:    "completed",
		FileCount: 3,
		Company:   sampleCompany(),
		VATSummary: map[string]string{
			"vatable_sales":        "100000",
			"sales_to_government":  "20000",
			"total_sales":          "120000",
			"output_vat":           "12000",
			"total_output_vat":     "13000",
			"input_vat_goods":      "5000",
			"total_input_vat":      "5000",
			"net_vat":              "8000",
		},
		MatchStats: map[string]interface{}{
			"matched_pairs":    15,
			"unmatched_records": 3,
			"unmatched_bank":   2,
			"match_rate":       0.75,
		},
		Anomalies: []AnomalyEntry{
			{Severity: "high", Description: "Missing invoice for transaction #123", Status: "open"},
			{Severity: "medium", Description: "Amount mismatch: PHP 1,000 vs PHP 1,050", Status: "review"},
			{Severity: "low", Description: "Date discrepancy", Status: "resolved"},
		},
	}

	var buf bytes.Buffer
	err := GenerateReconciliationPDF(&buf, input)
	if err != nil {
		t.Fatalf("GenerateReconciliationPDF: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Error("Not a valid PDF")
	}
	t.Logf("Reconciliation PDF: %d bytes", buf.Len())
}

func TestGenerateCSVExport(t *testing.T) {
	data := map[string]string{
		"line_1_vatable_sales":    "100000",
		"line_5_total_sales":      "128000",
		"line_16_total_amount_due": "3500",
		"vatable_sales":           "100000", // non-line_ key should be skipped
	}

	var buf bytes.Buffer
	err := GenerateCSVExport(&buf, data)
	if err != nil {
		t.Fatalf("GenerateCSVExport: %v", err)
	}

	csv := buf.String()
	if !strings.Contains(csv, "Line,Field,Value (PHP)") {
		t.Error("Missing CSV header")
	}
	// Should contain line items but not "vatable_sales" (no line_ prefix)
	if strings.Contains(csv, "vatable_sales,") {
		t.Error("Non-line_ keys should be excluded")
	}
	lines := strings.Split(strings.TrimSpace(csv), "\n")
	if len(lines) != 4 { // header + 3 line items
		t.Errorf("Expected 4 CSV lines (header + 3 data), got %d", len(lines))
	}
}

func TestGeneratePDFReport_EmptyData(t *testing.T) {
	// Should handle empty data without panicking.
	data := map[string]string{"period": "2025-01"}

	var buf bytes.Buffer
	err := GeneratePDFReport(&buf, "BIR_2550M", data, sampleCompany())
	if err != nil {
		t.Fatalf("Empty data: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Error("Not a valid PDF")
	}
}
