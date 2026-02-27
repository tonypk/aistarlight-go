package iraspdf

import "io"

// GenerateECI produces an IRAS Estimated Chargeable Income (ECI) PDF.
func GenerateECI(w io.Writer, data map[string]string, co CompanyData) error {
	b := newBuilder()
	b.titleBanner("ECI", "Estimated Chargeable Income")

	b.sectionHeader("Company Information")
	b.infoBlock([][2]string{
		{"Company Name", co.Name},
		{"UEN", co.UEN},
		{"Financial Year End", getVal(data, "period")},
		{"Corporate Tax Rate", "17%"},
	})

	// Revenue
	b.gap(8)
	b.sectionHeader("Revenue")
	b.lineItem("1", "Revenue / Turnover", getVal(data, "revenue"), false)

	// Estimated Income
	b.gap(8)
	b.sectionHeader("Estimated Chargeable Income")
	b.lineItem("2", "Adjusted Profit After Deductions", getVal(data, "adjusted_profit"), false)
	b.lineItem("3", "Less: Capital Allowances", getVal(data, "capital_allowances"), false)
	b.lineItem("4", "Estimated Chargeable Income (Line 2 - Line 3)", getVal(data, "estimated_chargeable_income"), true)

	// Tax Estimation
	b.gap(8)
	b.sectionHeader("Tax Estimation")
	b.lineItem("5", "Partial Tax Exemption", getVal(data, "partial_tax_exemption"), false)
	b.lineItem("6", "Taxable Income After Exemption", getVal(data, "taxable_after_exemption"), false)
	b.totalHighlight("7", "Estimated Tax Payable @ 17%", getVal(data, "estimated_tax"))

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation. The official ECI must be filed via myTax Portal.",
		"2. Filing Deadline: Within 3 months of the financial year-end.",
		"3. Companies with revenue ≤ S$5M and nil ECI may qualify for waiver.",
		"4. Partial tax exemption: 75% on first S$10,000 + 50% on next S$190,000.",
		"5. Start-up exemption (first 3 YAs): 75% on first S$100,000 + 50% on next S$100,000.",
	})

	return b.output(w)
}
