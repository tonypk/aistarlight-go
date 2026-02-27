package iraspdf

import "io"

// GenerateS45 produces an IRAS S45 Withholding Tax PDF.
func GenerateS45(w io.Writer, data map[string]string, co CompanyData) error {
	b := newBuilder()
	b.titleBanner("Section 45", "Withholding Tax on Payments to Non-Residents")

	b.sectionHeader("Payer Information")
	b.infoBlock([][2]string{
		{"Company Name", co.Name},
		{"UEN", co.UEN},
		{"Payment Period", getVal(data, "period")},
	})

	// Payment Details
	b.gap(8)
	b.sectionHeader("Payment Details")
	b.lineItem("", "Income Type", "", false)
	b.pdf.SetFont(fontFamily, "B", fontSize)
	b.pdf.SetXY(marginL+44, b.y-16)
	b.pdf.CellFormat(contentW-164, 12, getVal(data, "income_type")+" — "+getVal(data, "description"), "", 0, "L", false, 0, "")

	b.lineItem("1", "Gross Payment Amount", getVal(data, "payment_amount"), false)
	b.lineItem("2", "WHT Rate", getVal(data, "wht_rate"), false)
	b.lineItem("3", "Tax Withheld (Line 1 x Line 2)", getVal(data, "tax_withheld"), true)
	b.totalHighlight("4", "Net Payment to Non-Resident (Line 1 - Line 3)", getVal(data, "net_payment"))

	// WHT Rate Reference
	b.gap(8)
	b.sectionHeader("S45 Withholding Tax Rates (Non-Residents)")
	b.pdf.SetFont(fontFamily, "", smallSize)
	b.pdf.SetTextColor(0, 0, 0)
	rates := []string{
		"INT — Interest: 15%",
		"ROY — Royalties / IP: 10%",
		"TECH — Technical / Management Fees: 17% (prevailing corporate rate)",
		"DIR — Director Fees (non-resident): 22%",
		"RENT — Rental of Moveable Property: 15%",
		"SFC — SRS Withdrawal by Non-Resident: 22%",
	}
	for _, r := range rates {
		b.pdf.SetXY(marginL+16, b.y)
		b.pdf.CellFormat(contentW-20, 10, r, "", 0, "L", false, 0, "")
		b.y += 10
	}

	b.rateNote("Note: Rates may be reduced under applicable Double Taxation Agreements (DTAs)")

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation. The official S45 form must be filed via myTax Portal.",
		"2. Filing Deadline: By the 15th of the 2nd month from the date of payment.",
		"3. The payer is responsible for withholding and remitting tax to IRAS.",
		"4. Reduced rates may apply under Singapore's network of 90+ tax treaties (DTAs).",
		"5. Late filing incurs a penalty of 5% on tax amount + interest at prevailing rate.",
		"6. S45 applies to payments for services, royalties, interest, rent to non-residents.",
	})

	return b.output(w)
}
