package birpdf

import (
	"io"
)

// Generate0619E produces an official-style BIR 0619-E PDF.
// Monthly Remittance Form for Creditable Income Taxes Withheld (Expanded)
func Generate0619E(w io.Writer, data map[string]string, company CompanyData) error {
	b := newBuilder(SizeLetter, "0619-E", "(Rev. 2018)")
	b.addPage()

	// Form header
	b.formHeader("0619-E",
		"Monthly Remittance Form",
		"For Creditable Income Taxes Withheld (Expanded)")

	// Part I - Background Information
	b.partHeader("Part I - Background Information")
	b.tinBoxes("TIN", company.TIN)
	b.textField("Taxpayer's Name", company.Name, 90)
	b.textField("Registered Address", company.Address, 90)
	b.textFieldHalf("RDO Code", company.RDOCode, "Line of Business", company.LineOfBusiness)
	b.textFieldHalf("For the Month", data["period"], "Year", getVal(data, "year"))
	b.checkbox("Amended Return?", getVal(data, "amended") == "true")
	b.gap(4)

	// Part II - Computation of Tax
	b.partHeader("Part II - Computation of Tax")
	b.fieldRow("14", "Total Amount of Income Payments", getVal(data, "line_1_total_amount_of_income_payments", "line_14"), false)
	b.fieldRow("15", "Total Taxes Withheld for the Month", getVal(data, "line_2_total_taxes_withheld", "line_15"), false)
	b.fieldRow("16", "Adjustment for Over-Remittance from Previous Month(s)", getVal(data, "line_3_adjustment", "line_16"), false)
	b.totalRow("17", "Tax Still Due / (Overpayment) (Line 15 - Line 16)", getVal(data, "line_4_tax_still_due", "line_17"))
	b.gap(2)

	// Part III - Penalties
	b.fieldRowNoAmt("", "Add: Penalties")
	b.fieldRow("18", "Surcharge", getVal(data, "line_5_surcharge", "line_18"), false)
	b.fieldRow("19", "Interest", getVal(data, "line_6_interest", "line_19"), false)
	b.fieldRow("20", "Compromise Penalty", getVal(data, "line_7_compromise", "line_20"), false)
	b.totalRow("21", "TOTAL AMOUNT DUE / (Overpayment) (Line 17 + Lines 18 to 20)",
		getVal(data, "line_9_total_amount_due", "line_8_total_penalties", "line_21"))

	// Signature block
	b.gap(6)
	b.signatureBlock("")

	// Payment details
	b.checkPageBreak(100)
	b.paymentDetails()

	return b.output(w)
}
