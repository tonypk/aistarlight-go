package iraspdf

import "io"

// GenerateFormB produces an IRAS Form B Individual Income Tax PDF.
func GenerateFormB(w io.Writer, data map[string]string, co CompanyData) error {
	b := newBuilder()
	b.titleBanner("Form B", "Individual Income Tax Return")

	b.sectionHeader("Taxpayer Information")
	b.infoBlock([][2]string{
		{"Name", co.Name},
		{"UEN / NRIC", co.UEN},
		{"Year of Assessment", getVal(data, "period")},
	})

	// Income
	b.gap(8)
	b.sectionHeader("Part A — Income")
	b.lineItem("1", "Employment Income", getVal(data, "employment_income"), false)
	b.lineItem("2", "Trade / Business Income", getVal(data, "trade_income"), false)
	b.lineItem("3", "Rental Income", getVal(data, "rental_income"), false)
	b.lineItem("4", "Other Income", getVal(data, "other_income"), false)
	b.lineItem("5", "Total Income (Lines 1 to 4)", getVal(data, "total_income"), true)

	// Deductions
	b.gap(8)
	b.sectionHeader("Part B — Deductions & Reliefs")
	b.lineItem("6", "Total Personal Reliefs", getVal(data, "total_reliefs"), false)
	b.lineItem("7", "Donation Deduction (250% of qualifying donations)", getVal(data, "donation_deduction"), false)

	// Tax Computation
	b.gap(8)
	b.sectionHeader("Part C — Tax Computation")
	b.lineItem("8", "Chargeable Income (Line 5 - Lines 6, 7)", getVal(data, "chargeable_income"), true)
	b.totalHighlight("9", "Tax Payable (progressive rates 0% - 24%)", getVal(data, "tax_payable"))

	// Tax brackets reference
	b.gap(8)
	b.sectionHeader("Singapore Resident Tax Brackets (YA 2024)")
	b.pdf.SetFont(fontFamily, "", smallSize)
	b.pdf.SetTextColor(0, 0, 0)
	brackets := []string{
		"First S$20,000: 0%",
		"S$20,001 - S$30,000: 2%",
		"S$30,001 - S$40,000: 3.5%",
		"S$40,001 - S$80,000: 7%",
		"S$80,001 - S$120,000: 11.5%",
		"S$120,001 - S$160,000: 15%",
		"S$160,001 - S$200,000: 18%",
		"S$200,001 - S$240,000: 19%",
		"S$240,001 - S$280,000: 19.5%",
		"S$280,001 - S$320,000: 20%",
		"S$320,001 - S$500,000: 22%",
		"S$500,001 - S$1,000,000: 23%",
		"Above S$1,000,000: 24%",
	}
	for _, br := range brackets {
		b.pdf.SetXY(marginL+16, b.y)
		b.pdf.CellFormat(contentW-20, 10, br, "", 0, "L", false, 0, "")
		b.y += 10
	}

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation. The official Form B must be filed via myTax Portal.",
		"2. Filing Deadline: 15 April (paper) / 18 April (e-Filing) of the year following the income year.",
		"3. Form B is for individuals with income other than employment (e.g. self-employed, rental).",
		"4. Employees with only employment income use Form B1 (pre-filled by IRAS).",
		"5. Qualifying donations receive 250% tax deduction.",
	})

	return b.output(w)
}
