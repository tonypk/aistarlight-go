package service

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

func makeSourceFilesJSON(files []map[string]interface{}) []byte {
	b, _ := json.Marshal(files)
	return b
}

func TestValidateImportResult_NoSalesTransactions(t *testing.T) {
	summary := VATSummary{
		TotalInputVAT: decimal.NewFromFloat(1200),
	}
	sourceFiles := makeSourceFilesJSON([]map[string]interface{}{
		{"file_type": "combined", "row_count": 10},
	})
	txns := []map[string]interface{}{
		{"source_type": "purchase_record", "amount": 10000.0},
		{"source_type": "purchase_record", "amount": 5000.0},
	}

	warnings := ValidateImportResult(summary, sourceFiles, txns)

	found := false
	for _, w := range warnings {
		if w.Code == "NO_SALES_TRANSACTIONS" {
			found = true
			if w.Severity != "error" {
				t.Errorf("expected severity=error, got %s", w.Severity)
			}
		}
	}
	if !found {
		t.Error("expected NO_SALES_TRANSACTIONS warning")
	}
}

func TestValidateImportResult_NoPurchaseTransactions(t *testing.T) {
	summary := VATSummary{
		VatableSales:   decimal.NewFromFloat(100000),
		OutputVAT:      decimal.NewFromFloat(12000),
		TotalOutputVAT: decimal.NewFromFloat(12000),
	}
	sourceFiles := makeSourceFilesJSON([]map[string]interface{}{
		{"file_type": "combined", "row_count": 5},
	})
	txns := []map[string]interface{}{
		{"source_type": "sales_record", "amount": 50000.0},
		{"source_type": "sales_record", "amount": 50000.0},
	}

	warnings := ValidateImportResult(summary, sourceFiles, txns)

	found := false
	for _, w := range warnings {
		if w.Code == "NO_PURCHASE_TRANSACTIONS" {
			found = true
			if w.Severity != "error" {
				t.Errorf("expected severity=error, got %s", w.Severity)
			}
		}
	}
	if !found {
		t.Error("expected NO_PURCHASE_TRANSACTIONS warning")
	}
}

func TestValidateImportResult_OutputVATMismatch(t *testing.T) {
	// Vatable sales = 100000, expected output VAT = 12000, actual = 15000
	summary := VATSummary{
		VatableSales:   decimal.NewFromFloat(100000),
		OutputVAT:      decimal.NewFromFloat(15000),
		TotalOutputVAT: decimal.NewFromFloat(15000),
	}
	sourceFiles := makeSourceFilesJSON([]map[string]interface{}{
		{"file_type": "sales_record", "row_count": 3},
	})
	txns := []map[string]interface{}{
		{"source_type": "sales_record", "amount": 100000.0},
	}

	warnings := ValidateImportResult(summary, sourceFiles, txns)

	found := false
	for _, w := range warnings {
		if w.Code == "OUTPUT_VAT_MISMATCH" {
			found = true
			if w.Severity != "warning" {
				t.Errorf("expected severity=warning, got %s", w.Severity)
			}
		}
	}
	if !found {
		t.Error("expected OUTPUT_VAT_MISMATCH warning")
	}
}

func TestValidateImportResult_OutputVATMatchesNoWarning(t *testing.T) {
	// Correct: 100000 × 12% = 12000
	summary := VATSummary{
		VatableSales:   decimal.NewFromFloat(100000),
		OutputVAT:      decimal.NewFromFloat(12000),
		TotalOutputVAT: decimal.NewFromFloat(12000),
	}
	sourceFiles := makeSourceFilesJSON([]map[string]interface{}{
		{"file_type": "sales_record", "row_count": 3},
	})
	txns := []map[string]interface{}{
		{"source_type": "sales_record", "amount": 100000.0},
	}

	warnings := ValidateImportResult(summary, sourceFiles, txns)

	for _, w := range warnings {
		if w.Code == "OUTPUT_VAT_MISMATCH" {
			t.Error("should not have OUTPUT_VAT_MISMATCH when VAT matches")
		}
	}
}

func TestValidateImportResult_OnlyInputVAT(t *testing.T) {
	summary := VATSummary{
		TotalInputVAT:  decimal.NewFromFloat(6000),
		TotalOutputVAT: decimal.Zero,
		InputVATGoods:   decimal.NewFromFloat(6000),
	}
	sourceFiles := makeSourceFilesJSON([]map[string]interface{}{
		{"file_type": "purchase_record", "row_count": 5},
	})
	txns := []map[string]interface{}{
		{"source_type": "purchase_record", "amount": 50000.0},
	}

	warnings := ValidateImportResult(summary, sourceFiles, txns)

	found := false
	for _, w := range warnings {
		if w.Code == "ONLY_INPUT_VAT" {
			found = true
		}
	}
	if !found {
		t.Error("expected ONLY_INPUT_VAT warning")
	}
}

func TestValidateImportResult_ZeroAmountTransactions(t *testing.T) {
	summary := VATSummary{
		VatableSales:   decimal.NewFromFloat(50000),
		OutputVAT:      decimal.NewFromFloat(6000),
		TotalOutputVAT: decimal.NewFromFloat(6000),
		TotalSales:     decimal.NewFromFloat(50000),
	}
	sourceFiles := makeSourceFilesJSON([]map[string]interface{}{
		{"file_type": "sales_record", "row_count": 5},
	})
	txns := []map[string]interface{}{
		{"source_type": "sales_record", "amount": 50000.0},
		{"source_type": "sales_record", "amount": 0.0},
		{"source_type": "sales_record", "amount": 0.0},
	}

	warnings := ValidateImportResult(summary, sourceFiles, txns)

	found := false
	for _, w := range warnings {
		if w.Code == "ZERO_AMOUNT_TRANSACTIONS" {
			found = true
			if w.Details["zero_count"] != 2 {
				t.Errorf("expected zero_count=2, got %v", w.Details["zero_count"])
			}
		}
	}
	if !found {
		t.Error("expected ZERO_AMOUNT_TRANSACTIONS warning")
	}
}

func TestValidateImportResult_AllAmountsZero(t *testing.T) {
	summary := VATSummary{
		TotalSales:    decimal.Zero,
		TotalInputVAT: decimal.Zero,
	}
	sourceFiles := makeSourceFilesJSON([]map[string]interface{}{
		{"file_type": "sales_record", "row_count": 3},
	})
	txns := []map[string]interface{}{
		{"source_type": "sales_record", "amount": 0.0},
		{"source_type": "sales_record", "amount": 0.0},
	}

	warnings := ValidateImportResult(summary, sourceFiles, txns)

	found := false
	for _, w := range warnings {
		if w.Code == "ALL_AMOUNTS_ZERO" {
			found = true
			if w.Severity != "error" {
				t.Errorf("expected severity=error, got %s", w.Severity)
			}
		}
	}
	if !found {
		t.Error("expected ALL_AMOUNTS_ZERO warning")
	}
}

func TestValidateImportResult_HealthyImportNoWarnings(t *testing.T) {
	// A healthy import: sales and purchases with correct VAT
	summary := VATSummary{
		VatableSales:   decimal.NewFromFloat(100000),
		OutputVAT:      decimal.NewFromFloat(12000),
		TotalOutputVAT: decimal.NewFromFloat(12000),
		TotalSales:     decimal.NewFromFloat(100000),
		InputVATGoods:   decimal.NewFromFloat(6000),
		TotalInputVAT:  decimal.NewFromFloat(6000),
	}
	sourceFiles := makeSourceFilesJSON([]map[string]interface{}{
		{"file_type": "combined", "row_count": 5},
	})
	txns := []map[string]interface{}{
		{"source_type": "sales_record", "amount": 50000.0},
		{"source_type": "sales_record", "amount": 50000.0},
		{"source_type": "purchase_record", "amount": 25000.0},
		{"source_type": "purchase_record", "amount": 25000.0},
	}

	warnings := ValidateImportResult(summary, sourceFiles, txns)

	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings for healthy import, got %d: %+v", len(warnings), warnings)
	}
}

func TestValidateImportResult_SalesSheetNameDetection(t *testing.T) {
	summary := VATSummary{
		TotalInputVAT: decimal.NewFromFloat(6000),
		InputVATGoods:  decimal.NewFromFloat(6000),
	}
	// Sheet name contains "SLS" (BIR SLSP format)
	sourceFiles := makeSourceFilesJSON([]map[string]interface{}{
		{"file_type": "purchase_record", "sheet_name": "SLP", "row_count": 3},
		{"file_type": "purchase_record", "sheet_name": "SLS", "row_count": 3},
	})
	txns := []map[string]interface{}{
		{"source_type": "purchase_record", "amount": 50000.0},
	}

	warnings := ValidateImportResult(summary, sourceFiles, txns)

	found := false
	for _, w := range warnings {
		if w.Code == "NO_SALES_TRANSACTIONS" {
			found = true
		}
	}
	if !found {
		t.Error("expected NO_SALES_TRANSACTIONS warning when SLS sheet exists but no sales transactions")
	}
}
