package service

import (
	"strings"

	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/pkg/irasforms"
)

// SGWHTRate represents a Singapore S45 withholding tax rate entry.
type SGWHTRate struct {
	Code        string          `json:"code"`
	Description string          `json:"description"`
	Rate        decimal.Decimal `json:"rate"`
	Keywords    []string        `json:"keywords"`
}

// SGWHTRates is the reference table for Singapore S45 non-resident WHT.
var SGWHTRates = map[string]SGWHTRate{
	"INT":  {"INT", "Interest", decimal.NewFromFloat(0.15), []string{"interest", "loan", "deposit", "bond"}},
	"ROY":  {"ROY", "Royalties / IP", decimal.NewFromFloat(0.10), []string{"royalty", "ip", "patent", "copyright", "license"}},
	"TECH": {"TECH", "Technical Fees", decimal.NewFromFloat(0.17), []string{"technical", "consulting", "advisory"}},
	"MGMT": {"MGMT", "Management Fees", decimal.NewFromFloat(0.17), []string{"management", "admin fee"}},
	"DIR":  {"DIR", "Director Fees (non-resident)", decimal.NewFromFloat(0.22), []string{"director", "board"}},
	"RENT": {"RENT", "Rental of Moveable Property", decimal.NewFromFloat(0.15), []string{"rent", "lease", "equipment", "moveable"}},
	"SFC":  {"SFC", "SRS Withdrawal by Non-Resident", decimal.NewFromFloat(0.22), []string{"srs", "supplementary", "retirement"}},
}

// GetSGWHTRate returns the WHT rate for a given income type code.
func GetSGWHTRate(code string) (decimal.Decimal, error) {
	if nature, ok := irasforms.WHTNatureOfIncome[strings.ToUpper(code)]; ok {
		return nature.Rate, nil
	}
	return decimal.Zero, ErrUnknownATCCode
}

// GetSGWHTDescription returns the description for a given WHT income type.
func GetSGWHTDescription(code string) string {
	if nature, ok := irasforms.WHTNatureOfIncome[strings.ToUpper(code)]; ok {
		return nature.Description
	}
	return "Other income"
}

// FindSGWHTByKeywords matches a description to the best S45 income type code.
func FindSGWHTByKeywords(description string) string {
	desc := strings.ToLower(description)

	bestCode := ""
	bestScore := 0

	for code, rate := range SGWHTRates {
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

// ListSGWHTRates returns all S45 WHT rates for reference UI.
func ListSGWHTRates() []SGWHTRate {
	rates := make([]SGWHTRate, 0, len(SGWHTRates))
	for _, r := range SGWHTRates {
		rates = append(rates, r)
	}
	return rates
}
