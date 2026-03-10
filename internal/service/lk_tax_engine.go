package service

import (
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/pkg/lkforms"
)

// CalculateLKVATReturn computes the Sri Lanka VAT Return.
//
// Key fields:
//   standard_rated_supplies — total standard-rated supplies (18% VAT)
//   zero_rated_supplies     — export / BOI zero-rated
//   exempt_supplies         — exempt (education, healthcare, etc.)
//   taxable_purchases       — purchases eligible for input VAT credit
//   input_vat_claimed       — actual input VAT claimed
//   svat_credits            — SVAT scheme credits
func CalculateLKVATReturn(input map[string]interface{}) (TaxResult, error) {
	stdRated := toDecimal(input["standard_rated_supplies"])
	zeroRated := toDecimal(input["zero_rated_supplies"])
	exemptSupplies := toDecimal(input["exempt_supplies"])
	inputVATClaimed := toDecimal(input["input_vat_claimed"])
	svatCredits := toDecimal(input["svat_credits"])

	// Aggregate from raw sales_data if provided
	salesData := toMapSlice(input["sales_data"])
	for _, row := range salesData {
		amount := toDecimal(row["amount"])
		switch toString(row["vat_type"]) {
		case "standard", "standard_rated", "vatable":
			stdRated = stdRated.Add(amount)
		case "zero", "zero_rated":
			zeroRated = zeroRated.Add(amount)
		case "exempt":
			exemptSupplies = exemptSupplies.Add(amount)
		default:
			stdRated = stdRated.Add(amount)
		}
	}

	// Aggregate from raw purchases_data
	purchasesData := toMapSlice(input["purchases_data"])
	for _, row := range purchasesData {
		amount := toDecimal(row["amount"])
		vatAmount := toDecimal(row["vat_amount"])
		if vatAmount.IsZero() {
			vatAmount = amount.Mul(lkforms.VATRate)
		}
		inputVATClaimed = inputVATClaimed.Add(vatAmount)
	}

	totalSupplies := stdRated.Add(zeroRated).Add(exemptSupplies)
	outputVAT := stdRated.Mul(lkforms.VATRate)
	netVAT := outputVAT.Sub(inputVATClaimed).Sub(svatCredits)

	return TaxResult{
		"standard_rated_supplies": stdRated.StringFixed(2),
		"zero_rated_supplies":     zeroRated.StringFixed(2),
		"exempt_supplies":         exemptSupplies.StringFixed(2),
		"total_supplies":          totalSupplies.StringFixed(2),
		"output_vat":              outputVAT.StringFixed(2),
		"input_vat_claimed":       inputVATClaimed.StringFixed(2),
		"svat_credits":            svatCredits.StringFixed(2),
		"net_vat_payable":         netVAT.StringFixed(2),
	}, nil
}

// CalculateLKCIT computes the Sri Lanka Corporate Income Tax Return.
func CalculateLKCIT(input map[string]interface{}) (TaxResult, error) {
	revenue := toDecimal(input["revenue"])
	expenses := toDecimal(input["expenses"])
	otherIncome := toDecimal(input["other_income"])
	exemptIncome := toDecimal(input["exempt_income"])

	grossIncome := revenue.Add(otherIncome)
	taxableIncome := grossIncome.Sub(expenses).Sub(exemptIncome)
	if taxableIncome.LessThan(decimal.Zero) {
		taxableIncome = decimal.Zero
	}

	// Standard rate 30%, but SME rate 14% on first LKR 10M if turnover ≤ 500M
	taxRate := lkforms.CorporateStd
	isSME := toString(input["sme_eligible"]) == "true"
	if isSME && revenue.LessThanOrEqual(decimal.NewFromInt(500_000_000)) {
		smeThreshold := decimal.NewFromInt(10_000_000)
		if taxableIncome.LessThanOrEqual(smeThreshold) {
			taxRate = lkforms.CorporateSME
		}
	}

	taxPayable := taxableIncome.Mul(taxRate)
	whtCredits := toDecimal(input["wht_credits"])
	escCredits := toDecimal(input["esc_credits"])
	netTax := taxPayable.Sub(whtCredits).Sub(escCredits)
	if netTax.LessThan(decimal.Zero) {
		netTax = decimal.Zero
	}

	return TaxResult{
		"revenue":         revenue.StringFixed(2),
		"expenses":        expenses.StringFixed(2),
		"gross_income":    grossIncome.StringFixed(2),
		"exempt_income":   exemptIncome.StringFixed(2),
		"taxable_income":  taxableIncome.StringFixed(2),
		"tax_rate":        taxRate.StringFixed(4),
		"tax_payable":     taxPayable.StringFixed(2),
		"wht_credits":     whtCredits.StringFixed(2),
		"net_tax_payable": netTax.StringFixed(2),
	}, nil
}

// CalculateLKPAYE computes PAYE / APIT return for employees.
func CalculateLKPAYE(input map[string]interface{}) (TaxResult, error) {
	grossSalary := toDecimal(input["gross_salary"])
	epfEmployee := grossSalary.Mul(lkforms.EPFEmployee)
	epfEmployer := grossSalary.Mul(lkforms.EPFEmployer)
	etfEmployer := grossSalary.Mul(lkforms.ETFRate)

	// APIT calculation using progressive brackets
	annualGross := grossSalary.Mul(decimal.NewFromInt(12))
	annualTax := calculateLKProgressiveTax(annualGross)
	monthlyTax := annualTax.Div(decimal.NewFromInt(12)).Round(2)

	totalDeductions := epfEmployee.Add(monthlyTax)
	netSalary := grossSalary.Sub(totalDeductions)
	employerCost := grossSalary.Add(epfEmployer).Add(etfEmployer)

	return TaxResult{
		"gross_salary":       grossSalary.StringFixed(2),
		"epf_employee":       epfEmployee.StringFixed(2),
		"epf_employer":       epfEmployer.StringFixed(2),
		"etf_employer":       etfEmployer.StringFixed(2),
		"apit_monthly":       monthlyTax.StringFixed(2),
		"total_deductions":   totalDeductions.StringFixed(2),
		"net_salary":         netSalary.StringFixed(2),
		"total_employer_cost": employerCost.StringFixed(2),
	}, nil
}

// CalculateLKWHT computes a withholding tax amount.
func CalculateLKWHT(input map[string]interface{}) (TaxResult, error) {
	paymentAmount := toDecimal(input["payment_amount"])
	incomeType := toString(input["income_type"])

	rate := decimal.Zero
	description := "Other"
	for _, w := range lkforms.WHTNatureOfIncome {
		if w.Code == incomeType {
			rate = w.Rate
			description = w.Description
			break
		}
	}

	// Allow manual rate override
	if r := toDecimal(input["wht_rate"]); !r.IsZero() {
		rate = r
	}

	taxWithheld := paymentAmount.Mul(rate)
	netPayment := paymentAmount.Sub(taxWithheld)

	return TaxResult{
		"payment_amount": paymentAmount.StringFixed(2),
		"income_type":    incomeType,
		"description":    description,
		"wht_rate":       rate.StringFixed(4),
		"tax_withheld":   taxWithheld.StringFixed(2),
		"net_payment":    netPayment.StringFixed(2),
	}, nil
}

func calculateLKProgressiveTax(annualIncome decimal.Decimal) decimal.Decimal {
	tax := decimal.Zero
	for _, b := range lkforms.LKIncomeTaxBrackets {
		if annualIncome.LessThanOrEqual(b.Over) {
			break
		}
		taxable := annualIncome.Sub(b.Over)
		if !b.NotOver.IsZero() && taxable.GreaterThan(b.NotOver.Sub(b.Over)) {
			taxable = b.NotOver.Sub(b.Over)
		}
		tax = tax.Add(taxable.Mul(b.Rate))
	}
	return tax
}
