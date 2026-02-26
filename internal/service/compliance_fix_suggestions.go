package service

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/pkg/birforms"
)

// ComplianceBlockedError implements the error interface and carries structured
// fix suggestions when a compliance gate blocks a status transition.
type ComplianceBlockedError struct {
	Score        int              `json:"score"`
	Threshold    int              `json:"threshold"`
	FailedChecks []FailedCheckFix `json:"failed_checks"`
	Summary      string           `json:"summary"`
}

// FailedCheckFix pairs a failed compliance check with a concrete fix suggestion.
type FailedCheckFix struct {
	CheckID       string `json:"check_id"`
	CheckName     string `json:"check_name"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	FixSuggestion string `json:"fix_suggestion"`
	FixAction     string `json:"fix_action"`                // "edit_field" | "add_data" | "review"
	TargetField   string `json:"target_field,omitempty"`
}

func (e *ComplianceBlockedError) Error() string {
	return fmt.Sprintf("compliance score %d/100 is below threshold (%d)", e.Score, e.Threshold)
}

// GenerateFixSuggestions creates concrete fix suggestions for each failed check.
func GenerateFixSuggestions(checks []CheckResult, calcData map[string]interface{}) []FailedCheckFix {
	var fixes []FailedCheckFix
	for _, c := range checks {
		if c.Passed {
			continue
		}
		fix := FailedCheckFix{
			CheckID:   c.CheckID,
			CheckName: c.CheckName,
			Severity:  c.Severity,
			Message:   c.Message,
		}
		generateFix(&fix, calcData)
		fixes = append(fixes, fix)
	}
	return fixes
}

func generateFix(fix *FailedCheckFix, calcData map[string]interface{}) {
	switch fix.CheckID {
	case "required_fields":
		fix.FixAction = "add_data"
		// Extract missing field names from message
		if idx := strings.Index(fix.Message, ": "); idx >= 0 {
			fields := fix.Message[idx+2:]
			fix.FixSuggestion = fmt.Sprintf("Add the missing fields: %s. These are required for this form type.", fields)
		} else {
			fix.FixSuggestion = "Add all required fields for this form type."
		}

	case "cross_field":
		fix.FixAction = "edit_field"
		fix.TargetField = "total_sales"
		vatable := toDecimal(calcData["vatable_sales"])
		govt := toDecimal(calcData["sales_to_government"])
		zeroRated := toDecimal(calcData["zero_rated_sales"])
		exempt := toDecimal(calcData["exempt_sales"])
		expected := vatable.Add(govt).Add(zeroRated).Add(exempt)
		fix.FixSuggestion = fmt.Sprintf(
			"Set total_sales to ₱%s (sum: vatable_sales + sales_to_government + zero_rated_sales + exempt_sales)",
			expected.StringFixed(2),
		)

	case "output_vat":
		fix.FixAction = "edit_field"
		fix.TargetField = "output_vat"
		vatable := toDecimal(calcData["vatable_sales"])
		expected := vatable.Mul(birforms.VATRate)
		fix.FixSuggestion = fmt.Sprintf(
			"Set output_vat to ₱%s (vatable_sales ₱%s × 12%%)",
			expected.StringFixed(2), vatable.StringFixed(2),
		)

	case "govt_vat":
		fix.FixAction = "edit_field"
		fix.TargetField = "output_vat_government"
		govtSales := toDecimal(calcData["sales_to_government"])
		expected := govtSales.Mul(birforms.GovtVATRate)
		fix.FixSuggestion = fmt.Sprintf(
			"Set output_vat_government to ₱%s (sales_to_government ₱%s × 5%%)",
			expected.StringFixed(2), govtSales.StringFixed(2),
		)

	case "amount_ranges":
		fix.FixAction = "review"
		if strings.Contains(fix.Message, "negative") {
			fix.FixSuggestion = "Review the flagged field — negative amounts are not allowed. Check if the sign is correct or if a credit note should be used instead."
		} else {
			fix.FixSuggestion = "Review the flagged field — the amount exceeds the maximum (₱999,999,999). Verify the value is correct and not a data entry error."
		}

	case "tin_format":
		fix.FixAction = "edit_field"
		fix.TargetField = "tin_number"
		fix.FixSuggestion = "Update TIN to match the format ###-###-###-### (12 digits with dashes)."

	case "filing_deadline":
		fix.FixAction = "review"
		fix.FixSuggestion = "Filing deadline has passed. File as soon as possible to minimize penalties (25% surcharge + 20%/year interest per BIR regulations)."

	case "period_anomaly":
		fix.FixAction = "review"
		fix.FixSuggestion = "This period shows a significant change (>50%) compared to the prior period. Review the data to confirm the change is legitimate and not a data entry error."

	case "duplicate":
		fix.FixAction = "review"
		fix.FixSuggestion = "Multiple active reports exist for the same form type and period. Archive or delete duplicate reports before approving."

	case "capital_goods":
		fix.FixAction = "review"
		inputVATCapital := toDecimal(calcData["input_vat_capital"])
		months := inputVATCapital.Div(decimal.NewFromInt(60)).Round(2)
		fix.FixSuggestion = fmt.Sprintf(
			"Input VAT on capital goods exceeds ₱1M. Per RR 16-2005, amortize over 60 months (≈₱%s/month). Adjust input_vat_capital accordingly.",
			months.StringFixed(2),
		)

	case "zero_filing":
		fix.FixAction = "review"
		fix.FixSuggestion = "This is a nil/zero return filing. Confirm this is intentional — if the business had transactions this period, the amounts may be missing."

	default:
		fix.FixAction = "review"
		fix.FixSuggestion = "Review and correct the issue described above."
	}
}
