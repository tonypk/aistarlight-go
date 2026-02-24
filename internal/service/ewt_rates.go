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

// EWTRates is the BIR RR No. 2-98 rate table as amended by TRAIN Law.
var EWTRates = map[string]EWTRate{
	"WI010": {"WI010", "Professional fees - Individual <3M", decimal.NewFromFloat(0.05), "services", []string{"professional", "consultant", "individual"}},
	"WI020": {"WI020", "Professional fees - Individual >=3M", decimal.NewFromFloat(0.10), "services", []string{"professional", "consultant", "individual", "high value"}},
	"WC010": {"WC010", "Professional fees - Corporation", decimal.NewFromFloat(0.10), "services", []string{"professional", "consultant", "corporation", "corporate"}},
	"WC020": {"WC020", "Professional fees - Corp >720K", decimal.NewFromFloat(0.15), "services", []string{"professional", "consultant", "corporation", "high value"}},
	"WI030": {"WI030", "Rent - Real property", decimal.NewFromFloat(0.05), "services", []string{"rent", "lease", "real property", "office", "space"}},
	"WI040": {"WI040", "Rent - Personal property", decimal.NewFromFloat(0.05), "services", []string{"rent", "lease", "equipment", "vehicle"}},
	"WI050": {"WI050", "Contractors - Individual", decimal.NewFromFloat(0.02), "services", []string{"contractor", "subcontractor", "individual", "construction"}},
	"WC050": {"WC050", "Contractors - Corporation", decimal.NewFromFloat(0.02), "services", []string{"contractor", "subcontractor", "corporation", "construction"}},
	"WC060": {"WC060", "Advertising/Promotions", decimal.NewFromFloat(0.02), "services", []string{"advertising", "promotion", "marketing", "media"}},
	"WI070": {"WI070", "Commission - Individual", decimal.NewFromFloat(0.10), "services", []string{"commission", "agent", "broker", "individual"}},
	"WC070": {"WC070", "Commission - Corporation", decimal.NewFromFloat(0.10), "services", []string{"commission", "agent", "broker", "corporation"}},
	"WI100": {"WI100", "Purchase of goods - Individual >3M", decimal.NewFromFloat(0.01), "goods", []string{"purchase", "goods", "supplies", "individual"}},
	"WC100": {"WC100", "Purchase of goods - Corporation >3M", decimal.NewFromFloat(0.01), "goods", []string{"purchase", "goods", "supplies", "corporation"}},
	"WI120": {"WI120", "Service payments - Individual", decimal.NewFromFloat(0.02), "services", []string{"service", "payment", "individual"}},
	"WC120": {"WC120", "Service payments - Corporation", decimal.NewFromFloat(0.02), "services", []string{"service", "payment", "corporation"}},
	"WI150": {"WI150", "Transport/Delivery/Freight", decimal.NewFromFloat(0.02), "services", []string{"transport", "delivery", "freight", "shipping", "logistics"}},
	"WI160": {"WI160", "Toll fees", decimal.NewFromFloat(0.01), "services", []string{"toll", "expressway", "highway"}},
	"WI170": {"WI170", "Insurance premiums", decimal.NewFromFloat(0.02), "services", []string{"insurance", "premium", "coverage"}},
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
