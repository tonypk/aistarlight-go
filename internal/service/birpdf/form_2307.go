package birpdf

import (
	"fmt"
	"io"
	"strconv"
)

// Generate2307 produces an official-style BIR 2307 PDF.
// Certificate of Creditable Tax Withheld at Source
func Generate2307(w io.Writer, data map[string]string, company CompanyData) error {
	b := newBuilder(SizeLegal, "2307", "(Rev. 2018)")
	b.addPage()

	// Form header
	b.formHeader("2307",
		"Certificate of Creditable Tax Withheld At Source",
		"")

	// Part I - Payee Information
	b.partHeader("Part I - Payee Information")
	b.tinBoxes("TIN", getVal(data, "payee_tin"))
	b.textField("Payee's Name", getVal(data, "payee_name"), 90)
	b.textField("Registered Address", getVal(data, "payee_address"), 90)
	b.gap(4)

	// Part II - Payor Information
	b.partHeader("Part II - Payor Information")
	b.tinBoxes("TIN", company.TIN)
	b.textField("Payor's Name", company.Name, 90)
	b.textField("Registered Address", company.Address, 90)
	b.gap(4)

	// Part III - Details of Income Payment and Tax Withheld
	b.partHeader("Part III - Details of Monthly Income Payments and Taxes Withheld")
	b.textFieldHalf("For the Period", getVal(data, "period"), "Quarter", getVal(data, "quarter"))
	b.gap(2)

	// Table header
	cw := b.contentW()
	seqW := cw * 0.06
	atcW := cw * 0.14
	incW := cw * 0.30
	rateW := cw * 0.15
	taxW := cw * 0.35

	b.gridHeaderRow([]GridCell{
		{Text: "Seq", Width: seqW, Align: "C", Bold: true},
		{Text: "ATC Code", Width: atcW, Align: "C", Bold: true},
		{Text: "Income Payment", Width: incW, Align: "C", Bold: true},
		{Text: "Tax Rate", Width: rateW, Align: "C", Bold: true},
		{Text: "Tax Withheld", Width: taxW, Align: "C", Bold: true},
	})

	// Line items
	totalItems := parseIntSafe(getVal(data, "total_items"))
	for i := 1; i <= totalItems; i++ {
		prefix := fmt.Sprintf("item_%d", i)
		b.checkPageBreak(20)
		b.gridRow([]GridCell{
			{Text: getVal(data, prefix+"_seq_no"), Width: seqW, Align: "C"},
			{Text: getVal(data, prefix+"_atc_code"), Width: atcW, Align: "C"},
			{Text: formatAmount(getVal(data, prefix+"_income_amount")), Width: incW, Align: "R"},
			{Text: formatPercent(getVal(data, prefix+"_tax_rate")), Width: rateW, Align: "C"},
			{Text: formatAmount(getVal(data, prefix+"_tax_withheld")), Width: taxW, Align: "R"},
		})
	}

	// Total row
	b.checkPageBreak(20)
	b.gridRow([]GridCell{
		{Text: "", Width: seqW, Bold: true, Fill: true, FillR: 235, FillG: 235, FillB: 235},
		{Text: "TOTAL", Width: atcW, Align: "C", Bold: true, Fill: true, FillR: 235, FillG: 235, FillB: 235},
		{Text: formatAmount(getVal(data, "total_income_amount")), Width: incW, Align: "R", Bold: true, Fill: true, FillR: 235, FillG: 235, FillB: 235},
		{Text: "", Width: rateW, Fill: true, FillR: 235, FillG: 235, FillB: 235},
		{Text: formatAmount(getVal(data, "total_tax_withheld")), Width: taxW, Align: "R", Bold: true, Fill: true, FillR: 235, FillG: 235, FillB: 235},
	})

	// Signature block
	b.gap(6)
	b.signatureBlock("I declare, under the penalties of perjury, that this certificate has been made in good faith, verified by me, and to the best of my knowledge and belief, is true and correct, pursuant to the provisions of the National Internal Revenue Code, as amended, and the regulations issued under authority thereof.")

	// Payment details
	b.checkPageBreak(100)
	b.paymentDetails()

	return b.output(w)
}

// formatPercent formats a decimal rate as a percentage string (e.g. "0.05" → "5%").
func formatPercent(value string) string {
	if value == "" || value == "0" {
		return "-"
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}
	if f == 0 {
		return "-"
	}
	// If value looks like a fraction (< 1), convert to percentage
	if f < 1 {
		return fmt.Sprintf("%.0f%%", f*100)
	}
	return fmt.Sprintf("%.0f%%", f)
}

// parseIntSafe parses a string to int, returning 0 on failure.
func parseIntSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
