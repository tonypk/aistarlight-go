package service

import (
	"strings"

	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/pkg/lkforms"
)

// LKWHTRate represents a Sri Lanka withholding tax rate entry.
type LKWHTRate struct {
	Code        string          `json:"code"`
	Description string          `json:"description"`
	Rate        decimal.Decimal `json:"rate"`
	Keywords    []string        `json:"keywords"`
}

// LKWHTRates is the reference table for Sri Lanka WHT.
var LKWHTRates = map[string]LKWHTRate{
	"INT":      {"INT", "Interest", decimal.NewFromFloat(0.05), []string{"interest", "loan", "deposit", "bond"}},
	"DIV":      {"DIV", "Dividends", decimal.NewFromFloat(0.15), []string{"dividend", "distribution"}},
	"ROY":      {"ROY", "Royalties", decimal.NewFromFloat(0.14), []string{"royalty", "ip", "patent", "copyright", "license"}},
	"RENT":     {"RENT", "Rent", decimal.NewFromFloat(0.10), []string{"rent", "lease", "property"}},
	"SVC":      {"SVC", "Service Fees (resident)", decimal.NewFromFloat(0.05), []string{"service", "consultancy", "advisory"}},
	"SVC_NR":   {"SVC_NR", "Service Fees (non-resident)", decimal.NewFromFloat(0.14), []string{"service", "foreign", "non-resident"}},
	"MGMT":     {"MGMT", "Management Fees (non-resident)", decimal.NewFromFloat(0.14), []string{"management", "admin fee"}},
	"TECH":     {"TECH", "Technical Fees (non-resident)", decimal.NewFromFloat(0.14), []string{"technical", "engineering"}},
	"INS":      {"INS", "Insurance Premiums (non-resident)", decimal.NewFromFloat(0.14), []string{"insurance", "premium"}},
	"CONTRACT": {"CONTRACT", "Contract Payments", decimal.NewFromFloat(0.05), []string{"contract", "contractor"}},
	"WINNINGS": {"WINNINGS", "Lottery / Gambling Winnings", decimal.NewFromFloat(0.14), []string{"lottery", "gambling", "winnings"}},
}

// GetLKWHTRate returns the WHT rate for a given income type code.
func GetLKWHTRate(code string) (decimal.Decimal, error) {
	code = strings.ToUpper(code)
	if rate, ok := LKWHTRates[code]; ok {
		return rate.Rate, nil
	}
	return decimal.Zero, ErrUnknownATCCode
}

// FindLKWHTByKeywords matches a description to the best LK WHT income type code.
func FindLKWHTByKeywords(description string) string {
	desc := strings.ToLower(description)

	bestCode := ""
	bestScore := 0

	for code, rate := range LKWHTRates {
		score := 0
		for _, kw := range rate.Keywords {
			if strings.Contains(desc, kw) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestCode = code
		}
	}

	return bestCode
}

// ListLKWHTRates returns all LK WHT rates for reference UI.
func ListLKWHTRates() []LKWHTRate {
	// Return in consistent order from the lkforms constants
	rates := make([]LKWHTRate, 0, len(lkforms.WHTNatureOfIncome))
	for _, w := range lkforms.WHTNatureOfIncome {
		if r, ok := LKWHTRates[w.Code]; ok {
			rates = append(rates, r)
		}
	}
	return rates
}
