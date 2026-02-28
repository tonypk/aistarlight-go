package service

import (
	"errors"
	"strings"

	"github.com/shopspring/decimal"
)

// EWTRate represents a single ATC code and its withholding tax rate.
type EWTRate struct {
	ATCCode     string          `json:"atc_code"`
	Description string          `json:"description"`
	Rate        decimal.Decimal `json:"rate"`
	Category    string          `json:"category"` // services, goods
	Keywords    []string        `json:"keywords"`
}

var ErrUnknownATCCode = errors.New("unknown ATC code")

// EWTRates is the canonical BIR ATC rate table per RR No. 2-98 as amended by TRAIN Law.
// This is the single source of truth for all ATC code lookups.
var EWTRates = map[string]EWTRate{
	// Professional/talent fees
	"WI010":  {"WI010", "Professional fees - Individual <3M", decimal.NewFromFloat(0.05), "services", []string{"professional", "consultant", "individual"}},
	"WI010A": {"WI010A", "Medical practitioner", decimal.NewFromFloat(0.05), "services", []string{"medical", "doctor", "physician", "dentist"}},
	"WI020":  {"WI020", "Professional fees - Individual >=3M", decimal.NewFromFloat(0.10), "services", []string{"professional", "consultant", "individual", "high value"}},
	"WC010":  {"WC010", "Professional fees - Corporation", decimal.NewFromFloat(0.10), "services", []string{"professional", "consultant", "corporation", "corporate"}},
	"WC020":  {"WC020", "Professional fees - Corp >720K", decimal.NewFromFloat(0.15), "services", []string{"professional", "consultant", "corporation", "high value"}},

	// Rent
	"WI030": {"WI030", "Rent - Real property", decimal.NewFromFloat(0.05), "services", []string{"rent", "lease", "real property", "office", "space"}},
	"WI040": {"WI040", "Rent - Personal property", decimal.NewFromFloat(0.05), "services", []string{"rent", "lease", "equipment", "vehicle"}},

	// Contractors/subcontractors
	"WI050": {"WI050", "Contractors - Individual", decimal.NewFromFloat(0.02), "services", []string{"contractor", "subcontractor", "individual", "construction"}},
	"WC050": {"WC050", "Contractors - Corporation", decimal.NewFromFloat(0.02), "services", []string{"contractor", "subcontractor", "corporation", "construction"}},

	// Advertising
	"WC060": {"WC060", "Advertising/Promotions", decimal.NewFromFloat(0.02), "services", []string{"advertising", "promotion", "marketing", "media"}},

	// Commissions/agent fees
	"WI070": {"WI070", "Commission - Individual", decimal.NewFromFloat(0.10), "services", []string{"commission", "agent", "individual"}},
	"WC070": {"WC070", "Commission - Corporation", decimal.NewFromFloat(0.10), "services", []string{"commission", "agent", "corporation"}},
	"WB010": {"WB010", "Broker fees", decimal.NewFromFloat(0.10), "services", []string{"broker", "brokerage"}},

	// Purchase of goods
	"WI100": {"WI100", "Purchase of goods - Individual >3M", decimal.NewFromFloat(0.01), "goods", []string{"purchase", "goods", "supplies", "individual"}},
	"WC100": {"WC100", "Purchase of goods - Corporation >3M", decimal.NewFromFloat(0.01), "goods", []string{"purchase", "goods", "supplies", "corporation"}},
	"WI155": {"WI155", "Gross purchase of agricultural products", decimal.NewFromFloat(0.01), "goods", []string{"agriculture", "farm", "crop", "produce"}},
	"WI157": {"WI157", "Income payment to supplier of goods", decimal.NewFromFloat(0.01), "goods", []string{"supplier", "goods", "supply"}},

	// Service payments
	"WI120": {"WI120", "Service payments - Individual", decimal.NewFromFloat(0.02), "services", []string{"service", "payment", "individual"}},
	"WC120": {"WC120", "Service payments - Corporation", decimal.NewFromFloat(0.02), "services", []string{"service", "payment", "corporation"}},
	"WI158": {"WI158", "Income payment to supplier of services", decimal.NewFromFloat(0.02), "services", []string{"supplier", "services", "supply"}},

	// Transport/logistics
	"WI150": {"WI150", "Transport/Delivery/Freight", decimal.NewFromFloat(0.02), "services", []string{"transport", "delivery", "freight", "shipping", "logistics"}},

	// Tolling/manufacturing
	"WI160": {"WI160", "Tolling/manufacturing fees", decimal.NewFromFloat(0.05), "services", []string{"tolling", "manufacturing", "processing"}},

	// Insurance
	"WI170": {"WI170", "Insurance premiums", decimal.NewFromFloat(0.02), "services", []string{"insurance", "premium", "coverage"}},
	"WI640": {"WI640", "Insurance commissions", decimal.NewFromFloat(0.10), "services", []string{"insurance", "commission"}},

	// Directors/trustees
	"WI700": {"WI700", "Directors fees", decimal.NewFromFloat(0.20), "services", []string{"director", "trustee", "board"}},

	// Estates/trusts
	"WI159": {"WI159", "Income to beneficiaries of estates/trusts", decimal.NewFromFloat(0.15), "services", []string{"estate", "trust", "beneficiary"}},

	// Government money payments
	"WI530": {"WI530", "Government payment to suppliers of goods", decimal.NewFromFloat(0.01), "goods", []string{"government", "goods", "procurement"}},
	"WI540": {"WI540", "Government payment to suppliers of services", decimal.NewFromFloat(0.02), "services", []string{"government", "services", "procurement"}},

	// VAT Withholding (government)
	"WV010": {"WV010", "VAT Withholding - government goods", decimal.NewFromFloat(0.05), "goods", []string{"vat", "government", "goods", "withholding"}},
	"WV020": {"WV020", "VAT Withholding - government services", decimal.NewFromFloat(0.05), "services", []string{"vat", "government", "services", "withholding"}},
}

// GetEWTRate returns the rate for a given ATC code.
func GetEWTRate(atcCode string) (decimal.Decimal, error) {
	r, ok := EWTRates[strings.ToUpper(atcCode)]
	if !ok {
		return decimal.Zero, ErrUnknownATCCode
	}
	return r.Rate, nil
}

// GetEWTIncomeType returns the description for a given ATC code.
func GetEWTIncomeType(atcCode string) string {
	r, ok := EWTRates[strings.ToUpper(atcCode)]
	if !ok {
		return "Other income"
	}
	return r.Description
}

// FindATCByKeywords matches a description to the best ATC code.
// supplierType should be "individual" or "corporation".
func FindATCByKeywords(description string, supplierType string) string {
	desc := strings.ToLower(description)
	supplierType = strings.ToLower(supplierType)

	// Prefer WI* for individual, WC* for corporation
	prefix := "WC"
	if supplierType == "individual" {
		prefix = "WI"
	}

	bestCode := ""
	bestScore := 0

	for code, rate := range EWTRates {
		score := 0
		for _, kw := range rate.Keywords {
			if strings.Contains(desc, kw) {
				score++
			}
		}
		if score == 0 {
			continue
		}

		// Prefer matching supplier type prefix
		if strings.HasPrefix(code, prefix) {
			score += 2
		}

		if score > bestScore {
			bestScore = score
			bestCode = code
		}
	}

	return bestCode
}

// ListEWTRates returns all ATC rates for reference UI.
func ListEWTRates() []EWTRate {
	rates := make([]EWTRate, 0, len(EWTRates))
	for _, r := range EWTRates {
		rates = append(rates, r)
	}
	return rates
}
