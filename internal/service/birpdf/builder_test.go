package birpdf

import (
	"bytes"
	"testing"
)

func testCompany() CompanyData {
	return CompanyData{
		Name:           "Test Corp, Inc.",
		TIN:            "123-456-789-000",
		RDOCode:        "044",
		Address:        "123 Main Street, Makati City",
		LineOfBusiness: "IT Services",
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
	}
}

func assertValidPDF(t *testing.T, buf *bytes.Buffer, label string) {
	t.Helper()
	if buf.Len() < 1000 {
		t.Errorf("%s: PDF too small: %d bytes", label, buf.Len())
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Errorf("%s: output does not start with %%PDF header", label)
	}
	t.Logf("%s: %d bytes", label, buf.Len())
}

func TestFormatAmount(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"0", "-"},
		{"100", "100.00"},
		{"1234.56", "1,234.56"},
		{"1000000", "1,000,000.00"},
		{"-500", "(500.00)"},
		{"abc", "abc"},
		{"", "-"},
	}
	for _, tt := range tests {
		got := formatAmount(tt.input)
		if got != tt.want {
			t.Errorf("formatAmount(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetVal(t *testing.T) {
	data := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	if got := getVal(data, "key1"); got != "value1" {
		t.Errorf("expected value1, got %s", got)
	}
	if got := getVal(data, "missing", "key2"); got != "value2" {
		t.Errorf("expected value2, got %s", got)
	}
	if got := getVal(data, "missing"); got != "0" {
		t.Errorf("expected 0, got %s", got)
	}
}

func TestGenerate0619E(t *testing.T) {
	data := map[string]string{
		"period":                                 "2025-01",
		"line_1_total_amount_of_income_payments": "800000",
		"line_2_total_taxes_withheld":            "40000",
		"line_3_adjustment":                      "0",
		"line_4_tax_still_due":                   "40000",
		"line_5_surcharge":                       "0",
		"line_6_interest":                        "0",
		"line_7_compromise":                      "0",
		"line_8_total_penalties":                 "0",
		"line_9_total_amount_due":                "40000",
	}
	var buf bytes.Buffer
	err := Generate0619E(&buf, data, testCompany())
	if err != nil {
		t.Fatalf("Generate0619E: %v", err)
	}
	assertValidPDF(t, &buf, "BIR 0619-E")
}

func TestGenerate1601C(t *testing.T) {
	data := map[string]string{
		"period":                        "2025-01",
		"line_1_total_compensation":     "500000",
		"line_2_statutory_minimum_wage": "100000",
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
	err := Generate1601C(&buf, data, testCompany())
	if err != nil {
		t.Fatalf("Generate1601C: %v", err)
	}
	assertValidPDF(t, &buf, "BIR 1601-C")
}

func TestGenerate2550M(t *testing.T) {
	var buf bytes.Buffer
	err := Generate2550M(&buf, sampleVATData(), testCompany())
	if err != nil {
		t.Fatalf("Generate2550M: %v", err)
	}
	assertValidPDF(t, &buf, "BIR 2550M")
}

func TestGenerate2550Q(t *testing.T) {
	var buf bytes.Buffer
	err := Generate2550Q(&buf, sampleVATData(), testCompany())
	if err != nil {
		t.Fatalf("Generate2550Q: %v", err)
	}
	assertValidPDF(t, &buf, "BIR 2550Q")
}

func TestGenerate1701(t *testing.T) {
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
	err := Generate1701(&buf, data, testCompany())
	if err != nil {
		t.Fatalf("Generate1701: %v", err)
	}
	assertValidPDF(t, &buf, "BIR 1701")
}

func TestGenerate1702(t *testing.T) {
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
	err := Generate1702(&buf, data, testCompany())
	if err != nil {
		t.Fatalf("Generate1702: %v", err)
	}
	assertValidPDF(t, &buf, "BIR 1702-RT")
}

func TestGenerate2316(t *testing.T) {
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
	err := Generate2316(&buf, data, testCompany())
	if err != nil {
		t.Fatalf("Generate2316: %v", err)
	}
	assertValidPDF(t, &buf, "BIR 2316")
}

func TestGenerate2316_StillDue(t *testing.T) {
	data := map[string]string{
		"period":                         "2024",
		"employee_name":                  "Maria Santos",
		"present_employer_compensation":  "1000000",
		"present_employer_nontaxable":    "200000",
		"present_employer_taxable":       "800000",
		"total_compensation":             "1000000",
		"total_nontaxable_compensation":  "200000",
		"total_taxable_compensation":     "800000",
		"tax_due":                        "102500",
		"tax_withheld_present":           "90000",
		"total_tax_withheld":             "90000",
		"amount_refunded":                "0",
		"amount_still_due":               "12500",
	}
	var buf bytes.Buffer
	err := Generate2316(&buf, data, testCompany())
	if err != nil {
		t.Fatalf("Generate2316 StillDue: %v", err)
	}
	assertValidPDF(t, &buf, "BIR 2316 (Still Due)")
}

func TestGenerate_EmptyData(t *testing.T) {
	data := map[string]string{"period": "2025-01"}
	co := testCompany()

	// Each form should handle empty data without panicking
	forms := []struct {
		name string
		gen  func() error
	}{
		{"0619-E", func() error { var buf bytes.Buffer; return Generate0619E(&buf, data, co) }},
		{"1601-C", func() error { var buf bytes.Buffer; return Generate1601C(&buf, data, co) }},
		{"2550M", func() error { var buf bytes.Buffer; return Generate2550M(&buf, data, co) }},
		{"2550Q", func() error { var buf bytes.Buffer; return Generate2550Q(&buf, data, co) }},
		{"1701", func() error { var buf bytes.Buffer; return Generate1701(&buf, data, co) }},
		{"1702", func() error { var buf bytes.Buffer; return Generate1702(&buf, data, co) }},
		{"2316", func() error { var buf bytes.Buffer; return Generate2316(&buf, data, co) }},
	}

	for _, f := range forms {
		t.Run(f.name, func(t *testing.T) {
			if err := f.gen(); err != nil {
				t.Fatalf("%s with empty data: %v", f.name, err)
			}
		})
	}
}
