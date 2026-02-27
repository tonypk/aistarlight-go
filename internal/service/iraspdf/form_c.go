package iraspdf

import "io"

// GenerateFormC produces an IRAS Form C Corporate Income Tax PDF.
func GenerateFormC(w io.Writer, data map[string]string, co CompanyData) error {
	b := newBuilder()
	b.titleBanner("Form C", "Corporate Income Tax Return")

	b.sectionHeader("Company Information")
	b.infoBlock([][2]string{
		{"Company Name", co.Name},
		{"UEN", co.UEN},
		{"Year of Assessment", getVal(data, "period")},
		{"Corporate Tax Rate", "17%"},
	})

	// Revenue & Cost
	b.gap(8)
	b.sectionHeader("Part A — Revenue & Gross Profit")
	b.lineItem("1", "Revenue", getVal(data, "revenue"), false)
	b.lineItem("2", "Less: Cost of Sales", getVal(data, "cost_of_sales"), false)
	b.lineItem("3", "Gross Profit (Line 1 - Line 2)", getVal(data, "gross_profit"), true)

	// Adjusted Profit
	b.gap(8)
	b.sectionHeader("Part B — Adjusted Profit")
	b.lineItem("4", "Less: Operating Expenses", getVal(data, "operating_expenses"), false)
	b.lineItem("5", "Add: Other Income", getVal(data, "other_income"), false)
	b.lineItem("6", "Add: Non-Deductible Expenses", getVal(data, "non_deductible"), false)
	b.lineItem("7", "Less: Capital Allowances", getVal(data, "capital_allowances"), false)
	b.lineItem("8", "Adjusted Profit", getVal(data, "adjusted_profit"), true)

	// Deductions
	b.gap(8)
	b.sectionHeader("Part C — Deductions")
	b.lineItem("9", "Donation Deduction (250% of qualifying donations)", getVal(data, "donation_deduction"), false)
	b.lineItem("10", "Losses Utilised (carried forward)", getVal(data, "losses_utilized"), false)

	// Tax Computation
	b.gap(8)
	b.sectionHeader("Part D — Tax Computation")
	b.lineItem("11", "Chargeable Income", getVal(data, "chargeable_income"), true)
	b.lineItem("12", "Less: Partial Tax Exemption", getVal(data, "partial_exemption"), false)
	b.lineItem("13", "Taxable Income", getVal(data, "taxable_income"), true)
	b.totalHighlight("14", "Tax Payable (Line 13 x 17%)", getVal(data, "tax_payable"))

	b.rateNote("Partial Exemption: 75% on first S$10,000 + 50% on next S$190,000 of chargeable income")

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation. The official Form C must be filed via myTax Portal.",
		"2. Filing Deadline: 30 November of the year following the financial year end.",
		"3. Form C is for companies with revenue > S$5M or claiming tax incentives.",
		"4. Companies with revenue <= S$5M may use simplified Form C-S instead.",
		"5. Qualifying donations receive 250% tax deduction (subject to conditions).",
		"6. ECI (Estimated Chargeable Income) must be filed within 3 months of FY end.",
	})

	return b.output(w)
}

// GenerateFormCS produces an IRAS Form C-S Simplified Corporate Tax PDF.
func GenerateFormCS(w io.Writer, data map[string]string, co CompanyData) error {
	b := newBuilder()
	b.titleBanner("Form C-S", "Simplified Corporate Income Tax Return")

	b.sectionHeader("Company Information")
	b.infoBlock([][2]string{
		{"Company Name", co.Name},
		{"UEN", co.UEN},
		{"Year of Assessment", getVal(data, "period")},
		{"Corporate Tax Rate", "17%"},
		{"Eligibility", "Revenue <= S$5,000,000"},
	})

	// Computation
	b.gap(8)
	b.sectionHeader("Tax Computation")
	b.lineItem("1", "Revenue", getVal(data, "revenue"), false)
	b.lineItem("2", "Less: Total Expenses", getVal(data, "total_expenses"), false)
	b.lineItem("3", "Add: Tax Adjustments", getVal(data, "tax_adjustments"), false)
	b.lineItem("4", "Less: Capital Allowances", getVal(data, "capital_allowances"), false)
	b.lineItem("5", "Adjusted Profit", getVal(data, "adjusted_profit"), true)
	b.lineItem("6", "Chargeable Income", getVal(data, "chargeable_income"), true)
	b.lineItem("7", "Less: Partial Tax Exemption", getVal(data, "partial_exemption"), false)
	b.lineItem("8", "Taxable Income", getVal(data, "taxable_income"), true)
	b.totalHighlight("9", "Tax Payable (Line 8 x 17%)", getVal(data, "tax_payable"))

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation. The official Form C-S must be filed via myTax Portal.",
		"2. Filing Deadline: 30 November of the year following the financial year end.",
		"3. Form C-S eligibility: annual revenue <= S$5M, only Singapore income, not claiming tax incentives.",
		"4. If revenue exceeds S$5M, use Form C instead.",
		"5. Partial exemption: 75% on first S$10,000 + 50% on next S$190,000.",
	})

	return b.output(w)
}
