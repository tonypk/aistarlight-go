package birforms

import "github.com/shopspring/decimal"

// BIR Form Types
const (
	FormBIR2550M = "BIR_2550M"
	FormBIR2550Q = "BIR_2550Q"
	FormBIR1601C = "BIR_1601C"
	FormBIR0619E = "BIR_0619E"
	FormBIR1701  = "BIR_1701"
	FormBIR1702  = "BIR_1702"
	FormBIR2316  = "BIR_2316"
	FormBIR2307  = "BIR_2307"
	FormSAWT     = "SAWT"
)

// Tax rates
var (
	VATRate     = decimal.NewFromFloat(0.12)
	GovtVATRate = decimal.NewFromFloat(0.05)
	RCIT        = decimal.NewFromFloat(0.25)
	RCITReduced = decimal.NewFromFloat(0.20) // Net taxable income ≤ 5M and total assets ≤ 100M
)

// MCITRate returns the Minimum Corporate Income Tax rate for a given taxable year.
// Per NIRC Sec 27(E)(2):
//   - Standard rate: 2%
//   - CREATE Act temporary reduction: 1% (Jul 1, 2020 – Jun 30, 2023)
//   - For taxable years starting Jul 1, 2023+: reverts to 2%
func MCITRate(taxableYear int) decimal.Decimal {
	if taxableYear >= 2020 && taxableYear <= 2022 {
		return decimal.NewFromFloat(0.01)
	}
	return decimal.NewFromFloat(0.02)
}

// TRAIN Law progressive tax rates (annual income brackets)
type TaxBracket struct {
	Over      decimal.Decimal
	NotOver   decimal.Decimal
	BaseTax   decimal.Decimal
	Rate      decimal.Decimal
}

var TRAINBrackets = []TaxBracket{
	{Over: decimal.Zero, NotOver: decimal.NewFromInt(250000), BaseTax: decimal.Zero, Rate: decimal.Zero},
	{Over: decimal.NewFromInt(250000), NotOver: decimal.NewFromInt(400000), BaseTax: decimal.Zero, Rate: decimal.NewFromFloat(0.15)},
	{Over: decimal.NewFromInt(400000), NotOver: decimal.NewFromInt(800000), BaseTax: decimal.NewFromInt(22500), Rate: decimal.NewFromFloat(0.20)},
	{Over: decimal.NewFromInt(800000), NotOver: decimal.NewFromInt(2000000), BaseTax: decimal.NewFromInt(102500), Rate: decimal.NewFromFloat(0.25)},
	{Over: decimal.NewFromInt(2000000), NotOver: decimal.NewFromInt(8000000), BaseTax: decimal.NewFromInt(402500), Rate: decimal.NewFromFloat(0.30)},
	{Over: decimal.NewFromInt(8000000), NotOver: decimal.NewFromInt(0), BaseTax: decimal.NewFromInt(2202500), Rate: decimal.NewFromFloat(0.35)}, // NotOver=0 means no upper limit
}

// AllFormTypes lists all supported BIR form types.
var AllFormTypes = []string{
	FormBIR2550M, FormBIR2550Q, FormBIR1601C, FormBIR0619E,
	FormBIR1701, FormBIR1702, FormBIR2316, FormBIR2307, FormSAWT,
}
