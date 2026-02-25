package birpdf

import (
	"io"
)

// Generate2550M produces an official-style BIR 2550M PDF.
// Monthly Value-Added Tax Declaration
func Generate2550M(w io.Writer, data map[string]string, company CompanyData) error {
	b := newBuilder(SizeLegal, "2550M", "(Rev. 2023)")
	b.addPage()

	// Form header
	b.formHeader("2550M",
		"Monthly Value-Added Tax Declaration",
		"")

	// Part I - Background Information
	b.partHeader("Part I - Background Information")
	b.tinBoxes("TIN", company.TIN)
	b.textField("Taxpayer's Name", company.Name, 90)
	b.textField("Registered Address", company.Address, 90)
	b.textFieldHalf("RDO Code", company.RDOCode, "Line of Business", company.LineOfBusiness)
	b.textFieldHalf("Taxable Period (Month/Year)", data["period"], "Amendment?", boolToYesNo(getVal(data, "amended")))
	b.checkbox("VAT Taxpayer?", true)
	b.gap(4)

	// Part II - Computation of Tax
	b.partHeader("Part II - Computation of Tax")

	// A. Sales/Receipts
	b.labelRow("A. Total Sales/Receipts and Output Tax", true)
	b.fieldRow("12", "Vatable Sales/Receipts", getVal(data, "line_1_vatable_sales", "vatable_sales", "line_12"), false)
	b.fieldRow("12A", "Output Tax (Line 12 x 12%)", getVal(data, "line_6_output_vat", "output_vat", "line_12a"), false)
	b.fieldRow("13", "Sales to Government", getVal(data, "line_2_sales_to_government", "line_13"), false)
	b.fieldRow("13A", "Output Tax (Line 13 x 5%)", getVal(data, "line_6a_output_vat_government", "line_13a"), false)
	b.fieldRow("14", "Zero-Rated Sales", getVal(data, "line_3_zero_rated_sales", "zero_rated_sales", "line_14"), false)
	b.fieldRow("15", "Exempt Sales", getVal(data, "line_4_exempt_sales", "vat_exempt_sales", "line_15"), false)
	b.totalRow("16", "Total Sales/Receipts (Sum of Lines 12 to 15)", getVal(data, "line_5_total_sales", "total_sales", "line_16"))
	b.totalRow("17", "Total Output Tax (Line 12A + Line 13A)", getVal(data, "line_6b_total_output_vat", "total_output_vat", "line_17"))
	b.gap(2)

	// B. Allowable Input Tax
	b.labelRow("B. Less: Allowable Input Tax", true)
	b.fieldRow("18", "Input Tax on Domestic Purchases of Goods", getVal(data, "line_7_input_vat_goods", "input_vat_goods", "line_18"), false)
	b.fieldRow("19", "Input Tax on Domestic Purchases of Capital Goods", getVal(data, "line_8_input_vat_capital", "input_vat_capital", "line_19"), false)
	b.fieldRow("20", "Input Tax on Domestic Purchases of Services", getVal(data, "line_9_input_vat_services", "input_vat_services", "line_20"), false)
	b.fieldRow("21", "Input Tax on Importation of Goods", getVal(data, "line_10_input_vat_imports", "line_21"), false)
	b.totalRow("22", "Total Allowable Input Tax (Sum of Lines 18 to 21)", getVal(data, "line_11_total_input_vat", "total_input_vat", "line_22"))
	b.gap(2)

	// C. Tax Due
	b.labelRow("C. Tax Due / (Overpayment)", true)
	b.fieldRow("23", "VAT Payable (Line 17 - Line 22)", getVal(data, "line_12_vat_payable", "vat_payable", "line_23"), false)
	b.fieldRow("24", "Less: Tax Credits / Payments", getVal(data, "line_13_less_tax_credits", "line_24"), false)
	b.fieldRow("25", "Net VAT Payable / (Excess Input Tax)", getVal(data, "line_14_net_vat_payable", "net_vat_payable", "line_25"), false)
	b.gap(2)

	// D. Penalties
	b.fieldRowNoAmt("", "Add: Penalties")
	b.fieldRow("26", "Surcharge", getVal(data, "line_15_surcharge", "line_26"), false)
	b.fieldRow("27", "Interest", getVal(data, "line_15_interest", "line_27"), false)
	b.fieldRow("28", "Compromise Penalty", getVal(data, "line_15_compromise", "line_28"), false)

	b.totalRow("29", "TOTAL AMOUNT DUE / (Overpayment)",
		getVal(data, "line_16_total_amount_due", "net_vat_payable", "line_29"))

	// Signature
	b.gap(6)
	b.signatureBlock("")

	return b.output(w)
}

func boolToYesNo(val string) string {
	if val == "true" || val == "yes" || val == "1" {
		return "Yes"
	}
	return "No"
}
