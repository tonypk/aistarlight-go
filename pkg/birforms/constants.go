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
	MCIT        = decimal.NewFromFloat(0.01)
)

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

// Common ATC codes for EWT
var ATCCodes = map[string]struct {
	Description string
	Rate        decimal.Decimal
}{
	"WI010":  {Description: "Professional fees - individual", Rate: decimal.NewFromFloat(0.05)},
	"WI020":  {Description: "Professional fees - juridical", Rate: decimal.NewFromFloat(0.10)},
	"WI040":  {Description: "Income payments to partners", Rate: decimal.NewFromFloat(0.10)},
	"WI100":  {Description: "Rentals - real property", Rate: decimal.NewFromFloat(0.05)},
	"WI120":  {Description: "Rentals - personal property", Rate: decimal.NewFromFloat(0.05)},
	"WI157":  {Description: "Income payment to supplier of goods", Rate: decimal.NewFromFloat(0.01)},
	"WI158":  {Description: "Income payment to supplier of services", Rate: decimal.NewFromFloat(0.02)},
	"WC010":  {Description: "Compensation - regular", Rate: decimal.Zero},
	"WC011":  {Description: "Compensation - MWE", Rate: decimal.Zero},
	"WC100":  {Description: "Compensation - supplementary", Rate: decimal.Zero},
	"WB010":  {Description: "Broker fees", Rate: decimal.NewFromFloat(0.10)},
	"WI530":  {Description: "Government money payments to suppliers of goods", Rate: decimal.NewFromFloat(0.01)},
	"WI540":  {Description: "Government money payments to suppliers of services", Rate: decimal.NewFromFloat(0.02)},
	"WI600":  {Description: "Tolling fees", Rate: decimal.NewFromFloat(0.05)},
	"WI640":  {Description: "Insurance commissions", Rate: decimal.NewFromFloat(0.10)},
	"WI700":  {Description: "Directors fees", Rate: decimal.NewFromFloat(0.20)},
	"WI160":  {Description: "Commissions/agent fees", Rate: decimal.NewFromFloat(0.10)},
	"WV010":  {Description: "VAT Withholding - government goods", Rate: decimal.NewFromFloat(0.05)},
	"WV020":  {Description: "VAT Withholding - government services", Rate: decimal.NewFromFloat(0.05)},
	"WI150":  {Description: "Income payment for purchase of minerals", Rate: decimal.NewFromFloat(0.05)},
	"WI010A": {Description: "Medical practitioner", Rate: decimal.NewFromFloat(0.05)},
	"WI155":  {Description: "Gross purchase of agricultural products", Rate: decimal.NewFromFloat(0.01)},
	"WI159":  {Description: "Income to beneficiaries of estates/trusts", Rate: decimal.NewFromFloat(0.15)},
}

// AllFormTypes lists all supported BIR form types.
var AllFormTypes = []string{
	FormBIR2550M, FormBIR2550Q, FormBIR1601C, FormBIR0619E,
	FormBIR1701, FormBIR1702, FormBIR2316, FormBIR2307, FormSAWT,
}
