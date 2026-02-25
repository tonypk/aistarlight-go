package birpdf

import (
	"io"
)

// Generate2550Q produces an official-style BIR 2550Q PDF.
// Quarterly Value-Added Tax Return
func Generate2550Q(w io.Writer, data map[string]string, company CompanyData) error {
	b := newBuilder(SizeLegal, "2550Q", "(Rev. 2023)")
	b.addPage()

	// Form header
	b.formHeader("2550Q",
		"Quarterly Value-Added Tax Return",
		"")

	// Part I - Background Information
	b.partHeader("Part I - Background Information")
	b.tinBoxes("TIN", company.TIN)
	b.textField("Taxpayer's Name", company.Name, 90)
	b.textField("Registered Address", company.Address, 90)
	b.textFieldHalf("RDO Code", company.RDOCode, "Line of Business", company.LineOfBusiness)
	b.textFieldHalf("Taxable Quarter", data["period"], "Calendar Year", getVal(data, "year"))
	b.checkbox("Amended Return?", getVal(data, "amended") == "true")
	b.gap(4)

	// Part II - Total Tax Due
	b.partHeader("Part II - Total Tax Due")
	b.fieldRow("15", "Vatable Sales/Receipts", getVal(data, "line_1_vatable_sales", "vatable_sales", "line_15"), false)
	b.fieldRow("16", "Sales to Government", getVal(data, "line_2_sales_to_government", "line_16"), false)
	b.fieldRow("17", "Zero-Rated Sales", getVal(data, "line_3_zero_rated_sales", "zero_rated_sales", "line_17"), false)
	b.fieldRow("18", "Exempt Sales", getVal(data, "line_4_exempt_sales", "vat_exempt_sales", "line_18"), false)
	b.totalRow("19", "Total Sales (Sum of Lines 15 to 18)", getVal(data, "line_5_total_sales", "total_sales", "line_19"))
	b.gap(2)

	b.fieldRow("20", "Output Tax on Vatable Sales (Line 15 x 12%)", getVal(data, "line_6_output_vat", "output_vat", "line_20"), false)
	b.fieldRow("21", "Output Tax on Sales to Government (Line 16 x 5%)", getVal(data, "line_6a_output_vat_government", "line_21"), false)
	b.totalRow("22", "Total Output Tax (Line 20 + Line 21)", getVal(data, "line_6b_total_output_vat", "total_output_vat", "line_22"))
	b.gap(2)

	b.fieldRow("23", "Less: Total Allowable Input Tax", getVal(data, "line_11_total_input_vat", "total_input_vat", "line_23"), false)
	b.fieldRow("24", "Net VAT Payable / (Excess Input Tax) (Line 22 - Line 23)", getVal(data, "line_14_net_vat_payable", "net_vat_payable", "line_24"), false)
	b.fieldRow("25", "Less: Tax Credits / Payments", getVal(data, "line_13_less_tax_credits", "line_25"), false)
	b.totalRow("26", "Tax Still Due / (Overpayment)", getVal(data, "line_14_net_vat_payable", "line_26"))

	// Signature block
	b.gap(6)
	b.signatureBlock("")

	// ---- Page 2: Detailed VAT Computation ----
	b.addPage()
	b.formHeader("2550Q",
		"Quarterly Value-Added Tax Return",
		"(Page 2 - Detailed Computation)")

	b.partHeader("Part IV - Detailed VAT Computation")

	// Sales/Receipts breakdown
	b.labelRow("A. Total Sales/Receipts and Output Tax", true)
	b.fieldRow("31", "Vatable Sales for the Quarter", getVal(data, "line_1_vatable_sales", "vatable_sales"), false)
	b.fieldRow("32", "Output Tax (Line 31 x 12%)", getVal(data, "line_6_output_vat", "output_vat"), false)
	b.fieldRow("33", "Sales to Government", getVal(data, "line_2_sales_to_government"), false)
	b.fieldRow("34", "Output Tax (Line 33 x 5%)", getVal(data, "line_6a_output_vat_government"), false)
	b.fieldRow("35", "Zero-Rated Sales", getVal(data, "line_3_zero_rated_sales", "zero_rated_sales"), false)
	b.fieldRow("36", "Exempt Sales", getVal(data, "line_4_exempt_sales", "vat_exempt_sales"), false)
	b.totalRow("37", "Total Sales (Sum of Lines 31, 33, 35, 36)", getVal(data, "line_5_total_sales", "total_sales"))
	b.totalRow("38", "Total Output Tax (Line 32 + Line 34)", getVal(data, "line_6b_total_output_vat", "total_output_vat"))
	b.gap(2)

	// Input Tax breakdown
	b.labelRow("B. Less: Allowable Input Tax", true)
	b.fieldRow("39", "Input Tax on Domestic Purchases of Goods", getVal(data, "line_7_input_vat_goods", "input_vat_goods"), false)
	b.fieldRow("40", "Input Tax on Domestic Purchases of Capital Goods", getVal(data, "line_8_input_vat_capital", "input_vat_capital"), false)
	b.fieldRow("41", "Input Tax on Domestic Purchases of Services", getVal(data, "line_9_input_vat_services", "input_vat_services"), false)
	b.fieldRow("42", "Input Tax on Importation of Goods", getVal(data, "line_10_input_vat_imports"), false)
	b.totalRow("43", "Total Allowable Input Tax (Sum of Lines 39 to 42)", getVal(data, "line_11_total_input_vat", "total_input_vat"))
	b.gap(2)

	// Tax payable/overpayment
	b.labelRow("C. Tax Due / (Overpayment)", true)
	b.fieldRow("44", "VAT Payable (Line 38 - Line 43)", getVal(data, "line_12_vat_payable", "vat_payable"), false)
	b.fieldRow("45", "Less: Tax Credits / Payments", getVal(data, "line_13_less_tax_credits"), false)
	b.fieldRow("46", "Net VAT Payable / (Excess Input Tax)", getVal(data, "line_14_net_vat_payable", "net_vat_payable"), false)
	b.gap(2)

	b.fieldRowNoAmt("", "Add: Penalties")
	b.fieldRow("47", "Surcharge", getVal(data, "line_15_surcharge"), false)
	b.fieldRow("48", "Interest", getVal(data, "line_15_interest"), false)
	b.fieldRow("49", "Compromise Penalty", getVal(data, "line_15_compromise"), false)
	b.totalRow("50", "TOTAL AMOUNT DUE / (Overpayment)",
		getVal(data, "line_16_total_amount_due", "net_vat_payable"))

	// Payment details
	b.gap(6)
	b.paymentDetails()

	return b.output(w)
}
