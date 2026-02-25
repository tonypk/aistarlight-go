package birpdf

import (
	"io"
)

// Generate1601C produces an official-style BIR 1601-C PDF.
// Monthly Remittance Return of Income Taxes Withheld on Compensation
func Generate1601C(w io.Writer, data map[string]string, company CompanyData) error {
	b := newBuilder(SizeFolio, "1601-C", "(Rev. 2018)")
	b.addPage()

	// Form header
	b.formHeader("1601-C",
		"Monthly Remittance Return of Income Taxes",
		"Withheld on Compensation")

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
	b.fieldRow("14", "Total Amount of Compensation Paid", getVal(data, "line_1_total_compensation", "line_14"), false)
	b.fieldRow("15", "Less: Statutory Minimum Wage / Holiday Pay / OT / NSD", getVal(data, "line_2_statutory_minimum_wage", "line_15"), false)
	b.fieldRow("16", "Less: Non-Taxable 13th Month Pay & Other Benefits (up to PHP 90,000)", getVal(data, "line_3_nontaxable_13th_month", "line_16"), false)
	b.fieldRow("17", "Less: De Minimis Benefits", getVal(data, "line_4_nontaxable_deminimis", "line_17"), false)
	b.fieldRow("18", "Less: SSS/GSIS/PHIC/Pag-IBIG Mandatory Contributions", getVal(data, "line_5_sss_gsis_phic_hdmf", "line_18"), false)
	b.fieldRow("19", "Less: Other Non-Taxable Compensation", getVal(data, "line_6_other_nontaxable", "line_19"), false)
	b.totalRow("20", "Total Non-Taxable Compensation (Sum of Lines 15 to 19)", getVal(data, "line_7_total_nontaxable", "line_20"))
	b.totalRow("21", "Taxable Compensation (Line 14 - Line 20)", getVal(data, "line_8_taxable_compensation", "line_21"))
	b.gap(2)

	b.fieldRow("22", "Tax Required to be Withheld (per Tax Table)", getVal(data, "line_9_tax_withheld", "line_22"), false)
	b.fieldRow("23", "Adjustment for Over/Under Withholding from Previous Month(s)", getVal(data, "line_10_adjustment", "line_23"), false)
	b.totalRow("24", "Total Tax to be Remitted (Line 22 +/- Line 23)", getVal(data, "line_11_total_tax_remitted", "line_24"))
	b.gap(2)

	// Part III - Penalties
	b.partHeader("Part III - Penalties")
	b.fieldRow("25", "Surcharge", getVal(data, "line_12_surcharge", "line_25"), false)
	b.fieldRow("26", "Interest", getVal(data, "line_13_interest", "line_26"), false)
	b.fieldRow("27", "Compromise Penalty", getVal(data, "line_14_compromise", "line_27"), false)
	b.totalRow("28", "Total Penalties (Sum of Lines 25 to 27)", getVal(data, "line_15_total_penalties", "line_28"))
	b.gap(2)

	b.totalRow("29", "TOTAL AMOUNT DUE (Line 24 + Line 28)",
		getVal(data, "line_16_total_amount_due", "line_29"))

	// Signature block
	b.gap(6)
	b.signatureBlock("")

	// Payment details (if space permits on same page)
	b.checkPageBreak(100)
	b.paymentDetails()

	return b.output(w)
}
