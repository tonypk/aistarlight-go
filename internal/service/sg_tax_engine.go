package service

import (
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/pkg/irasforms"
)

// CalculateGSTF5 computes the Singapore GST Return (GST F5).
//
// GST F5 Boxes:
//
//	Box 1: Total value of standard-rated supplies
//	Box 2: Total value of zero-rated supplies
//	Box 3: Total value of exempt supplies
//	Box 4: Total value of supplies (Box 1 + 2 + 3)
//	Box 5: Total value of taxable purchases
//	Box 6: Output tax due (Box 1 × 9%)
//	Box 7: Input tax and refunds claimed
//	Box 8: Net GST payable / (refundable) (Box 6 - Box 7)
//	Box 9: Total bad debt relief
//	Box 10: Pre-registration input tax
//	Box 11: Tourist refund scheme
func CalculateGSTF5(input map[string]interface{}) (TaxResult, error) {
	// Accept pre-aggregated summary fields or raw data
	standardRatedSupplies := toDecimal(input["standard_rated_supplies"])
	zeroRatedSupplies := toDecimal(input["zero_rated_supplies"])
	exemptSupplies := toDecimal(input["exempt_supplies"])
	taxablePurchases := toDecimal(input["taxable_purchases"])
	inputTaxClaimed := toDecimal(input["input_tax_claimed"])
	badDebtRelief := toDecimal(input["bad_debt_relief"])
	preRegInputTax := toDecimal(input["pre_registration_input_tax"])
	touristRefund := toDecimal(input["tourist_refund"])

	// If raw sales_data provided, aggregate
	salesData := toMapSlice(input["sales_data"])
	if len(salesData) > 0 {
		for _, row := range salesData {
			amount := toDecimal(row["amount"])
			supplyType := toString(row["supply_type"])
			switch supplyType {
			case "standard", "standard_rated":
				standardRatedSupplies = standardRatedSupplies.Add(amount)
			case "zero", "zero_rated":
				zeroRatedSupplies = zeroRatedSupplies.Add(amount)
			case "exempt":
				exemptSupplies = exemptSupplies.Add(amount)
			default:
				standardRatedSupplies = standardRatedSupplies.Add(amount)
			}
		}
	}

	// If raw purchases_data provided, aggregate
	purchasesData := toMapSlice(input["purchases_data"])
	if len(purchasesData) > 0 {
		for _, row := range purchasesData {
			amount := toDecimal(row["amount"])
			gstAmount := toDecimal(row["gst_amount"])
			taxablePurchases = taxablePurchases.Add(amount)
			inputTaxClaimed = inputTaxClaimed.Add(gstAmount)
		}
	}

	// Box 4: Total supplies
	totalSupplies := standardRatedSupplies.Add(zeroRatedSupplies).Add(exemptSupplies)

	// Box 6: Output tax
	outputTax := standardRatedSupplies.Mul(irasforms.GSTRate)

	// Box 7: Input tax and refunds claimed (does NOT include bad debt relief or tourist refund)
	totalInputTax := inputTaxClaimed.Add(preRegInputTax)

	// Box 8: Net GST = Output tax - Input tax claimed
	netGST := outputTax.Sub(totalInputTax)

	return TaxResult{
		"box_1_standard_rated_supplies": standardRatedSupplies.StringFixed(2),
		"box_2_zero_rated_supplies":     zeroRatedSupplies.StringFixed(2),
		"box_3_exempt_supplies":         exemptSupplies.StringFixed(2),
		"box_4_total_supplies":          totalSupplies.StringFixed(2),
		"box_5_taxable_purchases":       taxablePurchases.StringFixed(2),
		"box_6_output_tax":              outputTax.StringFixed(2),
		"box_7_input_tax_claimed":       totalInputTax.StringFixed(2),
		"box_8_net_gst":                 netGST.StringFixed(2),
		"box_9_bad_debt_relief":         badDebtRelief.StringFixed(2),
		"box_10_pre_reg_input_tax":      preRegInputTax.StringFixed(2),
		"box_11_tourist_refund":         touristRefund.StringFixed(2),
		"gst_rate":                      irasforms.GSTRate.String(),
	}, nil
}

// CalculateFormC computes Singapore Corporate Income Tax (Form C).
//
// Full Form C for companies with revenue > S$5M or claiming incentives.
// Tax computation: Revenue - Expenses = Adjusted Profit → Partial Exemption → Tax @ 17%
func CalculateFormC(input map[string]interface{}) (TaxResult, error) {
	revenue := toDecimal(input["revenue"])
	costOfSales := toDecimal(input["cost_of_sales"])
	operatingExpenses := toDecimal(input["operating_expenses"])
	otherIncome := toDecimal(input["other_income"])
	nonDeductible := toDecimal(input["non_deductible_expenses"])
	capitalAllowances := toDecimal(input["capital_allowances"])
	donations := toDecimal(input["donations"])
	lossesCarriedForward := toDecimal(input["losses_carried_forward"])

	// Gross profit
	grossProfit := revenue.Sub(costOfSales)

	// Adjusted profit before tax
	adjustedProfit := grossProfit.Sub(operatingExpenses).Add(otherIncome).Add(nonDeductible).Sub(capitalAllowances)

	// Deductions
	donationDeduction := donations.Mul(decimal.NewFromFloat(2.5)) // 250% deduction for qualifying donations
	totalDeductions := donationDeduction.Add(lossesCarriedForward)

	// Chargeable income
	chargeableIncome := decMax(adjustedProfit.Sub(totalDeductions), decimal.Zero)

	// Partial tax exemption
	exemptAmount := computePartialExemption(chargeableIncome)
	taxableIncome := decMax(chargeableIncome.Sub(exemptAmount), decimal.Zero)

	// Tax @ 17%
	taxPayable := taxableIncome.Mul(irasforms.CorporateRate)

	return TaxResult{
		"revenue":              revenue.StringFixed(2),
		"cost_of_sales":        costOfSales.StringFixed(2),
		"gross_profit":         grossProfit.StringFixed(2),
		"operating_expenses":   operatingExpenses.StringFixed(2),
		"other_income":         otherIncome.StringFixed(2),
		"non_deductible":       nonDeductible.StringFixed(2),
		"capital_allowances":   capitalAllowances.StringFixed(2),
		"adjusted_profit":      adjustedProfit.StringFixed(2),
		"donation_deduction":   donationDeduction.StringFixed(2),
		"losses_utilized":      lossesCarriedForward.StringFixed(2),
		"chargeable_income":    chargeableIncome.StringFixed(2),
		"partial_exemption":    exemptAmount.StringFixed(2),
		"taxable_income":       taxableIncome.StringFixed(2),
		"corporate_tax_rate":   irasforms.CorporateRate.String(),
		"tax_payable":          taxPayable.StringFixed(2),
	}, nil
}

// CalculateFormCS computes Singapore Simplified Corporate Tax (Form C-S).
//
// For companies with annual revenue ≤ S$5M, single corporate rate only.
func CalculateFormCS(input map[string]interface{}) (TaxResult, error) {
	revenue := toDecimal(input["revenue"])
	expenses := toDecimal(input["total_expenses"])
	adjustments := toDecimal(input["tax_adjustments"])
	capitalAllowances := toDecimal(input["capital_allowances"])

	// Eligibility check: revenue must be ≤ S$5M
	if revenue.GreaterThan(irasforms.FormCSRevenueLimit) {
		return nil, fmt.Errorf("Form C-S not eligible: revenue S$%s exceeds S$5M limit, use Form C", revenue.String())
	}

	// Adjusted profit
	adjustedProfit := revenue.Sub(expenses).Add(adjustments).Sub(capitalAllowances)

	// Chargeable income
	chargeableIncome := decMax(adjustedProfit, decimal.Zero)

	// Partial exemption
	exemptAmount := computePartialExemption(chargeableIncome)
	taxableIncome := decMax(chargeableIncome.Sub(exemptAmount), decimal.Zero)

	// Tax @ 17%
	taxPayable := taxableIncome.Mul(irasforms.CorporateRate)

	return TaxResult{
		"revenue":            revenue.StringFixed(2),
		"total_expenses":     expenses.StringFixed(2),
		"tax_adjustments":    adjustments.StringFixed(2),
		"capital_allowances": capitalAllowances.StringFixed(2),
		"adjusted_profit":    adjustedProfit.StringFixed(2),
		"chargeable_income":  chargeableIncome.StringFixed(2),
		"partial_exemption":  exemptAmount.StringFixed(2),
		"taxable_income":     taxableIncome.StringFixed(2),
		"corporate_tax_rate": irasforms.CorporateRate.String(),
		"tax_payable":        taxPayable.StringFixed(2),
	}, nil
}

// CalculateFormB computes Singapore Individual Income Tax (Form B).
//
// Uses progressive tax brackets from 0% to 24%.
func CalculateFormB(input map[string]interface{}) (TaxResult, error) {
	employmentIncome := toDecimal(input["employment_income"])
	tradeIncome := toDecimal(input["trade_income"])
	rentalIncome := toDecimal(input["rental_income"])
	otherIncome := toDecimal(input["other_income"])
	reliefs := toDecimal(input["total_reliefs"])
	donations := toDecimal(input["donations"])

	// Total income
	totalIncome := employmentIncome.Add(tradeIncome).Add(rentalIncome).Add(otherIncome)

	// Deductions
	donationDeduction := donations.Mul(decimal.NewFromFloat(2.5)) // 250% for qualifying donations
	totalDeductions := reliefs.Add(donationDeduction)

	// Chargeable income
	chargeableIncome := decMax(totalIncome.Sub(totalDeductions), decimal.Zero)

	// Progressive tax
	taxPayable := computeSGProgressiveTax(chargeableIncome)

	return TaxResult{
		"employment_income":  employmentIncome.StringFixed(2),
		"trade_income":       tradeIncome.StringFixed(2),
		"rental_income":      rentalIncome.StringFixed(2),
		"other_income":       otherIncome.StringFixed(2),
		"total_income":       totalIncome.StringFixed(2),
		"total_reliefs":      reliefs.StringFixed(2),
		"donation_deduction": donationDeduction.StringFixed(2),
		"chargeable_income":  chargeableIncome.StringFixed(2),
		"tax_payable":        taxPayable.StringFixed(2),
	}, nil
}

// CalculateIR8A computes Singapore Employer Remuneration Return (IR8A).
//
// Reports employment income + CPF contributions for each employee.
func CalculateIR8A(input map[string]interface{}) (TaxResult, error) {
	grossSalary := toDecimal(input["gross_salary"])
	bonus := toDecimal(input["bonus"])
	directorFees := toDecimal(input["director_fees"])
	otherAllowances := toDecimal(input["other_allowances"])
	benefitsInKind := toDecimal(input["benefits_in_kind"])
	employerCPF := toDecimal(input["employer_cpf"])
	employeeCPF := toDecimal(input["employee_cpf"])

	// Total gross remuneration
	totalGross := grossSalary.Add(bonus).Add(directorFees).Add(otherAllowances).Add(benefitsInKind)

	// Auto-compute CPF if not provided (cap at OW ceiling S$6,800/month)
	monthlyOW := grossSalary.Div(decimal.NewFromInt(12))
	if monthlyOW.GreaterThan(irasforms.CPFOWCeiling) {
		monthlyOW = irasforms.CPFOWCeiling
	}
	annualCappedOW := monthlyOW.Mul(decimal.NewFromInt(12))

	if employerCPF.IsZero() && !grossSalary.IsZero() {
		employerCPF = annualCappedOW.Mul(irasforms.CPFEmployer)
	}
	if employeeCPF.IsZero() && !grossSalary.IsZero() {
		employeeCPF = annualCappedOW.Mul(irasforms.CPFEmployee)
	}

	// Net salary (after employee CPF)
	netSalary := totalGross.Sub(employeeCPF)

	return TaxResult{
		"gross_salary":      grossSalary.StringFixed(2),
		"bonus":             bonus.StringFixed(2),
		"director_fees":     directorFees.StringFixed(2),
		"other_allowances":  otherAllowances.StringFixed(2),
		"benefits_in_kind":  benefitsInKind.StringFixed(2),
		"total_gross":       totalGross.StringFixed(2),
		"employer_cpf":      employerCPF.StringFixed(2),
		"employee_cpf":      employeeCPF.StringFixed(2),
		"net_salary":        netSalary.StringFixed(2),
		"cpf_employer_rate": irasforms.CPFEmployer.String(),
		"cpf_employee_rate": irasforms.CPFEmployee.String(),
	}, nil
}

// CalculateS45 computes Singapore Withholding Tax (S45) for non-resident payments.
//
// When paying non-residents for services/royalties/interest, the payer must
// withhold tax at the applicable rate and remit to IRAS.
func CalculateS45(input map[string]interface{}) (TaxResult, error) {
	paymentAmount := toDecimal(input["payment_amount"])
	incomeType := toString(input["income_type"])
	customRate := toDecimal(input["custom_rate"]) // For treaty rates

	if paymentAmount.IsZero() {
		return nil, fmt.Errorf("payment_amount is required for S45 calculation")
	}

	// Determine WHT rate
	var whtRate decimal.Decimal
	var description string

	if !customRate.IsZero() {
		// Treaty rate override
		whtRate = customRate
		description = fmt.Sprintf("Treaty rate for %s", incomeType)
	} else if nature, ok := irasforms.WHTNatureOfIncome[incomeType]; ok {
		whtRate = nature.Rate
		description = nature.Description
	} else {
		return nil, fmt.Errorf("unknown income type %q; valid: INT, ROY, TECH, MGMT, DIR, RENT, SFC", incomeType)
	}

	// Calculate withholding
	taxWithheld := paymentAmount.Mul(whtRate)
	netPayment := paymentAmount.Sub(taxWithheld)

	return TaxResult{
		"payment_amount": paymentAmount.StringFixed(2),
		"income_type":    incomeType,
		"description":    description,
		"wht_rate":       whtRate.String(),
		"tax_withheld":   taxWithheld.StringFixed(2),
		"net_payment":    netPayment.StringFixed(2),
	}, nil
}

// computeSGProgressiveTax calculates tax using Singapore progressive brackets.
func computeSGProgressiveTax(chargeableIncome decimal.Decimal) decimal.Decimal {
	if chargeableIncome.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}

	for _, b := range irasforms.SGTaxBrackets {
		notOver := b.NotOver
		// NotOver = 0 means no upper limit (last bracket)
		if notOver.IsZero() || chargeableIncome.LessThanOrEqual(notOver) {
			excess := chargeableIncome.Sub(b.Over)
			return b.BaseTax.Add(excess.Mul(b.Rate)).Round(2)
		}
	}

	// Fallback: use last bracket
	last := irasforms.SGTaxBrackets[len(irasforms.SGTaxBrackets)-1]
	excess := chargeableIncome.Sub(last.Over)
	return last.BaseTax.Add(excess.Mul(last.Rate)).Round(2)
}

// computePartialExemption calculates the IRAS partial tax exemption.
//
// Partial exemption scheme:
//   - 75% exemption on first S$10,000 of chargeable income
//   - 50% exemption on next S$190,000 of chargeable income
func computePartialExemption(chargeableIncome decimal.Decimal) decimal.Decimal {
	if chargeableIncome.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}

	exemption := decimal.Zero

	// First S$10,000 @ 75%
	firstTier := decimal.Min(chargeableIncome, irasforms.PartialExemptionFirst)
	exemption = exemption.Add(firstTier.Mul(decimal.NewFromFloat(0.75)))

	// Next S$190,000 @ 50%
	remaining := chargeableIncome.Sub(irasforms.PartialExemptionFirst)
	if remaining.GreaterThan(decimal.Zero) {
		secondTier := decimal.Min(remaining, irasforms.PartialExemptionNext)
		exemption = exemption.Add(secondTier.Mul(decimal.NewFromFloat(0.50)))
	}

	return exemption
}

// CalculateECI computes the Estimated Chargeable Income (ECI).
// ECI is a simplified estimate filed within 3 months of FY-end.
func CalculateECI(input map[string]interface{}) (TaxResult, error) {
	revenue := toDecimal(input["revenue"])
	adjustedProfit := toDecimal(input["adjusted_profit"])
	capitalAllowances := toDecimal(input["capital_allowances"])

	estimatedCI := adjustedProfit.Sub(capitalAllowances)
	if estimatedCI.LessThan(decimal.Zero) {
		estimatedCI = decimal.Zero
	}

	// Apply partial exemption
	exemption := computePartialExemption(estimatedCI)
	taxableAfterExemption := estimatedCI.Sub(exemption)
	if taxableAfterExemption.LessThan(decimal.Zero) {
		taxableAfterExemption = decimal.Zero
	}

	estimatedTax := taxableAfterExemption.Mul(irasforms.CorporateRate)

	return TaxResult{
		"revenue":                      revenue.StringFixed(2),
		"adjusted_profit":              adjustedProfit.StringFixed(2),
		"capital_allowances":           capitalAllowances.StringFixed(2),
		"estimated_chargeable_income":  estimatedCI.StringFixed(2),
		"partial_tax_exemption":        exemption.StringFixed(2),
		"taxable_after_exemption":      taxableAfterExemption.StringFixed(2),
		"estimated_tax":                estimatedTax.StringFixed(2),
	}, nil
}
