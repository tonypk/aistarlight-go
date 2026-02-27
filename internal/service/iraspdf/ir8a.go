package iraspdf

import "io"

// GenerateIR8A produces an IRAS IR8A Employer Remuneration Return PDF.
func GenerateIR8A(w io.Writer, data map[string]string, co CompanyData) error {
	b := newBuilder()
	b.titleBanner("Form IR8A", "Return of Employee's Remuneration")

	b.sectionHeader("Employer Information")
	b.infoBlock([][2]string{
		{"Company Name", co.Name},
		{"UEN", co.UEN},
		{"Year of Assessment", getVal(data, "period")},
	})

	// Remuneration
	b.gap(8)
	b.sectionHeader("Part A — Employment Income")
	b.lineItem("1", "Gross Salary / Wages", getVal(data, "gross_salary"), false)
	b.lineItem("2", "Bonus", getVal(data, "bonus"), false)
	b.lineItem("3", "Director Fees", getVal(data, "director_fees"), false)
	b.lineItem("4", "Other Allowances", getVal(data, "other_allowances"), false)
	b.lineItem("5", "Benefits-in-Kind", getVal(data, "benefits_in_kind"), false)
	b.lineItem("6", "Total Gross Remuneration (Lines 1 to 5)", getVal(data, "total_gross"), true)

	// CPF
	b.gap(8)
	b.sectionHeader("Part B — CPF Contributions")
	b.lineItem("7", "Employer CPF Contribution", getVal(data, "employer_cpf"), false)
	b.lineItem("8", "Employee CPF Contribution", getVal(data, "employee_cpf"), false)

	b.rateNote("CPF rates (employee <= 55 years): Employer 17% + Employee 20% = Total 37%")

	// Net Salary
	b.gap(8)
	b.sectionHeader("Part C — Net Salary")
	b.totalHighlight("9", "Net Salary (Line 6 - Line 8)", getVal(data, "net_salary"))

	b.notes([]string{
		"IMPORTANT NOTES:",
		"1. This is a draft computation. The official IR8A must be submitted via AIS (Auto-Inclusion Scheme).",
		"2. Filing Deadline: 1 March of the year following the income year.",
		"3. Employers with >= 5 employees must participate in the Auto-Inclusion Scheme.",
		"4. CPF contribution rates vary by age group. Rates shown are for employees <= 55 years old.",
		"5. Additional forms may be required: Appendix 8A (benefits-in-kind), Appendix 8B (stock options).",
		"6. IR8A covers all forms of remuneration including allowances, bonuses, and benefits-in-kind.",
	})

	return b.output(w)
}
