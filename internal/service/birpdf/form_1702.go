package birpdf

import (
	"fmt"
	"io"
)

// Generate1702 produces an official-style BIR 1702-RT PDF.
// Annual Income Tax Return for Corporations
func Generate1702(w io.Writer, data map[string]string, company CompanyData) error {
	b := newBuilder(SizeFolio, "1702-RT", "(Rev. 2023)")
	b.addPage()

	rcitRate := getVal(data, "rcit_rate")
	rateLabel := "25% (Standard)"
	if rcitRate == "0.20" || rcitRate == "0.2" {
		rateLabel = "20% (SME Rate)"
	}

	deductionMethod := getVal(data, "deduction_method")
	if deductionMethod == "0" || deductionMethod == "" {
		deductionMethod = "itemized"
	}

	// Form header
	b.formHeader("1702-RT",
		"Annual Income Tax Return",
		"For Corporations, Partnerships and Other Juridical Persons")

	// Part I - Background Information
	b.partHeader("Part I - Background Information")
	b.tinBoxes("TIN", company.TIN)
	b.textField("Corporation Name", company.Name, 90)
	b.textField("Registered Address", company.Address, 90)
	b.textFieldHalf("RDO Code", company.RDOCode, "Date of Incorporation", getVal(data, "date_of_incorporation"))
	b.textFieldHalf("Taxable Year", data["period"], "Fiscal Year End", getVal(data, "fiscal_year_end"))
	b.textFieldHalf("RCIT Rate Applied", rateLabel, "Deduction Method", deductionMethod)
	b.checkbox("Amended Return?", getVal(data, "amended") == "true")
	b.gap(4)

	// Part II - Total Tax Due
	b.partHeader("Part II - Total Tax Due")
	b.fieldRow("27", "Income Tax Due (higher of RCIT or MCIT)", getVal(data, "income_tax_due"), false)
	b.fieldRow("28", "Less: Total Tax Credits / Payments", getVal(data, "total_tax_credits"), false)
	b.fieldRow("29", "Tax Payable / (Overpayment)", getVal(data, "tax_payable"), false)
	b.gap(2)

	b.fieldRowNoAmt("", "Add: Penalties")
	b.fieldRow("30", "Surcharge", getVal(data, "surcharge"), false)
	b.fieldRow("31", "Interest", getVal(data, "interest"), false)
	b.fieldRow("32", "Compromise Penalty", getVal(data, "compromise"), false)
	b.totalRow("33", "Total Penalties", getVal(data, "total_penalties"))
	b.totalRow("34", "TOTAL AMOUNT DUE / (Overpayment)", getVal(data, "total_amount_due"))

	// Signature
	b.gap(6)
	b.signatureBlock("")

	// ---- Page 2: Detailed Computation ----
	b.addPage()
	b.formHeader("1702-RT",
		"Annual Income Tax Return",
		"(Page 2 - Detailed Computation)")

	// Part IV - Detailed Computation
	b.partHeader("Part IV - Computation of Tax")

	// Gross Income
	b.labelRow("A. Gross Income", true)
	b.fieldRow("35", "Gross Income / Revenue", getVal(data, "gross_income"), false)
	b.fieldRow("36", "Less: Sales Returns, Allowances & Discounts", getVal(data, "sales_returns"), false)
	b.fieldRow("37", "Net Revenue (Line 35 - Line 36)", getVal(data, "net_revenue"), false)
	b.fieldRow("38", "Less: Cost of Sales / Services", getVal(data, "cost_of_sales"), false)
	b.totalRow("39", "Gross Profit (Line 37 - Line 38)", getVal(data, "gross_profit"))
	b.fieldRow("40", "Add: Other Income", getVal(data, "other_income"), false)
	b.totalRow("41", "Total Gross Income (Line 39 + Line 40)", getVal(data, "total_gross_income"))
	b.gap(2)

	// Deductions
	b.labelRow("B. Deductions", true)
	if deductionMethod == "osd" {
		b.fieldRow("42", "Optional Standard Deduction (40% of Gross Income)", getVal(data, "osd_amount"), false)
	} else {
		b.fieldRow("42", "Itemized Deductions (per attached schedule)", getVal(data, "itemized_deductions"), false)
	}
	b.totalRow("43", "Total Deductions", getVal(data, "total_deductions"))
	b.gap(2)

	// Tax Computation (RCIT vs MCIT)
	b.labelRow("C. Tax Computation - RCIT vs MCIT", true)
	b.totalRow("44", "Net Taxable Income (Line 41 - Line 43)", getVal(data, "net_taxable_income"))
	b.fieldRow("45", fmt.Sprintf("RCIT (%s of Net Taxable Income)", rateLabel), getVal(data, "rcit_amount"), false)
	b.fieldRow("46", "MCIT (1% of Gross Income - Line 41)", getVal(data, "mcit_amount"), false)
	b.totalRow("47", "Income Tax Due (Higher of RCIT or MCIT)", getVal(data, "income_tax_due"))
	b.gap(2)

	// Tax Credits
	b.labelRow("D. Tax Credits / Payments", true)
	b.fieldRow("48", "Excess MCIT from Prior Year(s)", getVal(data, "excess_mcit_prior"), false)
	b.fieldRow("49", "Creditable Withholding Tax (per BIR 2307)", getVal(data, "creditable_withholding_tax"), false)
	b.fieldRow("50", "1st Quarter Tax Payment (1702Q)", getVal(data, "quarterly_payment_q1", "quarterly_payments"), false)
	b.fieldRow("51", "2nd Quarter Tax Payment (1702Q)", getVal(data, "quarterly_payment_q2"), false)
	b.fieldRow("52", "3rd Quarter Tax Payment (1702Q)", getVal(data, "quarterly_payment_q3"), false)
	b.fieldRow("53", "Other Tax Credits", getVal(data, "other_credits"), false)
	b.totalRow("54", "Total Tax Credits (Sum of Lines 48 to 53)", getVal(data, "total_tax_credits"))

	// CREATE Law note
	b.gap(6)
	b.labelRow("Notes on CREATE Law (RA 11534)", true)
	notes := []string{
		"RCIT: 25% standard rate; 20% for SMEs (net taxable income <= PHP 5M, total assets <= PHP 100M excl. land)",
		"MCIT: 1% of gross income (reduced from 2% under CREATE Law, effective Jul 1, 2020 to Jun 30, 2023)",
		"Income Tax Due = higher of RCIT or MCIT. Excess MCIT over RCIT can be carried forward for 3 years.",
		"OSD for corporations: 40% of gross income (not gross sales/receipts).",
	}
	for _, note := range notes {
		b.pdf.SetFont(birFontFamily, "", birTinySize)
		b.pdf.SetXY(b.marginL+4, b.y)
		b.pdf.CellFormat(b.contentW()-8, 9, note, "", 0, "L", false, 0, "")
		b.y += 9
	}

	return b.output(w)
}
