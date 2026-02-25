package birpdf

import (
	"io"
)

// Generate1701 produces an official-style BIR 1701 PDF.
// Annual Income Tax Return for Self-Employed Individuals, Estates, and Trusts
func Generate1701(w io.Writer, data map[string]string, company CompanyData) error {
	b := newBuilder(SizeFolio, "1701", "(Rev. 2023)")
	b.addPage()

	deductionMethod := getVal(data, "deduction_method")
	if deductionMethod == "0" || deductionMethod == "" {
		deductionMethod = "OSD"
	}

	// Form header
	b.formHeader("1701",
		"Annual Income Tax Return",
		"For Self-Employed Individuals, Estates, and Trusts")

	// Part I - Background Information
	b.partHeader("Part I - Background Information")
	b.tinBoxes("TIN", company.TIN)
	b.textField("Taxpayer's Name", company.Name, 90)
	b.textField("Registered Address", company.Address, 90)
	b.textFieldHalf("RDO Code", company.RDOCode, "Date of Birth", getVal(data, "date_of_birth"))
	b.textFieldHalf("Taxable Year", data["period"], "Deduction Method", deductionMethod)
	b.textField("Civil Status", getVal(data, "civil_status"), 90)
	b.checkbox("Amended Return?", getVal(data, "amended") == "true")
	b.gap(4)

	// Part II - Total Tax Payable (Summary)
	b.partHeader("Part II - Total Tax Payable")
	b.fieldRow("30", "Income Tax Due", getVal(data, "income_tax_due"), false)
	b.fieldRow("31", "Less: Total Tax Credits/Payments", getVal(data, "total_tax_credits"), false)
	b.fieldRow("32", "Tax Payable / (Overpayment)", getVal(data, "tax_payable"), false)
	b.gap(2)
	b.fieldRowNoAmt("", "Add: Penalties")
	b.fieldRow("33", "Surcharge", getVal(data, "surcharge"), false)
	b.fieldRow("34", "Interest", getVal(data, "interest"), false)
	b.fieldRow("35", "Compromise Penalty", getVal(data, "compromise"), false)
	b.totalRow("36", "Total Penalties", getVal(data, "total_penalties"))
	b.totalRow("37", "TOTAL AMOUNT DUE / (Overpayment)", getVal(data, "total_amount_due"))

	// Signature
	b.gap(6)
	b.signatureBlock("")

	// ---- Page 2: Detailed Computation ----
	b.addPage()
	b.formHeader("1701",
		"Annual Income Tax Return",
		"(Page 2 - Detailed Computation)")

	// Part V - Computation of Income Tax
	b.partHeader("Part V - Computation of Income Tax")

	// Schedule of Gross Income
	b.labelRow("A. Gross Income from Business / Profession", true)
	b.fieldRow("38", "Gross Sales / Receipts / Revenue / Fees", getVal(data, "gross_sales_receipts"), false)
	b.fieldRow("39", "Less: Sales Returns, Allowances, and Discounts", getVal(data, "sales_returns"), false)
	b.fieldRow("40", "Net Sales / Receipts / Revenue / Fees", getVal(data, "net_sales"), false)
	b.fieldRow("41", "Less: Cost of Sales / Services", getVal(data, "cost_of_sales"), false)
	b.totalRow("42", "Gross Income from Operations (Line 40 - Line 41)", getVal(data, "gross_income_from_business"))
	b.fieldRow("43", "Add: Other Taxable Income Not Subjected to Final Tax", getVal(data, "other_taxable_income"), false)
	b.totalRow("44", "Total Gross Income (Line 42 + Line 43)", getVal(data, "total_gross_income"))
	b.gap(2)

	// Deductions
	b.labelRow("B. Deductions", true)
	if deductionMethod == "itemized" {
		b.fieldRow("45", "Itemized Deductions (per schedule)", getVal(data, "itemized_deductions"), false)
	} else {
		b.fieldRow("45", "Optional Standard Deduction (40% of Gross Sales)", getVal(data, "osd_amount"), false)
	}
	b.totalRow("46", "Total Deductions", getVal(data, "total_deductions"))
	b.gap(2)

	// Tax computation
	b.labelRow("C. Tax Computation (TRAIN Law Graduated Rates)", true)
	b.totalRow("47", "Net Taxable Income (Line 44 - Line 46)", getVal(data, "net_taxable_income"))
	b.totalRow("48", "Income Tax Due (per Graduated Tax Table)", getVal(data, "income_tax_due"))
	b.gap(2)

	// Tax Credits
	b.labelRow("D. Tax Credits / Payments", true)
	b.fieldRow("49", "Creditable Withholding Tax (per BIR 2307)", getVal(data, "creditable_withholding_tax"), false)
	b.fieldRow("50", "Tax Paid in Return Previously Filed", getVal(data, "tax_previously_filed"), false)
	b.fieldRow("51", "1st Quarter Tax Payment (1701Q)", getVal(data, "quarterly_payment_q1"), false)
	b.fieldRow("52", "2nd Quarter Tax Payment (1701Q)", getVal(data, "quarterly_payment_q2"), false)
	b.fieldRow("53", "3rd Quarter Tax Payment (1701Q)", getVal(data, "quarterly_payment_q3"), false)
	b.fieldRow("54", "Other Tax Credits", getVal(data, "other_credits"), false)
	b.totalRow("55", "Total Tax Credits (Sum of Lines 49 to 54)", getVal(data, "total_tax_credits"))

	// TRAIN Law reference
	b.gap(8)
	b.labelRow("TRAIN Law Graduated Tax Table (Effective Jan 2023)", true)
	brackets := []struct{ bracket, rate string }{
		{"PHP 0 - 250,000", "0%"},
		{"PHP 250,001 - 400,000", "15% of excess over 250,000"},
		{"PHP 400,001 - 800,000", "22,500 + 20% of excess over 400,000"},
		{"PHP 800,001 - 2,000,000", "102,500 + 25% of excess over 800,000"},
		{"PHP 2,000,001 - 8,000,000", "402,500 + 30% of excess over 2,000,000"},
		{"Over PHP 8,000,000", "2,202,500 + 35% of excess over 8,000,000"},
	}

	cw := b.contentW()
	for _, br := range brackets {
		b.gridRow([]GridCell{
			{Text: br.bracket, Width: cw * 0.4, Align: "L"},
			{Text: br.rate, Width: cw * 0.6, Align: "L"},
		})
	}

	return b.output(w)
}
