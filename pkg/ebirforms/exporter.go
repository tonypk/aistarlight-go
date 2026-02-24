package ebirforms

import (
	"fmt"
	"strings"
	"time"
)

// ExportToDAT converts tax calculation results to BIR eBIRForms DAT format.
func ExportToDAT(formType string, result map[string]string, org OrgInfo, periodStart time.Time) ([]byte, string, error) {
	switch formType {
	case "BIR_2550M":
		return export2550M(result, org, periodStart)
	default:
		return nil, "", fmt.Errorf("DAT export not yet supported for form type: %s", formType)
	}
}

// export2550M generates a BIR 2550M DAT file.
func export2550M(result map[string]string, org OrgInfo, periodStart time.Time) ([]byte, string, error) {
	// Return period: MMYYYY
	returnPeriod := fmt.Sprintf("%02d%04d", periodStart.Month(), periodStart.Year())

	// Header record
	headerValues := map[string]string{
		"record_type":     "H",
		"form_type":       "2550M",
		"tin":             FormatTIN(org.TIN),
		"branch_code":     "00000",
		"registered_name": org.RegisteredName,
		"return_period":   returnPeriod,
		"rdo_code":        org.RDOCode,
		"amended_return":  "N",
	}
	header := FormatRecord(Header2550M, headerValues)

	// Detail records for each line
	lineMapping := []struct {
		lineNum  string
		resultKey string
	}{
		{"0001", "line_1_vatable_sales"},
		{"0002", "line_2_sales_to_government"},
		{"0003", "line_3_zero_rated_sales"},
		{"0004", "line_4_exempt_sales"},
		{"0005", "line_5_total_sales"},
		{"0006", "line_6_output_vat"},
		{"006A", "line_6a_output_vat_government"},
		{"006B", "line_6b_total_output_vat"},
		{"0007", "line_7_input_vat_goods"},
		{"0008", "line_8_input_vat_capital"},
		{"0009", "line_9_input_vat_services"},
		{"0010", "line_10_input_vat_imports"},
		{"0011", "line_11_total_input_vat"},
		{"0012", "line_12_vat_payable"},
		{"0013", "line_13_less_tax_credits"},
		{"0014", "line_14_net_vat_payable"},
		{"0015", "line_15_add_penalties"},
		{"0016", "line_16_total_amount_due"},
	}

	var lines []string
	lines = append(lines, header)

	for _, lm := range lineMapping {
		amountStr := result[lm.resultKey]
		if amountStr == "" {
			amountStr = "0"
		}

		detailValues := map[string]string{
			"record_type": "D",
			"line_number": lm.lineNum,
			"amount":      FormatAmount(amountStr, 15),
		}
		lines = append(lines, FormatRecord(Detail2550M, detailValues))
	}

	content := strings.Join(lines, "\r\n") + "\r\n"
	filename := fmt.Sprintf("%s_2550M_%s.dat", sanitizeTIN(org.TIN), returnPeriod)

	return []byte(content), filename, nil
}

// sanitizeTIN removes dashes from TIN for filename.
func sanitizeTIN(tin string) string {
	return strings.ReplaceAll(tin, "-", "")
}

// Filename generates the standard BIR DAT filename.
func Filename(tin, formType string, period time.Time) string {
	cleanTIN := sanitizeTIN(tin)
	periodStr := fmt.Sprintf("%02d%04d", period.Month(), period.Year())

	// Remove "BIR_" prefix if present
	form := strings.TrimPrefix(formType, "BIR_")

	return fmt.Sprintf("%s_%s_%s.dat", cleanTIN, form, periodStr)
}
