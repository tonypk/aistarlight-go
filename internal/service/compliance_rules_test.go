package service

import (
	"testing"

	"github.com/tonypk/aistarlight-go/pkg/birforms"
)

func TestRunAllChecks_AllPass(t *testing.T) {
	data := map[string]interface{}{
		"vatable_sales":         100000.0,
		"sales_to_government":   10000.0,
		"zero_rated_sales":      5000.0,
		"vat_exempt_sales":      2000.0,
		"total_sales":           117000.0,
		"output_vat":            12000.0,
		"output_vat_government": 500.0,
		"total_input_vat":       8000.0,
		"total_amount_due":      4000.0,
		"tin_number":            "123-456-789-000",
		"period":                "2030-01",
	}

	results := RunAllChecks(data, birforms.FormBIR2550M, nil, nil)
	for _, r := range results {
		if !r.Passed && r.Severity == "critical" {
			t.Errorf("Critical check %q failed: %s", r.CheckID, r.Message)
		}
	}
}

func TestCheckRequiredFields_Missing(t *testing.T) {
	data := map[string]interface{}{
		"vatable_sales": 100.0,
		// missing: sales_to_government, zero_rated_sales, exempt_sales, output_vat, total_input_vat
	}

	result := checkRequiredFields(data, birforms.FormBIR2550M)
	if result.Passed {
		t.Error("Expected checkRequiredFields to fail for missing fields")
	}
	if result.Severity != "critical" {
		t.Errorf("Expected severity critical, got %s", result.Severity)
	}
}

func TestCheckRequiredFields_1601C(t *testing.T) {
	data := map[string]interface{}{
		"line_1_total_compensation":  50000.0,
		"line_9_tax_withheld":        5000.0,
		"line_11_total_tax_remitted": 5000.0,
	}

	result := checkRequiredFields(data, birforms.FormBIR1601C)
	if !result.Passed {
		t.Errorf("Expected pass for 1601C with all fields, got: %s", result.Message)
	}
}

func TestCheckCrossFieldConsistency(t *testing.T) {
	// Sum matches
	data := map[string]interface{}{
		"vatable_sales":       80000.0,
		"sales_to_government": 10000.0,
		"zero_rated_sales":    5000.0,
		"vat_exempt_sales":    5000.0,
		"total_sales":         100000.0,
	}
	result := checkCrossFieldConsistency(data, birforms.FormBIR2550M)
	if !result.Passed {
		t.Errorf("Expected pass: %s", result.Message)
	}

	// Sum doesn't match
	data["total_sales"] = 50000.0
	result = checkCrossFieldConsistency(data, birforms.FormBIR2550M)
	if result.Passed {
		t.Error("Expected fail for inconsistent totals")
	}
}

func TestCheckCrossFieldConsistency_NotApplicable(t *testing.T) {
	data := map[string]interface{}{}
	result := checkCrossFieldConsistency(data, birforms.FormBIR1601C)
	if !result.Passed {
		t.Error("Expected pass for non-VAT form")
	}
}

func TestCheckOutputVATAccuracy(t *testing.T) {
	// 12% of 100000 = 12000
	data := map[string]interface{}{
		"vatable_sales": 100000.0,
		"output_vat":    12000.0,
	}
	result := checkOutputVATAccuracy(data, birforms.FormBIR2550M)
	if !result.Passed {
		t.Errorf("Expected pass: %s", result.Message)
	}

	// Wrong VAT
	data["output_vat"] = 5000.0
	result = checkOutputVATAccuracy(data, birforms.FormBIR2550M)
	if result.Passed {
		t.Error("Expected fail for wrong output VAT")
	}
}

func TestCheckGovernmentVATRate(t *testing.T) {
	// 5% of 100000 = 5000
	data := map[string]interface{}{
		"sales_to_government":  100000.0,
		"output_vat_government": 5000.0,
	}
	result := checkGovernmentVATRate(data, birforms.FormBIR2550M)
	if !result.Passed {
		t.Errorf("Expected pass: %s", result.Message)
	}

	// Zero govt sales → pass
	data["sales_to_government"] = 0.0
	result = checkGovernmentVATRate(data, birforms.FormBIR2550M)
	if !result.Passed {
		t.Error("Expected pass for zero govt sales")
	}
}

func TestCheckAmountRanges(t *testing.T) {
	// Normal range
	data := map[string]interface{}{
		"vatable_sales": 500000.0,
		"output_vat":    60000.0,
	}
	result := checkAmountRanges(data, birforms.FormBIR2550M)
	if !result.Passed {
		t.Errorf("Expected pass: %s", result.Message)
	}

	// Negative amount
	data["vatable_sales"] = -100.0
	result = checkAmountRanges(data, birforms.FormBIR2550M)
	if result.Passed {
		t.Error("Expected fail for negative amount")
	}

	// Exceeds max
	data["vatable_sales"] = 9999999999.0
	result = checkAmountRanges(data, birforms.FormBIR2550M)
	if result.Passed {
		t.Error("Expected fail for amount exceeding max")
	}
}

func TestCheckTINFormat(t *testing.T) {
	tests := []struct {
		tin    string
		passed bool
	}{
		{"123-456-789-000", true},
		{"999-999-999-999", true},
		{"", true},         // No TIN is OK
		{"12345678900", false}, // No dashes
		{"123-456-789", false}, // Too short
		{"abc-def-ghi-jkl", false},
	}

	for _, tt := range tests {
		t.Run(tt.tin, func(t *testing.T) {
			data := map[string]interface{}{"tin_number": tt.tin}
			result := checkTINFormat(data, birforms.FormBIR2550M)
			if result.Passed != tt.passed {
				t.Errorf("checkTINFormat(%q) passed=%v, want %v: %s", tt.tin, result.Passed, tt.passed, result.Message)
			}
		})
	}
}

func TestCheckPeriodOverPeriodAnomaly(t *testing.T) {
	// < 50% change → pass
	data := map[string]interface{}{"total_sales": 110000.0}
	prior := map[string]interface{}{"total_sales": 100000.0}
	result := checkPeriodOverPeriodAnomaly(data, birforms.FormBIR2550M, prior)
	if !result.Passed {
		t.Errorf("Expected pass for 10%% change: %s", result.Message)
	}

	// > 50% change → fail
	data["total_sales"] = 200000.0
	result = checkPeriodOverPeriodAnomaly(data, birforms.FormBIR2550M, prior)
	if result.Passed {
		t.Error("Expected fail for 100% change")
	}

	// No prior data → pass
	result = checkPeriodOverPeriodAnomaly(data, birforms.FormBIR2550M, nil)
	if !result.Passed {
		t.Error("Expected pass when no prior data")
	}
}

func TestCheckDuplicateReport(t *testing.T) {
	data := map[string]interface{}{"period": "2024-01"}
	existing := []map[string]interface{}{
		{"report_type": birforms.FormBIR2550M, "period": "2024-01", "status": "draft"},
		{"report_type": birforms.FormBIR2550M, "period": "2024-01", "status": "review"},
	}

	result := checkDuplicateReport(data, birforms.FormBIR2550M, existing)
	if result.Passed {
		t.Error("Expected fail for duplicate reports")
	}

	// Archived reports don't count
	existing[1]["status"] = "archived"
	result = checkDuplicateReport(data, birforms.FormBIR2550M, existing)
	if !result.Passed {
		t.Errorf("Expected pass when second report is archived: %s", result.Message)
	}
}

func TestCheckCapitalGoodsThreshold(t *testing.T) {
	// Below threshold
	data := map[string]interface{}{"input_vat_capital": 500000.0}
	result := checkCapitalGoodsThreshold(data, birforms.FormBIR2550M)
	if !result.Passed {
		t.Errorf("Expected pass: %s", result.Message)
	}

	// Above threshold
	data["input_vat_capital"] = 1500000.0
	result = checkCapitalGoodsThreshold(data, birforms.FormBIR2550M)
	if result.Passed {
		t.Error("Expected fail for capital goods above 1M")
	}
}

func TestCheckZeroFiling(t *testing.T) {
	// Non-zero → pass
	data := map[string]interface{}{"total_amount_due": 1000.0, "income_tax_due": 500.0}
	result := checkZeroFilingWarning(data, birforms.FormBIR2550M)
	if !result.Passed {
		t.Errorf("Expected pass: %s", result.Message)
	}

	// Zero filing
	data = map[string]interface{}{"total_amount_due": 0.0, "income_tax_due": 0.0}
	result = checkZeroFilingWarning(data, birforms.FormBIR2550M)
	if result.Passed {
		t.Error("Expected fail for zero filing")
	}
}
