package lkforms

import "github.com/shopspring/decimal"

// IRD Sri Lanka Form Types
const (
	FormVATReturn   = "IRDSL_VAT_RETURN"   // Monthly/Quarterly VAT Return
	FormITReturn    = "IRDSL_IT_RETURN"     // Annual Income Tax Return
	FormPAYE        = "IRDSL_PAYE"          // Pay-As-You-Earn Monthly Return
	FormWHT         = "IRDSL_WHT"           // Withholding Tax Return
	FormAPIT        = "IRDSL_APIT"          // Advanced Personal Income Tax
	FormSSCL        = "IRDSL_SSCL"          // Social Security Contribution Levy
	FormESC         = "IRDSL_ESC"           // Economic Service Charge (abolished 2020, legacy)
	FormSVAT        = "IRDSL_SVAT"          // Simplified VAT Declaration
	FormCIT         = "IRDSL_CIT"           // Corporate Income Tax Return
	FormPartnership = "IRDSL_PARTNERSHIP"   // Partnership Return
)

// Tax rates — Sri Lanka (as of 2024/2025)
var (
	VATRate      = decimal.NewFromFloat(0.18)  // 18% VAT (raised from 15% in Jan 2024)
	SSCLRate     = decimal.NewFromFloat(0.025) // 2.5% Social Security Contribution Levy
	CorporateStd = decimal.NewFromFloat(0.30)  // 30% standard corporate income tax
	CorporateSME = decimal.NewFromFloat(0.14)  // 14% for SMEs (turnover ≤ LKR 500M, first LKR 10M)
	CorporateExport = decimal.NewFromFloat(0.14) // 14% concessionary for export
	EPFEmployer  = decimal.NewFromFloat(0.12)  // 12% Employees' Provident Fund (employer)
	EPFEmployee  = decimal.NewFromFloat(0.08)  // 8% EPF (employee)
	ETFRate      = decimal.NewFromFloat(0.03)  // 3% Employees' Trust Fund (employer only)
)

// TaxBracket for Sri Lanka progressive personal income tax.
type TaxBracket struct {
	Over    decimal.Decimal
	NotOver decimal.Decimal // 0 means no upper limit
	BaseTax decimal.Decimal
	Rate    decimal.Decimal
}

// LKIncomeTaxBrackets — Sri Lanka Resident Individual Income Tax (YA 2024/2025 onwards).
// Inland Revenue Act No. 24 of 2017 as amended:
//   First LKR 500,000 — Exempt (tax-free allowance)
//   Next  LKR 500,000 — 6%
//   Next  LKR 500,000 — 12%
//   Next  LKR 500,000 — 18%
//   Next  LKR 500,000 — 24%
//   Next  LKR 500,000 — 30%
//   Balance           — 36%
var LKIncomeTaxBrackets = []TaxBracket{
	{Over: decimal.Zero, NotOver: decimal.NewFromInt(500000), BaseTax: decimal.Zero, Rate: decimal.Zero},
	{Over: decimal.NewFromInt(500000), NotOver: decimal.NewFromInt(1000000), BaseTax: decimal.Zero, Rate: decimal.NewFromFloat(0.06)},
	{Over: decimal.NewFromInt(1000000), NotOver: decimal.NewFromInt(1500000), BaseTax: decimal.NewFromInt(30000), Rate: decimal.NewFromFloat(0.12)},
	{Over: decimal.NewFromInt(1500000), NotOver: decimal.NewFromInt(2000000), BaseTax: decimal.NewFromInt(90000), Rate: decimal.NewFromFloat(0.18)},
	{Over: decimal.NewFromInt(2000000), NotOver: decimal.NewFromInt(2500000), BaseTax: decimal.NewFromInt(180000), Rate: decimal.NewFromFloat(0.24)},
	{Over: decimal.NewFromInt(2500000), NotOver: decimal.NewFromInt(3000000), BaseTax: decimal.NewFromInt(300000), Rate: decimal.NewFromFloat(0.30)},
	{Over: decimal.NewFromInt(3000000), NotOver: decimal.Zero, BaseTax: decimal.NewFromInt(450000), Rate: decimal.NewFromFloat(0.36)},
}

// WHTIncomeType describes a withholding tax nature of income.
type WHTIncomeType struct {
	Code        string
	Description string
	Rate        decimal.Decimal
}

// WHTNatureOfIncome — Sri Lanka WHT rates under Inland Revenue Act.
var WHTNatureOfIncome = []WHTIncomeType{
	{"INT", "Interest", decimal.NewFromFloat(0.05)},
	{"DIV", "Dividends", decimal.NewFromFloat(0.15)},
	{"ROY", "Royalties", decimal.NewFromFloat(0.14)},
	{"RENT", "Rent", decimal.NewFromFloat(0.10)},
	{"SVC", "Service Fees (resident)", decimal.NewFromFloat(0.05)},
	{"SVC_NR", "Service Fees (non-resident)", decimal.NewFromFloat(0.14)},
	{"MGMT", "Management Fees (non-resident)", decimal.NewFromFloat(0.14)},
	{"TECH", "Technical Fees (non-resident)", decimal.NewFromFloat(0.14)},
	{"INS", "Insurance Premiums (non-resident)", decimal.NewFromFloat(0.14)},
	{"CONTRACT", "Contract Payments (resident)", decimal.NewFromFloat(0.05)},
	{"WINNINGS", "Lottery / Gambling Winnings", decimal.NewFromFloat(0.14)},
}

// AllFormTypes lists all supported IRD Sri Lanka form types.
var AllFormTypes = []string{
	FormVATReturn, FormITReturn, FormPAYE, FormWHT, FormAPIT,
	FormSSCL, FormSVAT, FormCIT, FormPartnership,
}
