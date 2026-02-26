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

// RecommendForms returns the list of forms a company should file.
func (r *FormRouter) RecommendForms(profile CompanyProfile) []FormRecommendation {
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
