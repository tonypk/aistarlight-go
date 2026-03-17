package ebirforms

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// lineMapping defines the mapping from BIR line numbers to result map keys.
type lineMapping struct {
	lineNum   string
	resultKey string
}

// ExportToDAT converts tax calculation results to BIR eBIRForms DAT format.
func ExportToDAT(formType string, result map[string]string, org OrgInfo, periodStart time.Time) ([]byte, string, error) {
	switch formType {
	case "BIR_2550M":
		return exportStandardForm("2550M", lines2550M, result, org, periodStart)
	case "BIR_2550Q":
		return exportStandardForm("2550Q", lines2550M, result, org, periodStart) // same lines as 2550M
	case "BIR_1601C":
		return exportStandardForm("1601C", lines1601C, result, org, periodStart)
	case "BIR_0619E":
		return exportStandardForm("0619E", lines0619E, result, org, periodStart)
	case "BIR_1701":
		return exportStandardForm("1701", lines1701, result, org, periodStart)
	case "BIR_1702":
		return exportStandardForm("1702", lines1702, result, org, periodStart)
	case "BIR_2316":
		return export2316(result, org, periodStart)
	case "BIR_2307":
		return export2307(result, org, periodStart)
	case "SAWT":
		return exportSAWT(result, org, periodStart)
	default:
		return nil, "", fmt.Errorf("DAT export not supported for form type: %s", formType)
	}
}

// --- Line mappings for standard (header + detail lines) forms ---

var lines2550M = []lineMapping{
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

var lines1601C = []lineMapping{
	{"0001", "line_1_total_compensation"},
	{"0002", "line_2_statutory_minimum_wage"},
	{"0003", "line_3_nontaxable_13th_month"},
	{"0004", "line_4_nontaxable_deminimis"},
	{"0005", "line_5_sss_gsis_phic_hdmf"},
	{"0006", "line_6_other_nontaxable"},
	{"0007", "line_7_total_nontaxable"},
	{"0008", "line_8_taxable_compensation"},
	{"0009", "line_9_tax_withheld"},
	{"0010", "line_10_adjustment"},
	{"0011", "line_11_total_tax_remitted"},
	{"0012", "line_12_surcharge"},
	{"0013", "line_13_interest"},
	{"0014", "line_14_compromise"},
	{"0015", "line_15_total_penalties"},
	{"0016", "line_16_total_amount_due"},
}

var lines0619E = []lineMapping{
	{"0001", "line_1_total_amount_of_income_payments"},
	{"0002", "line_2_total_taxes_withheld"},
	{"0003", "line_3_adjustment"},
	{"0004", "line_4_tax_still_due"},
	{"0005", "line_5_surcharge"},
	{"0006", "line_6_interest"},
	{"0007", "line_7_compromise"},
	{"0008", "line_8_total_penalties"},
	{"0009", "line_9_total_amount_due"},
}

var lines1701 = []lineMapping{
	{"0001", "gross_sales_receipts"},
	{"0002", "cost_of_sales"},
	{"0003", "gross_income_from_business"},
	{"0004", "other_taxable_income"},
	{"0005", "total_gross_income"},
	{"0006", "itemized_deductions"},
	{"0007", "osd_amount"},
	{"0008", "total_deductions"},
	{"0009", "net_taxable_income"},
	{"0010", "income_tax_due"},
	{"0011", "creditable_withholding_tax"},
	{"0012", "quarterly_payments"},
	{"0013", "other_credits"},
	{"0014", "total_tax_credits"},
	{"0015", "tax_payable"},
	{"0016", "surcharge"},
	{"0017", "interest"},
	{"0018", "compromise"},
	{"0019", "total_penalties"},
	{"0020", "total_amount_due"},
}

var lines1702 = []lineMapping{
	{"0001", "gross_income"},
	{"0002", "cost_of_sales"},
	{"0003", "gross_profit"},
	{"0004", "other_income"},
	{"0005", "total_gross_income"},
	{"0006", "itemized_deductions"},
	{"0007", "osd_amount"},
	{"0008", "total_deductions"},
	{"0009", "net_taxable_income"},
	{"0010", "rcit_amount"},
	{"0011", "mcit_amount"},
	{"0012", "income_tax_due"},
	{"0013", "excess_mcit_prior"},
	{"0014", "excess_mcit_current"},
	{"0015", "creditable_withholding_tax"},
	{"0016", "quarterly_payments"},
	{"0017", "other_credits"},
	{"0018", "total_tax_credits"},
	{"0019", "tax_payable"},
	{"0020", "surcharge"},
	{"0021", "interest"},
	{"0022", "compromise"},
	{"0023", "total_penalties"},
	{"0024", "total_amount_due"},
}

// --- Export functions ---

// exportStandardForm generates a DAT file for forms with header + line-based detail records.
func exportStandardForm(formCode string, lines []lineMapping, result map[string]string, org OrgInfo, periodStart time.Time) ([]byte, string, error) {
	returnPeriod := formatReturnPeriod(formCode, periodStart)

	headerValues := map[string]string{
		"record_type":     "H",
		"form_type":       formCode,
		"tin":             FormatTIN(org.TIN),
		"branch_code":     "00000",
		"registered_name": org.RegisteredName,
		"return_period":   returnPeriod,
		"rdo_code":        org.RDOCode,
		"amended_return":  "N",
	}
	header := FormatRecord(HeaderStandard, headerValues)

	var datLines []string
	datLines = append(datLines, header)

	for _, lm := range lines {
		amountStr := result[lm.resultKey]
		if amountStr == "" {
			amountStr = "0"
		}
		detailValues := map[string]string{
			"record_type": "D",
			"line_number": lm.lineNum,
			"amount":      FormatAmount(amountStr, 15),
		}
		datLines = append(datLines, FormatRecord(DetailStandard, detailValues))
	}

	content := strings.Join(datLines, "\r\n") + "\r\n"
	filename := fmt.Sprintf("%s_%s_%s.dat", sanitizeTIN(org.TIN), formCode, returnPeriod)

	return []byte(content), filename, nil
}

// export2316 generates a BIR 2316 DAT file (per-employee certificate).
func export2316(result map[string]string, org OrgInfo, periodStart time.Time) ([]byte, string, error) {
	returnPeriod := fmt.Sprintf("%04d", periodStart.Year())

	headerValues := map[string]string{
		"record_type":     "H",
		"form_type":       "2316",
		"tin":             FormatTIN(org.TIN),
		"branch_code":     "00000",
		"registered_name": org.RegisteredName,
		"return_period":   returnPeriod,
		"rdo_code":        org.RDOCode,
		"amended_return":  "N",
	}

	var datLines []string
	datLines = append(datLines, FormatRecord(HeaderStandard, headerValues))

	detailValues := map[string]string{
		"record_type":     "D",
		"employee_tin":    FormatTIN(result["employee_tin"]),
		"employee_name":   result["employee_name"],
		"present_comp":    FormatAmount(result["present_employer_compensation"], 15),
		"present_nt":      FormatAmount(result["present_employer_nontaxable"], 15),
		"present_taxable": FormatAmount(result["present_employer_taxable"], 15),
		"tax_due":         FormatAmount(result["tax_due"], 15),
		"tax_withheld":    FormatAmount(result["total_tax_withheld"], 15),
	}
	datLines = append(datLines, FormatRecord(Detail2316, detailValues))

	content := strings.Join(datLines, "\r\n") + "\r\n"
	filename := fmt.Sprintf("%s_2316_%s.dat", sanitizeTIN(org.TIN), returnPeriod)

	return []byte(content), filename, nil
}

// export2307 generates a BIR 2307 DAT file (certificate of creditable tax withheld).
func export2307(result map[string]string, org OrgInfo, periodStart time.Time) ([]byte, string, error) {
	returnPeriod := formatReturnPeriod("2307", periodStart)

	headerValues := map[string]string{
		"record_type":     "H",
		"form_type":       "2307",
		"tin":             FormatTIN(org.TIN),
		"branch_code":     "00000",
		"registered_name": org.RegisteredName,
		"return_period":   returnPeriod,
		"rdo_code":        org.RDOCode,
		"amended_return":  "N",
	}

	var datLines []string
	datLines = append(datLines, FormatRecord(HeaderStandard, headerValues))

	// Parse dynamic item count
	totalItems, _ := strconv.Atoi(result["total_items"])
	if totalItems == 0 {
		totalItems = 1
	}

	for i := 1; i <= totalItems; i++ {
		prefix := fmt.Sprintf("item_%d", i)
		detailValues := map[string]string{
			"record_type":   "D",
			"seq_no":        fmt.Sprintf("%d", i),
			"payee_tin":     FormatTIN(result["payee_tin"]),
			"payee_name":    result["payee_name"],
			"atc_code":      result[prefix+"_atc_code"],
			"income_amount": FormatAmount(result[prefix+"_income_amount"], 15),
			"tax_rate":      FormatAmount(result[prefix+"_tax_rate"], 6),
			"tax_withheld":  FormatAmount(result[prefix+"_tax_withheld"], 15),
		}
		datLines = append(datLines, FormatRecord(Detail2307, detailValues))
	}

	content := strings.Join(datLines, "\r\n") + "\r\n"
	filename := fmt.Sprintf("%s_2307_%s.dat", sanitizeTIN(org.TIN), returnPeriod)

	return []byte(content), filename, nil
}

// exportSAWT generates a SAWT alphalist DAT file.
func exportSAWT(result map[string]string, org OrgInfo, periodStart time.Time) ([]byte, string, error) {
	returnPeriod := formatReturnPeriod("SAWT", periodStart)

	headerValues := map[string]string{
		"record_type":     "H",
		"form_type":       "SAWT",
		"tin":             FormatTIN(org.TIN),
		"branch_code":     "00000",
		"registered_name": org.RegisteredName,
		"return_period":   returnPeriod,
		"rdo_code":        org.RDOCode,
		"amended_return":  "N",
	}

	var datLines []string
	datLines = append(datLines, FormatRecord(HeaderStandard, headerValues))

	// Parse dynamic entry count
	totalEntries, _ := strconv.Atoi(result["total_entries"])

	for i := 1; i <= totalEntries; i++ {
		prefix := fmt.Sprintf("entry_%d", i)
		detailValues := map[string]string{
			"record_type":    "D",
			"seq_no":         fmt.Sprintf("%d", i),
			"tin":            FormatTIN(result[prefix+"_tin"]),
			"branch_code":    "00000",
			"registered_name": result[prefix+"_registered_name"],
			"atc_code":       result[prefix+"_atc_code"],
			"income_payment": FormatAmount(result[prefix+"_income_payment"], 15),
			"tax_withheld":   FormatAmount(result[prefix+"_tax_withheld"], 15),
		}
		datLines = append(datLines, FormatRecord(DetailSAWT, detailValues))
	}

	content := strings.Join(datLines, "\r\n") + "\r\n"
	filename := fmt.Sprintf("%s_SAWT_%s.dat", sanitizeTIN(org.TIN), returnPeriod)

	return []byte(content), filename, nil
}

// --- Helpers ---

// formatReturnPeriod formats the period for DAT header.
// Annual forms (1701, 1702, 2316): YYYY
// Monthly/Quarterly forms: MMYYYY
func formatReturnPeriod(formCode string, periodStart time.Time) string {
	switch formCode {
	case "1701", "1702", "2316":
		return fmt.Sprintf("%04d", periodStart.Year())
	default:
		return fmt.Sprintf("%02d%04d", periodStart.Month(), periodStart.Year())
	}
}

// sanitizeTIN removes dashes from TIN for filename.
func sanitizeTIN(tin string) string {
	return strings.ReplaceAll(tin, "-", "")
}

// Filename generates the standard BIR DAT filename.
func Filename(tin, formType string, period time.Time) string {
	cleanTIN := sanitizeTIN(tin)
	form := strings.TrimPrefix(formType, "BIR_")
	periodStr := formatReturnPeriod(form, period)
	return fmt.Sprintf("%s_%s_%s.dat", cleanTIN, form, periodStr)
}
