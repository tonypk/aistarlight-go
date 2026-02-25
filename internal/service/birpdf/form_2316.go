package birpdf

import (
	"io"
	"strconv"
)

// Generate2316 produces an official-style BIR 2316 PDF.
// Certificate of Compensation Payment/Tax Withheld
func Generate2316(w io.Writer, data map[string]string, company CompanyData) error {
	b := newBuilder(SizeFolio, "2316", "(Rev. 2023)")
	b.addPage()

	// Form header
	b.formHeader("2316",
		"Certificate of Compensation Payment/Tax Withheld",
		"For Compensation Payment With or Without Tax Withheld")

	// Part I - Employee Information
	b.partHeader("Part I - Employee Information")
	employeeTIN := getVal(data, "employee_tin")
	if employeeTIN != "0" {
		b.tinBoxes("Employee TIN", employeeTIN)
	}
	b.textField("Employee Name", getVal(data, "employee_name"), 90)
	b.textField("Employee Address", getVal(data, "employee_address"), 90)
	b.textFieldHalf("Date of Birth", getVal(data, "employee_dob"), "Civil Status", getVal(data, "civil_status"))
	b.gap(4)

	// Part II - Employer Information
	b.partHeader("Part II - Employer Information (Present)")
	employerName := data["employer_name"]
	if employerName == "" {
		employerName = company.Name
	}
	employerTIN := data["employer_tin"]
	if employerTIN == "" {
		employerTIN = company.TIN
	}
	if employerTIN != "" {
		b.tinBoxes("Employer TIN", employerTIN)
	}
	b.textField("Employer Name", employerName, 90)
	b.textField("Employer Address", company.Address, 90)
	b.textFieldHalf("Taxable Year", data["period"], "RDO Code", company.RDOCode)
	b.gap(4)

	// Part III - Compensation from Present Employer
	b.partHeader("Part III - Summary of Compensation")

	// Present Employer section
	b.labelRow("A. Compensation from Present Employer", true)
	b.fieldRow("21", "Gross Compensation Income", getVal(data, "present_employer_compensation"), false)
	b.fieldRow("22", "Less: Non-Taxable/Exempt Compensation", getVal(data, "present_employer_nontaxable"), false)
	b.totalRow("23", "Taxable Compensation (Present) (Line 21 - Line 22)", getVal(data, "present_employer_taxable"))
	b.gap(2)

	// Previous Employer section
	b.labelRow("B. Compensation from Previous Employer(s)", true)
	b.fieldRow("24", "Gross Compensation Income (Previous)", getVal(data, "previous_employer_compensation"), false)
	b.fieldRow("25", "Less: Non-Taxable/Exempt Compensation (Previous)", getVal(data, "previous_employer_nontaxable"), false)
	b.totalRow("26", "Taxable Compensation (Previous) (Line 24 - Line 25)", getVal(data, "previous_employer_taxable"))
	b.gap(2)

	// Total Summary
	b.labelRow("C. Total Compensation Summary", true)
	b.fieldRow("27", "Total Gross Compensation (Line 21 + Line 24)", getVal(data, "total_compensation"), false)
	b.fieldRow("28", "Total Non-Taxable Compensation (Line 22 + Line 25)", getVal(data, "total_nontaxable_compensation"), false)
	b.totalRow("29", "Total Taxable Compensation (Line 23 + Line 26)", getVal(data, "total_taxable_compensation"))
	b.gap(4)

	// Part IV - Tax Computation
	b.partHeader("Part IV - Tax Withheld & Year-End Adjustment")
	b.totalRow("30", "Tax Due (per Graduated Tax Table)", getVal(data, "tax_due"))
	b.fieldRow("31", "Tax Withheld by Present Employer", getVal(data, "tax_withheld_present"), false)
	b.fieldRow("32", "Tax Withheld by Previous Employer", getVal(data, "tax_withheld_previous"), false)
	b.totalRow("33", "Total Tax Withheld (Line 31 + Line 32)", getVal(data, "total_tax_withheld"))
	b.gap(2)

	// Year-end adjustment result
	refundStr := getVal(data, "amount_refunded")
	dueStr := getVal(data, "amount_still_due")
	refundVal, _ := strconv.ParseFloat(refundStr, 64)
	dueVal, _ := strconv.ParseFloat(dueStr, 64)

	if refundVal > 0 {
		b.totalRow("34", "Year-End Adjustment: AMOUNT REFUNDED TO EMPLOYEE", refundStr)
	} else if dueVal > 0 {
		b.totalRow("34", "Year-End Adjustment: AMOUNT STILL DUE FROM EMPLOYEE", dueStr)
	} else {
		b.fieldRow("34", "Year-End Adjustment", "0", false)
	}

	// Signature
	b.gap(6)
	b.signatureBlock("I declare, under the penalties of perjury, that the information herein stated are correct and that no amount has been paid as bribe or gift, or has been given as a donation/contribution, in violation of law.")

	return b.output(w)
}
