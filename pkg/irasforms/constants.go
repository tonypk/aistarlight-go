package irasforms

import "github.com/shopspring/decimal"

// IRAS Form Types
const (
	FormGSTF5  = "IRAS_GST_F5"
	FormFormC  = "IRAS_FORM_C"
	FormFormCS = "IRAS_FORM_CS"
	FormFormB  = "IRAS_FORM_B"
	FormIR8A   = "IRAS_IR8A"
	FormS45    = "IRAS_S45"
	FormECI    = "IRAS_ECI"
)

// Tax rates — Singapore
var (
	GSTRate       = decimal.NewFromFloat(0.09)  // 9% since Jan 2024
	CorporateRate = decimal.NewFromFloat(0.17)  // 17% flat
	CPFEmployer   = decimal.NewFromFloat(0.17)  // 17% employer contribution (≤55 years old)
	CPFEmployee   = decimal.NewFromFloat(0.20)  // 20% employee contribution (≤55 years old)
	CPFOWCeiling  = decimal.NewFromInt(6800)    // Ordinary Wage ceiling: S$6,800/month
)

// TaxBracket for Singapore progressive income tax.
type TaxBracket struct {
	Over    decimal.Decimal
	NotOver decimal.Decimal // 0 means no upper limit
	BaseTax decimal.Decimal
	Rate    decimal.Decimal
}

// SGTaxBrackets — Singapore Resident Individual Income Tax (YA 2024 onwards).
var SGTaxBrackets = []TaxBracket{
	{Over: decimal.Zero, NotOver: decimal.NewFromInt(20000), BaseTax: decimal.Zero, Rate: decimal.Zero},
	{Over: decimal.NewFromInt(20000), NotOver: decimal.NewFromInt(30000), BaseTax: decimal.Zero, Rate: decimal.NewFromFloat(0.02)},
	{Over: decimal.NewFromInt(30000), NotOver: decimal.NewFromInt(40000), BaseTax: decimal.NewFromInt(200), Rate: decimal.NewFromFloat(0.035)},
	{Over: decimal.NewFromInt(40000), NotOver: decimal.NewFromInt(80000), BaseTax: decimal.NewFromInt(550), Rate: decimal.NewFromFloat(0.07)},
	{Over: decimal.NewFromInt(80000), NotOver: decimal.NewFromInt(120000), BaseTax: decimal.NewFromInt(3350), Rate: decimal.NewFromFloat(0.115)},
	{Over: decimal.NewFromInt(120000), NotOver: decimal.NewFromInt(160000), BaseTax: decimal.NewFromInt(7950), Rate: decimal.NewFromFloat(0.15)},
	{Over: decimal.NewFromInt(160000), NotOver: decimal.NewFromInt(200000), BaseTax: decimal.NewFromInt(13950), Rate: decimal.NewFromFloat(0.18)},
	{Over: decimal.NewFromInt(200000), NotOver: decimal.NewFromInt(240000), BaseTax: decimal.NewFromInt(21150), Rate: decimal.NewFromFloat(0.19)},
	{Over: decimal.NewFromInt(240000), NotOver: decimal.NewFromInt(280000), BaseTax: decimal.NewFromInt(28750), Rate: decimal.NewFromFloat(0.195)},
	{Over: decimal.NewFromInt(280000), NotOver: decimal.NewFromInt(320000), BaseTax: decimal.NewFromInt(36550), Rate: decimal.NewFromFloat(0.20)},
	{Over: decimal.NewFromInt(320000), NotOver: decimal.NewFromInt(500000), BaseTax: decimal.NewFromInt(44550), Rate: decimal.NewFromFloat(0.22)},
	{Over: decimal.NewFromInt(500000), NotOver: decimal.NewFromInt(1000000), BaseTax: decimal.NewFromInt(84150), Rate: decimal.NewFromFloat(0.23)},
	{Over: decimal.NewFromInt(1000000), NotOver: decimal.Zero, BaseTax: decimal.NewFromInt(199150), Rate: decimal.NewFromFloat(0.24)},
}

// WHTIncomeType describes a withholding tax nature of income for non-residents.
type WHTIncomeType struct {
	Description string
	Rate        decimal.Decimal
}

// WHTNatureOfIncome — S45 Withholding Tax rates for non-resident payments.
var WHTNatureOfIncome = map[string]WHTIncomeType{
	"INT":  {Description: "Interest", Rate: decimal.NewFromFloat(0.15)},
	"ROY":  {Description: "Royalties / IP", Rate: decimal.NewFromFloat(0.10)},
	"TECH": {Description: "Technical Fees", Rate: decimal.NewFromFloat(0.17)},
	"MGMT": {Description: "Management Fees", Rate: decimal.NewFromFloat(0.17)},
	"DIR":  {Description: "Director Fees (non-resident)", Rate: decimal.NewFromFloat(0.22)},
	"RENT": {Description: "Rental of Moveable Property", Rate: decimal.NewFromFloat(0.15)},
	"SFC":  {Description: "SRS Withdrawal by Non-Resident", Rate: decimal.NewFromFloat(0.22)},
}

// Corporate tax exemption thresholds (partial exemption scheme)
var (
	PartialExemptionFirst = decimal.NewFromInt(10000)  // First S$10,000 @ 75% exempt
	PartialExemptionNext  = decimal.NewFromInt(190000) // Next S$190,000 @ 50% exempt
	FormCSRevenueLimit    = decimal.NewFromInt(5000000) // Form C-S eligibility: ≤ S$5M revenue
)

// AllFormTypes lists all supported IRAS form types.
var AllFormTypes = []string{
	FormGSTF5, FormFormC, FormFormCS, FormFormB, FormIR8A, FormS45, FormECI,
}
