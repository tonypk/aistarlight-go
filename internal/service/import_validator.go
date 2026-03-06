package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/pkg/birforms"
)

// ValidationWarning represents a post-import validation issue detected by the system.
type ValidationWarning struct {
	Code     string                 `json:"code"`
	Severity string                 `json:"severity"` // "error", "warning", "info"
	Message  string                 `json:"message"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

// ValidateImportResult runs sanity checks on the generated VAT summary and
// imported transactions to catch common data issues.
// sourceFilesJSON is the raw JSON from session.SourceFiles.
func ValidateImportResult(summary VATSummary, sourceFilesJSON []byte, transactions []map[string]interface{}) []ValidationWarning {
	var warnings []ValidationWarning

	// Parse source files metadata
	var sourceFiles []map[string]interface{}
	if len(sourceFilesJSON) > 0 {
		_ = json.Unmarshal(sourceFilesJSON, &sourceFiles)
	}

	// Count transactions by source type
	salesCount := 0
	purchaseCount := 0
	zeroAmountCount := 0
	for _, tx := range transactions {
		st := toString(tx["source_type"])
		switch st {
		case "sales_record":
			salesCount++
		case "purchase_record":
			purchaseCount++
		}
		if parseAmount(tx["amount"]) == 0 {
			zeroAmountCount++
		}
	}

	// --- Rule 1: Sales sheet detected but no sales transactions ---
	hasSalesSource := false
	hasPurchaseSource := false
	for _, f := range sourceFiles {
		ft := strings.ToLower(toString(f["file_type"]))
		sn := strings.ToLower(toString(f["sheet_name"]))

		if ft == "sales_record" || strings.Contains(sn, "sales") || strings.Contains(sn, "sls") {
			hasSalesSource = true
		}
		if ft == "purchase_record" || strings.Contains(sn, "purchase") || strings.Contains(sn, "slp") {
			hasPurchaseSource = true
		}
		if ft == "combined" {
			hasSalesSource = true
			hasPurchaseSource = true
		}
	}

	if hasSalesSource && salesCount == 0 {
		warnings = append(warnings, ValidationWarning{
			Code:     "NO_SALES_TRANSACTIONS",
			Severity: "error",
			Message:  "The uploaded file contains sales data, but no sales transactions were captured. Sales amounts will show as 0. Please check if the correct data category was selected.",
			Details: map[string]interface{}{
				"expected": "sales_record",
				"actual":   0,
			},
		})
	}

	if hasPurchaseSource && purchaseCount == 0 {
		warnings = append(warnings, ValidationWarning{
			Code:     "NO_PURCHASE_TRANSACTIONS",
			Severity: "error",
			Message:  "The uploaded file contains purchase data, but no purchase transactions were captured. Input VAT will show as 0. Please check if the correct data category was selected.",
			Details: map[string]interface{}{
				"expected": "purchase_record",
				"actual":   0,
			},
		})
	}

	// --- Rule 2: Output VAT ≠ Vatable Sales × 12% ---
	if !summary.VatableSales.IsZero() {
		expectedOutputVAT := summary.VatableSales.Mul(birforms.VATRate)
		outputVATDiff := summary.OutputVAT.Sub(expectedOutputVAT).Abs()
		// Allow 1 peso tolerance for rounding
		tolerance := decimal.NewFromFloat(1.0)
		if outputVATDiff.GreaterThan(tolerance) {
			warnings = append(warnings, ValidationWarning{
				Code:     "OUTPUT_VAT_MISMATCH",
				Severity: "warning",
				Message: fmt.Sprintf(
					"Output VAT (%s) doesn't match expected Vatable Sales × 12%% (%s). Difference: %s. This may indicate incorrect amount extraction or mixed VAT types.",
					summary.OutputVAT.StringFixed(2),
					expectedOutputVAT.StringFixed(2),
					outputVATDiff.StringFixed(2),
				),
				Details: map[string]interface{}{
					"output_vat":          summary.OutputVAT.StringFixed(2),
					"expected_output_vat": expectedOutputVAT.StringFixed(2),
					"difference":          outputVATDiff.StringFixed(2),
				},
			})
		}
	}

	// --- Rule 3: Only Input VAT present, no Output VAT ---
	if !summary.TotalInputVAT.IsZero() && summary.TotalOutputVAT.IsZero() && salesCount == 0 {
		warnings = append(warnings, ValidationWarning{
			Code:     "ONLY_INPUT_VAT",
			Severity: "warning",
			Message:  "Only purchase transactions (Input VAT) were captured with no sales data. If this file should also contain sales, the sales sheet may not have been processed.",
			Details: map[string]interface{}{
				"total_input_vat":  summary.TotalInputVAT.StringFixed(2),
				"total_output_vat": "0.00",
				"purchase_count":   purchaseCount,
				"sales_count":      salesCount,
			},
		})
	}

	// --- Rule 4: Zero amount transactions ---
	if zeroAmountCount > 0 {
		pct := float64(zeroAmountCount) / float64(len(transactions)) * 100
		severity := "info"
		if pct > 50 {
			severity = "warning"
		}
		warnings = append(warnings, ValidationWarning{
			Code:     "ZERO_AMOUNT_TRANSACTIONS",
			Severity: severity,
			Message:  fmt.Sprintf("%d of %d transaction(s) have zero amounts (%.0f%%). These may affect VAT computation accuracy. Check if the correct amount column was mapped.", zeroAmountCount, len(transactions), pct),
			Details: map[string]interface{}{
				"zero_count": zeroAmountCount,
				"total":      len(transactions),
				"percentage": fmt.Sprintf("%.1f%%", pct),
			},
		})
	}

	// --- Rule 5: Transactions exist but total sales AND total input VAT are both zero ---
	if len(transactions) > 0 && summary.TotalSales.IsZero() && summary.TotalInputVAT.IsZero() {
		warnings = append(warnings, ValidationWarning{
			Code:     "ALL_AMOUNTS_ZERO",
			Severity: "error",
			Message:  fmt.Sprintf("%d transactions imported but both total sales and total Input VAT are zero. The amount columns may not have been mapped correctly.", len(transactions)),
			Details: map[string]interface{}{
				"transaction_count": len(transactions),
			},
		})
	}

	return warnings
}
