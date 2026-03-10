package service

import "strings"

// CompanyProfile holds information used to determine required BIR forms.
type CompanyProfile struct {
	VATRegistered bool   `json:"vat_registered"`
	HasEmployees  bool   `json:"has_employees"`
	EntityType    string `json:"entity_type"` // "individual" or "corporation"
	Industry      string `json:"industry"`
	RevenueScale  string `json:"revenue_scale"` // "micro", "small", "medium", "large"
}

// FormRecommendation describes a form the company should file.
type FormRecommendation struct {
	FormType    string `json:"form_type"`
	Name        string `json:"name"`
	Frequency   string `json:"frequency"`
	Required    bool   `json:"required"`
	Reason      string `json:"reason"`
	DeadlineDay int    `json:"deadline_day,omitempty"` // day of month (filing deadline)
}

// FormRouter recommends which BIR forms a company needs to file.
type FormRouter struct{}

// NewFormRouter creates a form router.
func NewFormRouter() *FormRouter {
	return &FormRouter{}
}

// RecommendForms returns the list of forms a company should file based on jurisdiction.
func (r *FormRouter) RecommendForms(profile CompanyProfile, jurisdiction string) []FormRecommendation {
	switch jurisdiction {
	case "SG":
		return r.recommendSGForms(profile)
	case "LK":
		return r.recommendLKForms(profile)
	default:
		return r.recommendPHForms(profile)
	}
}

func (r *FormRouter) recommendPHForms(profile CompanyProfile) []FormRecommendation {
	var recs []FormRecommendation

	entityType := strings.ToLower(profile.EntityType)
	if entityType == "" {
		entityType = "corporation"
	}

	// VAT forms
	if profile.VATRegistered {
		recs = append(recs,
			FormRecommendation{
				FormType:    "BIR_2550M",
				Name:        "Monthly Value-Added Tax Declaration",
				Frequency:   "monthly",
				Required:    true,
				Reason:      "VAT-registered taxpayer must file monthly VAT declaration",
				DeadlineDay: 20,
			},
			FormRecommendation{
				FormType:    "BIR_2550Q",
				Name:        "Quarterly Value-Added Tax Return",
				Frequency:   "quarterly",
				Required:    true,
				Reason:      "VAT-registered taxpayer must file quarterly VAT return",
				DeadlineDay: 25,
			},
			FormRecommendation{
				FormType:    "BIR_0619E",
				Name:        "Monthly Remittance of Creditable Income Taxes Withheld (Expanded)",
				Frequency:   "monthly",
				Required:    true,
				Reason:      "Withholding agent must remit expanded withholding tax monthly",
				DeadlineDay: 10,
			},
			FormRecommendation{
				FormType:    "SAWT",
				Name:        "Summary Alphalist of Withholding Taxes",
				Frequency:   "quarterly",
				Required:    true,
				Reason:      "Required attachment to quarterly VAT return",
			},
		)
	}

	// Compensation/employee forms
	if profile.HasEmployees {
		recs = append(recs,
			FormRecommendation{
				FormType:    "BIR_1601C",
				Name:        "Monthly Remittance of Withholding Tax on Compensation",
				Frequency:   "monthly",
				Required:    true,
				Reason:      "Employer must remit compensation withholding tax monthly",
				DeadlineDay: 10,
			},
			FormRecommendation{
				FormType:    "BIR_2316",
				Name:        "Certificate of Compensation Payment / Tax Withheld",
				Frequency:   "annual",
				Required:    true,
				Reason:      "Must be issued to all employees annually",
			},
		)
	}

	// Annual income tax
	switch entityType {
	case "individual":
		recs = append(recs, FormRecommendation{
			FormType:  "BIR_1701",
			Name:      "Annual Income Tax Return (Individuals)",
			Frequency: "annual",
			Required:  true,
			Reason:    "All individual taxpayers must file annual ITR",
		})
	default: // corporation, partnership
		recs = append(recs, FormRecommendation{
			FormType:  "BIR_1702",
			Name:      "Annual Income Tax Return (Corporations)",
			Frequency: "annual",
			Required:  true,
			Reason:    "All corporate taxpayers must file annual ITR",
		})
	}

	// BIR 2307 (optional recommendation for withholding agents)
	if profile.VATRegistered {
		recs = append(recs, FormRecommendation{
			FormType:  "BIR_2307",
			Name:      "Certificate of Creditable Tax Withheld at Source",
			Frequency: "quarterly",
			Required:  false,
			Reason:    "Issue to payees/suppliers when withholding expanded/creditable taxes",
		})
	}

	return recs
}

func (r *FormRouter) recommendSGForms(profile CompanyProfile) []FormRecommendation {
	var recs []FormRecommendation

	entityType := strings.ToLower(profile.EntityType)
	if entityType == "" {
		entityType = "corporation"
	}

	// GST (Singapore's VAT equivalent)
	if profile.VATRegistered {
		recs = append(recs, FormRecommendation{
			FormType:  "IRAS_GST_F5",
			Name:      "GST Return (F5)",
			Frequency: "quarterly",
			Required:  true,
			Reason:    "GST-registered businesses must file quarterly GST returns",
		})
	}

	// Employee forms
	if profile.HasEmployees {
		recs = append(recs, FormRecommendation{
			FormType:  "IRAS_IR8A",
			Name:      "Return of Employee's Remuneration (IR8A)",
			Frequency: "annual",
			Required:  true,
			Reason:    "Employers must report employee remuneration annually by 1 Mar",
		})
	}

	// Corporate / Individual income tax
	switch entityType {
	case "individual":
		recs = append(recs, FormRecommendation{
			FormType:  "IRAS_FORM_B",
			Name:      "Income Tax Return (Individuals) — Form B",
			Frequency: "annual",
			Required:  true,
			Reason:    "Self-employed individuals must file Form B by 18 Apr",
		})
	default: // corporation
		// Estimated Chargeable Income
		recs = append(recs, FormRecommendation{
			FormType:  "IRAS_ECI",
			Name:      "Estimated Chargeable Income (ECI)",
			Frequency: "annual",
			Required:  true,
			Reason:    "Companies must file ECI within 3 months of financial year-end",
		})

		// Determine Form C vs C-S based on revenue
		if profile.RevenueScale == "micro" || profile.RevenueScale == "small" {
			recs = append(recs, FormRecommendation{
				FormType:  "IRAS_FORM_CS",
				Name:      "Corporate Tax Return (Simplified) — Form C-S",
				Frequency: "annual",
				Required:  true,
				Reason:    "Companies with revenue ≤ S$5M can file simplified Form C-S by 30 Nov",
			})
		} else {
			recs = append(recs, FormRecommendation{
				FormType:  "IRAS_FORM_C",
				Name:      "Corporate Tax Return — Form C",
				Frequency: "annual",
				Required:  true,
				Reason:    "Companies must file Form C by 30 Nov",
			})
		}
	}

	// S45 WHT (always recommended for companies that may have non-resident payments)
	recs = append(recs, FormRecommendation{
		FormType:  "IRAS_S45",
		Name:      "Withholding Tax on Non-Resident Payments (S45)",
		Frequency: "per-payment",
		Required:  false,
		Reason:    "File within 15 days of the 2nd month from payment date to non-residents",
	})

	return recs
}

func (r *FormRouter) recommendLKForms(profile CompanyProfile) []FormRecommendation {
	var recs []FormRecommendation

	entityType := strings.ToLower(profile.EntityType)
	if entityType == "" {
		entityType = "corporation"
	}

	// VAT Return (mandatory for VAT-registered)
	if profile.VATRegistered {
		recs = append(recs, FormRecommendation{
			FormType:  "IRDSL_VAT_RETURN",
			Name:      "VAT Return",
			Frequency: "monthly",
			Required:  true,
			Reason:    "VAT-registered businesses must file monthly VAT returns by the 20th",
		})
	}

	// Employee forms
	if profile.HasEmployees {
		recs = append(recs, FormRecommendation{
			FormType:  "IRDSL_PAYE",
			Name:      "PAYE / APIT Return",
			Frequency: "monthly",
			Required:  true,
			Reason:    "Employers must remit APIT by 15th of following month",
		})
		recs = append(recs, FormRecommendation{
			FormType:  "IRDSL_APIT",
			Name:      "Annual APIT Statement",
			Frequency: "annual",
			Required:  true,
			Reason:    "Annual statement of APIT deductions due 30 Apr",
		})
	}

	// Corporate / Individual income tax
	switch entityType {
	case "individual", "sole_proprietor":
		recs = append(recs, FormRecommendation{
			FormType:  "IRDSL_IT_RETURN",
			Name:      "Individual Income Tax Return",
			Frequency: "annual",
			Required:  true,
			Reason:    "Individuals must file income tax return by 30 Nov",
		})
	default:
		recs = append(recs, FormRecommendation{
			FormType:  "IRDSL_CIT",
			Name:      "Corporate Income Tax Return",
			Frequency: "annual",
			Required:  true,
			Reason:    "Companies must file CIT return by 30 Nov",
		})
	}

	// WHT (recommended for companies with non-resident payments)
	recs = append(recs, FormRecommendation{
		FormType:  "IRDSL_WHT",
		Name:      "Withholding Tax Return",
		Frequency: "per-payment",
		Required:  false,
		Reason:    "WHT on specified payments due by 15th of following month",
	})

	// SSCL (mandatory for businesses with turnover > LKR 120M/quarter)
	recs = append(recs, FormRecommendation{
		FormType:  "IRDSL_SSCL",
		Name:      "Social Security Contribution Levy",
		Frequency: "quarterly",
		Required:  false,
		Reason:    "SSCL at 2.5% on turnover exceeding LKR 120M/quarter",
	})

	return recs
}
