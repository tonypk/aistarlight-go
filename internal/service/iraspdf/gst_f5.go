package iraspdf

import "io"

// GenerateGSTF5 produces an IRAS GST F5 Return PDF.
func GenerateGSTF5(w io.Writer, data map[string]string, co CompanyData) error {
	b := newBuilder()
	b.titleBanner("GST F5", "Goods and Services Tax Return")

	b.sectionHeader("Company Information")
	b.infoBlock([][2]string{
		{"Company Name", co.Name},
		{"UEN", co.UEN},
		{"Accounting Period", getVal(data, "period")},
		{"GST Rate", getVal(data, "gst_rate") + "%"},
	})

	// Box 1-4: Supplies
	b.gap(8)
	b.sectionHeader("Part A — Value of Supplies")
	b.lineItem("Box 1", "Total value of standard-rated supplies", getVal(data, "box_1_standard_rated_supplies"), false)
	b.lineItem("Box 2", "Total value of zero-rated supplies", getVal(data, "box_2_zero_rated_supplies"), false)
	b.lineItem("Box 3", "Total value of exempt supplies", getVal(data, "box_3_exempt_supplies"), false)
	b.lineItem("Box 4", "Total value of supplies (Box 1 + 2 + 3)", getVal(data, "box_4_total_supplies"), true)

	// Box 5: Purchases
	b.gap(8)
	b.sectionHeader("Part B — Taxable Purchases")
	b.lineItem("Box 5", "Total value of taxable purchases", getVal(data, "box_5_taxable_purchases"), false)

	// Box 6-8: GST Computation
	b.gap(8)
	b.sectionHeader("Part C — GST Computation")
	b.lineItem("Box 6", "Output tax due (Box 1 x GST rate)", getVal(data, "box_6_output_tax"), false)
	b.lineItem("Box 7", "Input tax and refunds claimed", getVal(data, "box_7_input_tax_claimed"), false)
	b.totalHighlight("Box 8", "Net GST payable / (refundable) (Box 6 - Box 7)", getVal(data, "box_8_net_gst"))

	// Box 9-11: Adjustments
	b.gap(8)
	b.sectionHeader("Part D — Adjustments")
	b.lineItem("Box 9", "Bad debt relief", getVal(data, "box_9_bad_debt_relief"), false)
	b.lineItem("Box 10", "Pre-registration input tax", getVal(data, "box_10_pre_reg_input_tax"), false)
	b.lineItem("Box 11", "Tourist refund scheme", getVal(data, "box_11_tourist_refund"), false)

	b.rateNote("GST rate: 9% (effective 1 January 2024)")

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation for review. The official GST F5 must be filed via myTax Portal.",
		"2. Filing Deadline: Within 1 month from the end of the accounting period.",
		"3. Box 4 must equal the sum of Box 1 + Box 2 + Box 3.",
		"4. Box 6 (output tax) = Box 1 x 9%. Any difference may indicate rounding or special schemes.",
		"5. Box 8 = Box 6 - Box 7. If negative, a GST refund may be claimed.",
		"6. Companies with annual taxable turnover > S$1M must be GST-registered.",
	})

	return b.output(w)
}
