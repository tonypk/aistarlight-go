package service

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// generateBIR2550MPDF produces a BIR 2550M Monthly VAT Declaration PDF.
func generateBIR2550MPDF(w io.Writer, data map[string]string, co CompanyInfo) error {
	b := newPDF()
	b.titleBanner("BIR Form No. 2550M", "Monthly Value-Added Tax Declaration")

	// Part I
	b.sectionHeader("Part I - Background Information")
	b.infoBlock([][2]string{
		{"TIN", co.TINNumber},
		{"RDO Code", co.RDOCode},
		{"Taxpayer's Name / Company", co.CompanyName},
		{"Taxable Period", data["period"]},
		{"Amendment?", "No"},
	})

	// Part II - Sales
	b.gap(8)
	b.sectionHeader("Part II - Sales / Receipts")
	b.lineItem("1", "Vatable Sales / Receipts", getD(data, "line_1_vatable_sales", "vatable_sales"), false)
	b.lineItem("2", "Sales to Government (subject to 5% Final Withholding VAT)", getD(data, "line_2_sales_to_government"), false)
	b.lineItem("3", "Zero-Rated Sales", getD(data, "line_3_zero_rated_sales", "zero_rated_sales"), false)
	b.lineItem("4", "Exempt Sales", getD(data, "line_4_exempt_sales", "vat_exempt_sales"), false)
	b.lineItem("5", "Total Sales / Receipts (Sum of Lines 1 to 4)", getD(data, "line_5_total_sales", "total_sales"), true)

	// Part III - Output Tax
	b.gap(8)
	b.sectionHeader("Part III - Output Tax")
	b.lineItem("6", "Output VAT (Line 1 x 12%)", getD(data, "line_6_output_vat", "output_vat"), false)
	b.lineItem("6A", "Output VAT on Sales to Government (Line 2 x 5%)", getD(data, "line_6a_output_vat_government"), false)
	b.lineItem("6B", "Total Output Tax (Line 6 + Line 6A)", getD(data, "line_6b_total_output_vat"), true)

	// Part IV - Input Tax
	b.gap(8)
	b.sectionHeader("Part IV - Allowable Input Tax")
	b.lineItem("7", "Input VAT on Domestic Purchases of Goods", getD(data, "line_7_input_vat_goods", "input_vat_goods"), false)
	b.lineItem("8", "Input VAT on Domestic Purchases of Capital Goods", getD(data, "line_8_input_vat_capital", "input_vat_capital"), false)
	b.lineItem("9", "Input VAT on Domestic Purchases of Services", getD(data, "line_9_input_vat_services", "input_vat_services"), false)
	b.lineItem("10", "Input VAT on Importation of Goods", getD(data, "line_10_input_vat_imports"), false)
	b.lineItem("11", "Total Input Tax (Sum of Lines 7 to 10)", getD(data, "line_11_total_input_vat", "total_input_vat"), true)

	// Part V - Tax Due
	b.gap(8)
	b.sectionHeader("Part V - Tax Due")
	b.lineItem("12", "VAT Payable (Line 6B - Line 11)", getD(data, "line_12_vat_payable", "vat_payable"), false)
	b.lineItem("13", "Less: Tax Credits / Payments", getD(data, "line_13_less_tax_credits"), false)
	b.lineItem("14", "Net VAT Payable (Line 12 - Line 13)", getD(data, "line_14_net_vat_payable", "net_vat_payable"), false)
	b.lineItem("15", "Add: Penalties (Surcharge, Interest, Compromise)", getD(data, "line_15_add_penalties"), false)

	b.totalDueHighlight("16", "TOTAL AMOUNT DUE (Line 14 + Line 15)",
		getD(data, "line_16_total_amount_due", "net_vat_payable"))

	b.taxCreditNote(getD(data, "tax_credit_carried_forward"))

	// Compliance score
	if scoreStr, ok := data["compliance_score"]; ok {
		if score, err := strconv.Atoi(scoreStr); err == nil {
			b.complianceScore(score)
		}
	}

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation for review. The official BIR 2550M form must be filed via eBIRForms or eFPS.",
		"2. Sales to Government (Line 2): Subject to 5% final withholding VAT per RR 14-2003.",
		"3. Capital Goods (Line 8): Input VAT on depreciable assets > PHP 1,000,000 amortized over useful life (max 60 months).",
		"4. Tax Credits (Line 13): Include creditable withholding VAT, prior period excess credits, and other tax payments.",
		"5. Required Attachments: Summary List of Sales (SLS), Summary List of Purchases (SLP), SAWT if applicable.",
		"6. Filing Deadline: 20th day following the end of each month. Late filing incurs 25% surcharge + 12% annual interest.",
	})

	return b.output(w)
}

// generateBIR2550QPDF produces a BIR 2550Q Quarterly VAT Return PDF.
func generateBIR2550QPDF(w io.Writer, data map[string]string, co CompanyInfo) error {
	b := newPDF()
	b.titleBanner("BIR Form No. 2550Q", "Quarterly Value-Added Tax Return")

	b.sectionHeader("Part I - Background Information")
	b.infoBlock([][2]string{
		{"TIN", co.TINNumber},
		{"RDO Code", co.RDOCode},
		{"Taxpayer's Name / Company", co.CompanyName},
		{"Taxable Quarter", data["period"]},
		{"Amendment?", "No"},
	})

	// Identical line items to 2550M with quarterly branding.
	b.gap(8)
	b.sectionHeader("Part II - Sales / Receipts (Quarterly)")
	b.lineItem("1", "Vatable Sales / Receipts", getD(data, "line_1_vatable_sales", "vatable_sales"), false)
	b.lineItem("2", "Sales to Government (subject to 5% Final Withholding VAT)", getD(data, "line_2_sales_to_government"), false)
	b.lineItem("3", "Zero-Rated Sales", getD(data, "line_3_zero_rated_sales", "zero_rated_sales"), false)
	b.lineItem("4", "Exempt Sales", getD(data, "line_4_exempt_sales", "vat_exempt_sales"), false)
	b.lineItem("5", "Total Sales / Receipts (Sum of Lines 1 to 4)", getD(data, "line_5_total_sales", "total_sales"), true)

	b.gap(8)
	b.sectionHeader("Part III - Output Tax")
	b.lineItem("6", "Output VAT (Line 1 x 12%)", getD(data, "line_6_output_vat", "output_vat"), false)
	b.lineItem("6A", "Output VAT on Sales to Government (Line 2 x 5%)", getD(data, "line_6a_output_vat_government"), false)
	b.lineItem("6B", "Total Output Tax (Line 6 + Line 6A)", getD(data, "line_6b_total_output_vat"), true)

	b.gap(8)
	b.sectionHeader("Part IV - Allowable Input Tax")
	b.lineItem("7", "Input VAT on Domestic Purchases of Goods", getD(data, "line_7_input_vat_goods", "input_vat_goods"), false)
	b.lineItem("8", "Input VAT on Domestic Purchases of Capital Goods", getD(data, "line_8_input_vat_capital", "input_vat_capital"), false)
	b.lineItem("9", "Input VAT on Domestic Purchases of Services", getD(data, "line_9_input_vat_services", "input_vat_services"), false)
	b.lineItem("10", "Input VAT on Importation of Goods", getD(data, "line_10_input_vat_imports"), false)
	b.lineItem("11", "Total Input Tax (Sum of Lines 7 to 10)", getD(data, "line_11_total_input_vat", "total_input_vat"), true)

	b.gap(8)
	b.sectionHeader("Part V - Tax Due")
	b.lineItem("12", "VAT Payable (Line 6B - Line 11)", getD(data, "line_12_vat_payable", "vat_payable"), false)
	b.lineItem("13", "Less: Tax Credits / Payments", getD(data, "line_13_less_tax_credits"), false)
	b.lineItem("14", "Net VAT Payable (Line 12 - Line 13)", getD(data, "line_14_net_vat_payable", "net_vat_payable"), false)
	b.lineItem("15", "Add: Penalties (Surcharge, Interest, Compromise)", getD(data, "line_15_add_penalties"), false)

	b.totalDueHighlight("16", "TOTAL AMOUNT DUE (Line 14 + Line 15)",
		getD(data, "line_16_total_amount_due", "net_vat_payable"))
	b.taxCreditNote(getD(data, "tax_credit_carried_forward"))

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation for review. The official BIR 2550Q form must be filed via eBIRForms or eFPS.",
		"2. Filing Deadline: 25th day following the close of each quarter.",
		"3. Required Attachments: Summary List of Sales (SLS), Summary List of Purchases (SLP), SAWT.",
		"4. Quarterly VAT Return consolidates monthly transactions for the taxable quarter.",
	})

	return b.output(w)
}

// generateBIR1601CPDF produces a BIR 1601-C Monthly Withholding Tax on Compensation PDF.
func generateBIR1601CPDF(w io.Writer, data map[string]string, co CompanyInfo) error {
	b := newPDF()
	b.titleBanner("BIR Form No. 1601-C", "Monthly Remittance Return of Income Taxes Withheld on Compensation")

	b.sectionHeader("Part I - Background Information")
	b.infoBlock([][2]string{
		{"TIN", co.TINNumber},
		{"RDO Code", co.RDOCode},
		{"Taxpayer's Name / Company", co.CompanyName},
		{"Taxable Month", data["period"]},
	})

	// Part II - Computation
	b.gap(8)
	b.sectionHeader("Part II - Computation of Tax")
	b.lineItem("1", "Total Amount of Compensation Paid", getD(data, "line_1_total_compensation"), false)
	b.lineItem("2", "Less: Statutory Minimum Wage / Holiday / OT / NSD", getD(data, "line_2_statutory_minimum_wage"), false)
	b.lineItem("3", "Less: Non-Taxable 13th Month & Benefits (up to PHP 90,000)", getD(data, "line_3_nontaxable_13th_month"), false)
	b.lineItem("4", "Less: De Minimis Benefits", getD(data, "line_4_nontaxable_deminimis"), false)
	b.lineItem("5", "Less: SSS/GSIS/PHIC/HDMF Mandatory Contributions", getD(data, "line_5_sss_gsis_phic_hdmf"), false)
	b.lineItem("6", "Less: Other Non-Taxable Compensation", getD(data, "line_6_other_nontaxable"), false)
	b.lineItem("7", "Total Non-Taxable Compensation (Sum of Lines 2-6)", getD(data, "line_7_total_nontaxable"), true)
	b.lineItem("8", "Taxable Compensation (Line 1 - Line 7)", getD(data, "line_8_taxable_compensation"), true)
	b.gap(4)
	b.lineItem("9", "Tax Required to be Withheld (per Withholding Tax Table)", getD(data, "line_9_tax_withheld"), false)
	b.lineItem("10", "Adjustment for Over/Under Withholding", getD(data, "line_10_adjustment"), false)
	b.lineItem("11", "Total Tax to be Remitted (Line 9 + Line 10)", getD(data, "line_11_total_tax_remitted"), true)

	// Part III - Penalties
	b.gap(8)
	b.sectionHeader("Part III - Penalties")
	b.lineItem("12", "Surcharge", getD(data, "line_12_surcharge"), false)
	b.lineItem("13", "Interest", getD(data, "line_13_interest"), false)
	b.lineItem("14", "Compromise Penalty", getD(data, "line_14_compromise"), false)
	b.lineItem("15", "Total Penalties (Lines 12 to 14)", getD(data, "line_15_total_penalties"), true)

	b.totalDueHighlight("16", "TOTAL AMOUNT DUE (Line 11 + Line 15)",
		getD(data, "line_16_total_amount_due"))

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation. The official BIR 1601-C must be filed via eBIRForms or eFPS.",
		"2. Filing Deadline: 10th day of the month following the month of withholding.",
		"3. Withholding tax on compensation is based on the BIR Withholding Tax Table (RR 11-2018).",
		"4. Minimum wage earners are exempt from income tax. Their compensation should be excluded from Line 8.",
		"5. Annual reconciliation is done via BIR Form 1604-CF (due January 31).",
	})

	return b.output(w)
}

// generateBIR0619EPDF produces a BIR 0619-E Monthly EWT Remittance PDF.
func generateBIR0619EPDF(w io.Writer, data map[string]string, co CompanyInfo) error {
	b := newPDF()
	b.titleBanner("BIR Form No. 0619-E", "Monthly Remittance Form for Creditable Income Taxes Withheld (Expanded)")

	b.sectionHeader("Part I - Background Information")
	b.infoBlock([][2]string{
		{"TIN", co.TINNumber},
		{"RDO Code", co.RDOCode},
		{"Taxpayer's Name / Company", co.CompanyName},
		{"For the Month", data["period"]},
	})

	// Part II - Computation
	b.gap(8)
	b.sectionHeader("Part II - Computation of Tax")
	b.lineItem("1", "Total Amount of Income Payments", getD(data, "line_1_total_amount_of_income_payments"), false)
	b.lineItem("2", "Total Taxes Withheld for the Month", getD(data, "line_2_total_taxes_withheld"), false)
	b.lineItem("3", "Adjustment for Over-Remittance from Previous Month(s)", getD(data, "line_3_adjustment"), false)
	b.lineItem("4", "Tax Still Due (Line 2 - Line 3)", getD(data, "line_4_tax_still_due"), true)

	// Part III - Penalties
	b.gap(8)
	b.sectionHeader("Part III - Penalties")
	b.lineItem("5", "Surcharge", getD(data, "line_5_surcharge"), false)
	b.lineItem("6", "Interest", getD(data, "line_6_interest"), false)
	b.lineItem("7", "Compromise Penalty", getD(data, "line_7_compromise"), false)
	b.lineItem("8", "Total Penalties (Lines 5 to 7)", getD(data, "line_8_total_penalties"), true)

	b.totalDueHighlight("9", "TOTAL AMOUNT DUE (Line 4 + Line 8)",
		getD(data, "line_9_total_amount_due"))

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation. The official BIR 0619-E must be filed via eBIRForms or eFPS.",
		"2. Filing Deadline: 10th day of the month following the month of withholding.",
		"3. This form is for the first 2 months of each quarter only. For the 3rd month, use BIR 1601-EQ.",
		"4. EWT rates are per RR 2-98 as amended by RR 11-2018 (TRAIN Law).",
		"5. Each payee should receive a BIR Form 2307 certificate by the 20th of the month following the quarter.",
	})

	return b.output(w)
}

// generateBIR1701PDF produces a BIR 1701 Annual Individual Income Tax Return PDF.
func generateBIR1701PDF(w io.Writer, data map[string]string, co CompanyInfo) error {
	b := newPDF()
	b.titleBanner("BIR Form No. 1701", "Annual Income Tax Return (For Self-Employed Individuals)")

	deductionMethod := getD(data, "deduction_method")
	if deductionMethod == "0" {
		deductionMethod = "OSD"
	}

	b.sectionHeader("Part I - Background Information")
	b.infoBlock([][2]string{
		{"TIN", co.TINNumber},
		{"RDO Code", co.RDOCode},
		{"Taxpayer's Name", co.CompanyName},
		{"Taxable Year", data["period"]},
		{"Deduction Method", deductionMethod},
	})

	// Part II - Gross Income
	b.gap(8)
	b.sectionHeader("Part II - Gross Income from Business / Profession")
	b.lineItem("1", "Gross Sales / Receipts", getD(data, "gross_sales_receipts"), false)
	b.lineItem("2", "Less: Cost of Sales / Services", getD(data, "cost_of_sales"), false)
	b.lineItem("3", "Gross Income from Business (Line 1 - Line 2)", getD(data, "gross_income_from_business"), true)
	b.lineItem("4", "Other Taxable Income", getD(data, "other_taxable_income"), false)
	b.lineItem("5", "Total Gross Income (Line 3 + Line 4)", getD(data, "total_gross_income"), true)

	// Part III - Deductions
	b.gap(8)
	b.sectionHeader("Part III - Deductions")
	if deductionMethod == "itemized" {
		b.lineItem("6", "Itemized Deductions", getD(data, "itemized_deductions"), false)
	} else {
		b.lineItem("6", "Optional Standard Deduction (40% of Gross Sales)", getD(data, "osd_amount"), false)
	}
	b.lineItem("7", "Total Deductions", getD(data, "total_deductions"), true)

	// Part IV - Tax Computation
	b.gap(8)
	b.sectionHeader("Part IV - Tax Computation (TRAIN Law Graduated Rates)")
	b.lineItem("8", "Net Taxable Income (Line 5 - Line 7)", getD(data, "net_taxable_income"), true)
	b.lineItem("9", "Income Tax Due (per Graduated Tax Table)", getD(data, "income_tax_due"), true)

	// Part V - Tax Credits
	b.gap(8)
	b.sectionHeader("Part V - Tax Credits / Payments")
	b.lineItem("10", "Creditable Withholding Tax (per BIR 2307)", getD(data, "creditable_withholding_tax"), false)
	b.lineItem("11", "Quarterly Income Tax Payments", getD(data, "quarterly_payments"), false)
	b.lineItem("12", "Other Tax Credits", getD(data, "other_credits"), false)
	b.lineItem("13", "Total Tax Credits (Lines 10 to 12)", getD(data, "total_tax_credits"), true)

	// Part VI - Tax Due
	b.gap(8)
	b.sectionHeader("Part VI - Tax Due")
	b.lineItem("14", "Tax Payable (Line 9 - Line 13)", getD(data, "tax_payable"), false)
	b.lineItem("15A", "Surcharge", getD(data, "surcharge"), false)
	b.lineItem("15B", "Interest", getD(data, "interest"), false)
	b.lineItem("15C", "Compromise Penalty", getD(data, "compromise"), false)
	b.lineItem("16", "Total Penalties", getD(data, "total_penalties"), true)

	b.totalDueHighlight("17", "TOTAL AMOUNT DUE (Line 14 + Line 16)",
		getD(data, "total_amount_due"))

	// TRAIN Law Tax Table Reference
	b.gap(8)
	b.sectionHeader("TRAIN Law Graduated Tax Table (Effective Jan 2023)")
	b.pdf.SetFont("Helvetica", "", 7)
	b.pdf.SetTextColor(0, 0, 0)
	brackets := []string{
		"0 - 250,000: 0%",
		"250,001 - 400,000: 15% of excess over 250,000",
		"400,001 - 800,000: 22,500 + 20% of excess over 400,000",
		"800,001 - 2,000,000: 102,500 + 25% of excess over 800,000",
		"2,000,001 - 8,000,000: 402,500 + 30% of excess over 2,000,000",
		"Over 8,000,000: 2,202,500 + 35% of excess over 8,000,000",
	}
	for _, br := range brackets {
		b.pdf.SetXY(pdfMarginL+16, b.y)
		b.pdf.CellFormat(pdfContentW-20, 10, br, "", 0, "L", false, 0, "")
		b.y += 10
	}

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation. The official BIR 1701 must be filed via eBIRForms or eFPS.",
		"2. Filing Deadline: April 15 of the following year.",
		"3. OSD: 40% of gross sales/receipts (no need for supporting documents).",
		"4. Itemized: Requires full documentation of business expenses.",
		"5. Quarterly payments (BIR 1701Q) due Apr 15, Aug 15, Nov 15.",
	})

	return b.output(w)
}

// generateBIR1702PDF produces a BIR 1702 Annual Corporate Income Tax Return PDF.
func generateBIR1702PDF(w io.Writer, data map[string]string, co CompanyInfo) error {
	b := newPDF()
	b.titleBanner("BIR Form No. 1702", "Annual Income Tax Return (For Corporations)")

	rcitRate := getD(data, "rcit_rate")
	rateLabel := "25% (Standard)"
	if rcitRate == "0.20" || rcitRate == "0.2" {
		rateLabel = "20% (SME)"
	}

	deductionMethod := getD(data, "deduction_method")
	if deductionMethod == "0" {
		deductionMethod = "itemized"
	}

	b.sectionHeader("Part I - Background Information")
	b.infoBlock([][2]string{
		{"TIN", co.TINNumber},
		{"RDO Code", co.RDOCode},
		{"Corporation Name", co.CompanyName},
		{"Taxable Year", data["period"]},
		{"RCIT Rate", rateLabel},
		{"Deduction Method", deductionMethod},
	})

	// Part II - Gross Income
	b.gap(8)
	b.sectionHeader("Part II - Gross Income")
	b.lineItem("1", "Gross Income / Revenue", getD(data, "gross_income"), false)
	b.lineItem("2", "Less: Cost of Sales / Services", getD(data, "cost_of_sales"), false)
	b.lineItem("3", "Gross Profit (Line 1 - Line 2)", getD(data, "gross_profit"), true)
	b.lineItem("4", "Other Income", getD(data, "other_income"), false)
	b.lineItem("5", "Total Gross Income (Line 3 + Line 4)", getD(data, "total_gross_income"), true)

	// Part III - Deductions
	b.gap(8)
	b.sectionHeader("Part III - Deductions")
	if deductionMethod == "osd" {
		b.lineItem("6", "Optional Standard Deduction (40% of Gross Income)", getD(data, "osd_amount"), false)
	} else {
		b.lineItem("6", "Itemized Deductions", getD(data, "itemized_deductions"), false)
	}
	b.lineItem("7", "Total Deductions", getD(data, "total_deductions"), true)

	// Part IV - Tax Computation (RCIT vs MCIT)
	b.gap(8)
	b.sectionHeader("Part IV - Tax Computation (RCIT vs MCIT)")
	b.lineItem("8", "Net Taxable Income (Line 5 - Line 7)", getD(data, "net_taxable_income"), true)
	b.lineItem("9A", fmt.Sprintf("RCIT (%s)", rateLabel), getD(data, "rcit_amount"), false)
	b.lineItem("9B", "MCIT (1% of Gross Income)", getD(data, "mcit_amount"), false)
	b.lineItem("10", "Income Tax Due (Higher of RCIT or MCIT)", getD(data, "income_tax_due"), true)

	// Part V - Tax Credits
	b.gap(8)
	b.sectionHeader("Part V - Tax Credits / Payments")
	b.lineItem("11", "Excess MCIT from Prior Year(s)", getD(data, "excess_mcit_prior"), false)
	b.lineItem("12", "Creditable Withholding Tax (per BIR 2307)", getD(data, "creditable_withholding_tax"), false)
	b.lineItem("13", "Quarterly Income Tax Payments", getD(data, "quarterly_payments"), false)
	b.lineItem("14", "Other Tax Credits", getD(data, "other_credits"), false)
	b.lineItem("15", "Total Tax Credits (Lines 11 to 14)", getD(data, "total_tax_credits"), true)

	// Part VI - Tax Due
	b.gap(8)
	b.sectionHeader("Part VI - Tax Due")
	b.lineItem("16", "Tax Payable (Line 10 - Line 15)", getD(data, "tax_payable"), false)
	b.lineItem("17A", "Surcharge", getD(data, "surcharge"), false)
	b.lineItem("17B", "Interest", getD(data, "interest"), false)
	b.lineItem("17C", "Compromise Penalty", getD(data, "compromise"), false)
	b.lineItem("18", "Total Penalties", getD(data, "total_penalties"), true)

	b.totalDueHighlight("19", "TOTAL AMOUNT DUE (Line 16 + Line 18)",
		getD(data, "total_amount_due"))

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation. The official BIR 1702 must be filed via eBIRForms or eFPS.",
		"2. Filing Deadline: April 15 of the following year (calendar year filers).",
		"3. RCIT: 25% standard rate; 20% for SMEs (net taxable income <= PHP 5M and total assets <= PHP 100M).",
		"4. MCIT: 1% of gross income (reduced from 2% under CREATE Law RA 11534).",
		"5. Tax due = higher of RCIT or MCIT. Excess MCIT can be carried forward for 3 years.",
		"6. OSD for corporations: 40% of gross income (not gross sales).",
	})

	return b.output(w)
}

// generateBIR2316PDF produces a BIR 2316 Certificate of Compensation PDF.
func generateBIR2316PDF(w io.Writer, data map[string]string, co CompanyInfo) error {
	b := newPDF()
	b.titleBanner("BIR Form No. 2316", "Certificate of Compensation Payment / Tax Withheld")

	// Part I - Employee & Employer Info
	b.sectionHeader("Part I - Employee & Employer Information")
	employerName := data["employer_name"]
	if employerName == "" {
		employerName = co.CompanyName
	}
	employerTIN := data["employer_tin"]
	if employerTIN == "" {
		employerTIN = co.TINNumber
	}
	b.infoBlock([][2]string{
		{"Employee Name", data["employee_name"]},
		{"Employee TIN", data["employee_tin"]},
		{"Employer / Company", employerName},
		{"Employer TIN", employerTIN},
		{"Taxable Year", data["period"]},
	})

	// Part II - Present Employer
	b.gap(8)
	b.sectionHeader("Part II - Compensation from Present Employer")
	b.lineItem("1", "Gross Compensation", getD(data, "present_employer_compensation"), false)
	b.lineItem("2", "Less: Non-Taxable Compensation", getD(data, "present_employer_nontaxable"), false)
	b.lineItem("3", "Taxable Compensation (Line 1 - Line 2)", getD(data, "present_employer_taxable"), true)

	// Part III - Previous Employer
	b.gap(8)
	b.sectionHeader("Part III - Compensation from Previous Employer(s)")
	b.lineItem("4", "Gross Compensation (Previous)", getD(data, "previous_employer_compensation"), false)
	b.lineItem("5", "Less: Non-Taxable (Previous)", getD(data, "previous_employer_nontaxable"), false)
	b.lineItem("6", "Taxable Compensation (Previous)", getD(data, "previous_employer_taxable"), true)

	// Part IV - Totals
	b.gap(8)
	b.sectionHeader("Part IV - Total Compensation Summary")
	b.lineItem("7", "Total Compensation (Line 1 + Line 4)", getD(data, "total_compensation"), false)
	b.lineItem("8", "Total Non-Taxable Compensation (Line 2 + Line 5)", getD(data, "total_nontaxable_compensation"), false)
	b.lineItem("9", "Total Taxable Compensation (Line 3 + Line 6)", getD(data, "total_taxable_compensation"), true)

	// Part V - Tax Computation
	b.gap(8)
	b.sectionHeader("Part V - Tax Computation & Year-End Adjustment")
	b.lineItem("10", "Tax Due (per Graduated Tax Table)", getD(data, "tax_due"), true)
	b.lineItem("11", "Tax Withheld (Present Employer)", getD(data, "tax_withheld_present"), false)
	b.lineItem("12", "Tax Withheld (Previous Employer)", getD(data, "tax_withheld_previous"), false)
	b.lineItem("13", "Total Tax Withheld (Line 11 + Line 12)", getD(data, "total_tax_withheld"), true)

	// Year-end adjustment
	b.refundHighlight("14", getD(data, "amount_refunded"), getD(data, "amount_still_due"))

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft certificate. The official BIR 2316 must be issued by the employer.",
		"2. Deadline: January 31 of the following year (employer must provide to employee).",
		"3. Non-taxable compensation includes: statutory minimum wage, 13th month (up to PHP 90,000),",
		"   de minimis benefits, mandatory contributions (SSS/GSIS, PhilHealth, Pag-IBIG).",
		"4. Employee must attach this certificate when filing BIR 1700 (for purely compensation income).",
		"5. Year-end adjustment ensures total tax withheld matches the annual tax due.",
	})

	return b.output(w)
}

// generateGenericPDF produces a generic PDF for unsupported form types.
func generateGenericPDF(w io.Writer, reportType string, data map[string]string, co CompanyInfo) error {
	b := newPDF()

	formName := reportType
	if idx := len("BIR_"); len(reportType) > idx && reportType[:4] == "BIR_" {
		formName = reportType[4:]
	}

	b.titleBanner("BIR Form "+formName, reportType)

	b.sectionHeader("Company Information")
	b.infoBlock([][2]string{
		{"Company", co.CompanyName},
		{"TIN", co.TINNumber},
	})

	b.gap(8)
	b.sectionHeader("Report Data")
	for key, value := range data {
		b.lineItem("", key, value, false)
		if b.y > pdfPageH-80 {
			b.pdf.AddPage()
			b.y = 40
		}
	}

	return b.output(w)
}

// ReconciliationPDFInput holds all data needed for a reconciliation report PDF.
type ReconciliationPDFInput struct {
	SessionID  string
	Period     string
	Status     string
	FileCount  int
	Company    CompanyInfo
	VATSummary map[string]string
	MatchStats map[string]interface{}
	Anomalies  []AnomalyEntry
}

// AnomalyEntry represents a single anomaly for the reconciliation PDF.
type AnomalyEntry struct {
	Severity    string
	Description string
	Status      string
}

// GenerateReconciliationPDF produces a VAT reconciliation report PDF.
func GenerateReconciliationPDF(w io.Writer, input ReconciliationPDFInput) error {
	b := newPDF()

	// Title with period
	b.pdf.SetFillColor(30, 58, 95)
	b.pdf.Rect(0, 0, pdfPageW, 70, "F")
	b.pdf.SetTextColor(255, 255, 255)
	b.pdf.SetFont("Helvetica", "B", 18)
	b.centeredCell(16, 20, "VAT Reconciliation Report")
	b.pdf.SetFont("Helvetica", "", 10)
	b.centeredCell(36, 14, "Period: "+input.Period)
	b.pdf.SetFont("Helvetica", "", 7)
	b.centeredCell(52, 12,
		fmt.Sprintf("Company: %s | TIN: %s", input.Company.CompanyName, input.Company.TINNumber))
	b.y = 90

	// Session Info
	b.sectionHeader("Session Information")
	b.pdf.SetFont("Helvetica", "", 8)
	b.pdf.SetTextColor(0, 0, 0)
	items := [][2]string{
		{"Session ID", input.SessionID},
		{"Status", input.Status},
		{"Files", fmt.Sprintf("%d", input.FileCount)},
	}
	for _, item := range items {
		b.pdf.SetXY(pdfMarginL+6, b.y)
		b.pdf.CellFormat(100, 14, item[0]+":", "", 0, "L", false, 0, "")
		b.pdf.SetXY(pdfMarginL+110, b.y)
		b.pdf.CellFormat(200, 14, item[1], "", 0, "L", false, 0, "")
		b.y += 14
	}

	// VAT Summary
	b.gap(8)
	b.sectionHeader("VAT Summary")
	vatItems := [][2]string{
		{"Vatable Sales", "vatable_sales"},
		{"Sales to Government", "sales_to_government"},
		{"Zero-Rated Sales", "zero_rated_sales"},
		{"Exempt Sales", "vat_exempt_sales"},
		{"Total Sales", "total_sales"},
		{"Output VAT", "output_vat"},
		{"Output VAT (Government)", "output_vat_government"},
		{"Total Output VAT", "total_output_vat"},
		{"Input VAT (Goods)", "input_vat_goods"},
		{"Input VAT (Capital)", "input_vat_capital"},
		{"Input VAT (Services)", "input_vat_services"},
		{"Input VAT (Imports)", "input_vat_imports"},
		{"Total Input VAT", "total_input_vat"},
		{"Net VAT", "net_vat"},
	}
	for _, item := range vatItems {
		isTotal := len(item[0]) >= 5 && item[0][:5] == "Total" || item[0] == "Net VAT"
		b.lineItem("", item[0], getD(input.VATSummary, item[1]), isTotal)
	}

	// Match Statistics
	if len(input.MatchStats) > 0 {
		b.gap(12)
		b.sectionHeader("Transaction Matching")
		statItems := [][2]string{
			{"Matched Pairs", fmt.Sprintf("%v", input.MatchStats["matched_pairs"])},
			{"Unmatched Records", fmt.Sprintf("%v", input.MatchStats["unmatched_records"])},
			{"Unmatched Bank Entries", fmt.Sprintf("%v", input.MatchStats["unmatched_bank"])},
		}
		if rate, ok := input.MatchStats["match_rate"]; ok {
			if f, err := strconv.ParseFloat(fmt.Sprintf("%v", rate), 64); err == nil {
				statItems = append(statItems, [2]string{"Match Rate", fmt.Sprintf("%.1f%%", f*100)})
			}
		}
		b.pdf.SetTextColor(0, 0, 0)
		for _, item := range statItems {
			b.pdf.SetFont("Helvetica", "", 8)
			b.pdf.SetXY(pdfMarginL+6, b.y)
			b.pdf.CellFormat(180, 14, item[0]+":", "", 0, "L", false, 0, "")
			b.pdf.SetFont("Helvetica", "B", 8)
			b.pdf.SetXY(pdfMarginL+196, b.y)
			b.pdf.CellFormat(100, 14, item[1], "", 0, "L", false, 0, "")
			b.y += 14
		}
	}

	// Anomalies
	if len(input.Anomalies) > 0 {
		b.gap(12)
		b.sectionHeader(fmt.Sprintf("Anomalies (%d found)", len(input.Anomalies)))

		limit := len(input.Anomalies)
		if limit > 20 {
			limit = 20
		}
		for _, a := range input.Anomalies[:limit] {
			if b.y > pdfPageH-80 {
				b.pdf.AddPage()
				b.y = 40
			}

			switch a.Severity {
			case "high":
				b.pdf.SetTextColor(239, 68, 68)
			case "medium":
				b.pdf.SetTextColor(245, 158, 11)
			default:
				b.pdf.SetTextColor(107, 114, 128)
			}
			b.pdf.SetFont("Helvetica", "B", 7)
			b.pdf.SetXY(pdfMarginL+6, b.y)
			tag := "[" + strings.ToUpper(a.Severity) + "]"
			b.pdf.CellFormat(40, 12, tag, "", 0, "L", false, 0, "")

			b.pdf.SetTextColor(0, 0, 0)
			b.pdf.SetFont("Helvetica", "", 7)
			desc := a.Description
			if len(desc) > 100 {
				desc = desc[:100]
			}
			b.pdf.SetXY(pdfMarginL+50, b.y)
			b.pdf.CellFormat(pdfContentW-120, 12, desc, "", 0, "L", false, 0, "")

			b.pdf.SetFont("Helvetica", "", 6)
			b.pdf.SetXY(pdfPageW-pdfMarginR-60, b.y)
			b.pdf.CellFormat(60, 12, a.Status, "", 0, "R", false, 0, "")
			b.y += 12
		}
	}

	return b.output(w)
}
