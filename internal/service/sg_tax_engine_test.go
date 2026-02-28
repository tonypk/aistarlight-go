package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tonypk/aistarlight-go/pkg/irasforms"
)

// ============================================================
// GST F5 — Quarterly GST Return
// ============================================================

func TestCalculateGSTF5_StandardRetailBusiness(t *testing.T) {
	input := map[string]interface{}{
		"standard_rated_supplies": "500000",
		"zero_rated_supplies":     "20000",
		"exempt_supplies":         "0",
		"taxable_purchases":       "200000",
		"input_tax_claimed":       "18000",
	}
	result, err := CalculateGSTF5(input)
	require.NoError(t, err)

	assert.Equal(t, "500000.00", result["box_1_standard_rated_supplies"])
	assert.Equal(t, "20000.00", result["box_2_zero_rated_supplies"])
	assert.Equal(t, "0.00", result["box_3_exempt_supplies"])
	assert.Equal(t, "520000.00", result["box_4_total_supplies"])
	assert.Equal(t, "200000.00", result["box_5_taxable_purchases"])
	assert.Equal(t, "45000.00", result["box_6_output_tax"])        // 500k * 9%
	assert.Equal(t, "18000.00", result["box_7_input_tax_claimed"]) // input only
	assert.Equal(t, "27000.00", result["box_8_net_gst"])           // 45k - 18k
}

func TestCalculateGSTF5_PureExporter_RefundScenario(t *testing.T) {
	// 100% zero-rated exports -> output tax = 0, input tax claimed -> refund
	input := map[string]interface{}{
		"standard_rated_supplies": "0",
		"zero_rated_supplies":     "1000000",
		"exempt_supplies":         "0",
		"taxable_purchases":       "400000",
		"input_tax_claimed":       "36000",
	}
	result, err := CalculateGSTF5(input)
	require.NoError(t, err)

	assert.Equal(t, "0.00", result["box_6_output_tax"])
	assert.Equal(t, "36000.00", result["box_7_input_tax_claimed"])
	assert.Equal(t, "-36000.00", result["box_8_net_gst"]) // Refundable
}

func TestCalculateGSTF5_MixedSupplies(t *testing.T) {
	// Financial services: standard + exempt
	input := map[string]interface{}{
		"standard_rated_supplies": "300000",
		"zero_rated_supplies":     "0",
		"exempt_supplies":         "200000",
		"taxable_purchases":       "100000",
		"input_tax_claimed":       "5400", // Apportioned: only 60% claimable
	}
	result, err := CalculateGSTF5(input)
	require.NoError(t, err)

	assert.Equal(t, "500000.00", result["box_4_total_supplies"])
	assert.Equal(t, "27000.00", result["box_6_output_tax"]) // 300k * 9%
	assert.Equal(t, "5400.00", result["box_7_input_tax_claimed"])
	assert.Equal(t, "21600.00", result["box_8_net_gst"]) // 27k - 5.4k
}

func TestCalculateGSTF5_ZeroFiling(t *testing.T) {
	input := map[string]interface{}{
		"standard_rated_supplies": "0",
		"zero_rated_supplies":     "0",
		"exempt_supplies":         "0",
		"taxable_purchases":       "0",
		"input_tax_claimed":       "0",
	}
	result, err := CalculateGSTF5(input)
	require.NoError(t, err)

	assert.Equal(t, "0.00", result["box_4_total_supplies"])
	assert.Equal(t, "0.00", result["box_6_output_tax"])
	assert.Equal(t, "0.00", result["box_8_net_gst"])
}

func TestCalculateGSTF5_BadDebtRelief_NotInBox7(t *testing.T) {
	// Bad debt relief should appear in Box 9, NOT in Box 7
	input := map[string]interface{}{
		"standard_rated_supplies": "100000",
		"input_tax_claimed":       "5000",
		"bad_debt_relief":         "2000",
		"tourist_refund":          "500",
	}
	result, err := CalculateGSTF5(input)
	require.NoError(t, err)

	// Box 7 should only have input_tax_claimed, NOT bad_debt or tourist
	assert.Equal(t, "5000.00", result["box_7_input_tax_claimed"])
	assert.Equal(t, "2000.00", result["box_9_bad_debt_relief"])
	assert.Equal(t, "500.00", result["box_11_tourist_refund"])
	// Box 8 = 9000 (output) - 5000 (input) = 4000
	assert.Equal(t, "4000.00", result["box_8_net_gst"])
}

func TestCalculateGSTF5_RawSalesData(t *testing.T) {
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "100000", "supply_type": "standard"},
			map[string]interface{}{"amount": "50000", "supply_type": "standard_rated"},
			map[string]interface{}{"amount": "30000", "supply_type": "zero_rated"},
			map[string]interface{}{"amount": "20000", "supply_type": "exempt"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "50000", "gst_amount": "4500"},
		},
	}
	result, err := CalculateGSTF5(input)
	require.NoError(t, err)

	assert.Equal(t, "150000.00", result["box_1_standard_rated_supplies"]) // 100k + 50k
	assert.Equal(t, "30000.00", result["box_2_zero_rated_supplies"])
	assert.Equal(t, "20000.00", result["box_3_exempt_supplies"])
	assert.Equal(t, "200000.00", result["box_4_total_supplies"])
	assert.Equal(t, "13500.00", result["box_6_output_tax"]) // 150k * 9%
	assert.Equal(t, "4500.00", result["box_7_input_tax_claimed"])
}

func TestCalculateGSTF5_PreRegistrationInputTax(t *testing.T) {
	// Pre-registration input tax IS included in Box 7 (per IRAS rules)
	input := map[string]interface{}{
		"standard_rated_supplies":   "100000",
		"input_tax_claimed":         "5000",
		"pre_registration_input_tax": "1000",
	}
	result, err := CalculateGSTF5(input)
	require.NoError(t, err)

	// Box 7 = input_tax_claimed + pre_registration = 5000 + 1000 = 6000
	assert.Equal(t, "6000.00", result["box_7_input_tax_claimed"])
	assert.Equal(t, "1000.00", result["box_10_pre_reg_input_tax"])
}

func TestCalculateGSTF5_GSTRateIs9Percent(t *testing.T) {
	result, err := CalculateGSTF5(map[string]interface{}{
		"standard_rated_supplies": "100",
	})
	require.NoError(t, err)
	assert.Equal(t, "0.09", result["gst_rate"])
	assert.Equal(t, "9.00", result["box_6_output_tax"]) // 100 * 9% = 9
}

// ============================================================
// Form C — Corporate Income Tax
// ============================================================

func TestCalculateFormC_ProfitableSME(t *testing.T) {
	// S$300k CI: partial exemption = 7.5k + 95k = 102.5k
	// Taxable: 300k - 102.5k = 197.5k, Tax: 197.5k * 17% = 33575
	input := map[string]interface{}{
		"revenue":            "1000000",
		"cost_of_sales":      "500000",
		"operating_expenses": "200000",
	}
	result, err := CalculateFormC(input)
	require.NoError(t, err)

	assert.Equal(t, "500000.00", result["gross_profit"])
	assert.Equal(t, "300000.00", result["chargeable_income"])
	assert.Equal(t, "102500.00", result["partial_exemption"])
	assert.Equal(t, "197500.00", result["taxable_income"])
	assert.Equal(t, "33575.00", result["tax_payable"])
}

func TestCalculateFormC_LargeCorporation(t *testing.T) {
	// S$5M CI: partial exemption maxes at 102.5k
	input := map[string]interface{}{
		"revenue":            "20000000",
		"cost_of_sales":      "10000000",
		"operating_expenses": "5000000",
	}
	result, err := CalculateFormC(input)
	require.NoError(t, err)

	assert.Equal(t, "5000000.00", result["chargeable_income"])
	assert.Equal(t, "102500.00", result["partial_exemption"])
	assert.Equal(t, "4897500.00", result["taxable_income"])
	assert.Equal(t, "832575.00", result["tax_payable"]) // 4897500 * 0.17
}

func TestCalculateFormC_LossMaking(t *testing.T) {
	input := map[string]interface{}{
		"revenue":            "500000",
		"cost_of_sales":      "400000",
		"operating_expenses": "200000",
	}
	result, err := CalculateFormC(input)
	require.NoError(t, err)

	assert.Equal(t, "0.00", result["chargeable_income"])
	assert.Equal(t, "0.00", result["tax_payable"])
}

func TestCalculateFormC_PartialExemption_Exactly10k(t *testing.T) {
	// CI = S$10k -> exempt: 75% of 10k = 7.5k, taxable: 2.5k, tax: 425
	input := map[string]interface{}{
		"revenue":       "110000",
		"cost_of_sales": "100000",
	}
	result, err := CalculateFormC(input)
	require.NoError(t, err)

	assert.Equal(t, "10000.00", result["chargeable_income"])
	assert.Equal(t, "7500.00", result["partial_exemption"])
	assert.Equal(t, "2500.00", result["taxable_income"])
	assert.Equal(t, "425.00", result["tax_payable"])
}

func TestCalculateFormC_PartialExemption_200k(t *testing.T) {
	// CI = S$200k -> full partial exemption: 7.5k + 95k = 102.5k
	// Taxable: 97.5k, Tax: 16575
	input := map[string]interface{}{
		"revenue":       "300000",
		"cost_of_sales": "100000",
	}
	result, err := CalculateFormC(input)
	require.NoError(t, err)

	assert.Equal(t, "200000.00", result["chargeable_income"])
	assert.Equal(t, "102500.00", result["partial_exemption"])
	assert.Equal(t, "97500.00", result["taxable_income"])
	assert.Equal(t, "16575.00", result["tax_payable"])
}

func TestCalculateFormC_PartialExemption_Above200k(t *testing.T) {
	// CI = S$500k -> exemption still capped at 102.5k
	input := map[string]interface{}{
		"revenue":       "700000",
		"cost_of_sales": "200000",
	}
	result, err := CalculateFormC(input)
	require.NoError(t, err)

	assert.Equal(t, "500000.00", result["chargeable_income"])
	assert.Equal(t, "102500.00", result["partial_exemption"]) // Max exemption
	assert.Equal(t, "397500.00", result["taxable_income"])
	assert.Equal(t, "67575.00", result["tax_payable"]) // 397500 * 0.17
}

func TestCalculateFormC_DonationsDeduction(t *testing.T) {
	// S$10k donation -> S$25k deduction (250%)
	input := map[string]interface{}{
		"revenue":       "500000",
		"cost_of_sales": "200000",
		"donations":     "10000",
	}
	result, err := CalculateFormC(input)
	require.NoError(t, err)

	assert.Equal(t, "25000.00", result["donation_deduction"])
	assert.Equal(t, "275000.00", result["chargeable_income"]) // 300k - 25k
}

func TestCalculateFormC_LossesCarriedForward(t *testing.T) {
	input := map[string]interface{}{
		"revenue":                "500000",
		"cost_of_sales":          "200000",
		"losses_carried_forward": "250000",
	}
	result, err := CalculateFormC(input)
	require.NoError(t, err)

	assert.Equal(t, "50000.00", result["chargeable_income"]) // 300k - 250k
}

func TestCalculateFormC_CorporateRate17Percent(t *testing.T) {
	assert.True(t, irasforms.CorporateRate.Equal(decimal.NewFromFloat(0.17)),
		"Corporate rate should be 17%%")
}

// ============================================================
// Form C-S — Simplified Corporate Tax
// ============================================================

func TestCalculateFormCS_Eligible(t *testing.T) {
	input := map[string]interface{}{
		"revenue":            "3000000",
		"total_expenses":     "2500000",
		"capital_allowances": "50000",
	}
	result, err := CalculateFormCS(input)
	require.NoError(t, err)

	assert.Equal(t, "450000.00", result["chargeable_income"])
	assert.NotEmpty(t, result["tax_payable"])
}

func TestCalculateFormCS_ExceedsRevenueLimit(t *testing.T) {
	input := map[string]interface{}{
		"revenue":        "5000001",
		"total_expenses": "4000000",
	}
	_, err := CalculateFormCS(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not eligible")
}

func TestCalculateFormCS_ExactlyAtLimit(t *testing.T) {
	input := map[string]interface{}{
		"revenue":        "5000000",
		"total_expenses": "4000000",
	}
	_, err := CalculateFormCS(input)
	require.NoError(t, err)
}

func TestCalculateFormCS_SamePartialExemptionAsFormC(t *testing.T) {
	input := map[string]interface{}{
		"revenue":        "1000000",
		"total_expenses": "700000",
	}
	result, err := CalculateFormCS(input)
	require.NoError(t, err)

	// CI = 300k, same partial exemption as Form C
	assert.Equal(t, "300000.00", result["chargeable_income"])
	assert.Equal(t, "102500.00", result["partial_exemption"])
	assert.Equal(t, "197500.00", result["taxable_income"])
	assert.Equal(t, "33575.00", result["tax_payable"])
}

// ============================================================
// Form B — Individual Income Tax (Progressive Brackets)
// All values verified against IRAS Tax Calculator (YA 2024)
// ============================================================

func TestCalculateFormB_ProgressiveBrackets(t *testing.T) {
	tests := []struct {
		name        string
		income      string
		expectedTax string
	}{
		{"zero_income", "0", "0.00"},
		{"below_threshold_15k", "15000", "0.00"},
		{"exactly_20k", "20000", "0.00"},
		{"25k_2pct", "25000", "100.00"},               // (25k-20k)*2%
		{"30k_boundary", "30000", "200.00"},            // (30k-20k)*2%
		{"35k_35pct", "35000", "375.00"},               // 200 + (35k-30k)*3.5%
		{"40k_boundary", "40000", "550.00"},             // 200 + (40k-30k)*3.5%
		{"50k_7pct", "50000", "1250.00"},                // 550 + (50k-40k)*7%
		{"60k_7pct", "60000", "1950.00"},                // 550 + (60k-40k)*7%
		{"80k_boundary", "80000", "3350.00"},            // 550 + (80k-40k)*7%
		{"100k_115pct", "100000", "5650.00"},            // 3350 + (100k-80k)*11.5%
		{"120k_boundary", "120000", "7950.00"},          // 3350 + (120k-80k)*11.5%
		{"140k_15pct", "140000", "10950.00"},            // 7950 + (140k-120k)*15%
		{"160k_boundary", "160000", "13950.00"},         // 7950 + (160k-120k)*15%
		{"180k_18pct", "180000", "17550.00"},            // 13950 + (180k-160k)*18%
		{"200k_boundary", "200000", "21150.00"},         // 13950 + (200k-160k)*18%
		{"220k_19pct", "220000", "24950.00"},            // 21150 + (220k-200k)*19%
		{"240k_boundary", "240000", "28750.00"},         // 21150 + (240k-200k)*19%
		{"260k_195pct", "260000", "32650.00"},           // 28750 + (260k-240k)*19.5%
		{"280k_boundary", "280000", "36550.00"},         // 28750 + (280k-240k)*19.5%
		{"300k_20pct", "300000", "40550.00"},            // 36550 + (300k-280k)*20%
		{"320k_boundary", "320000", "44550.00"},         // 36550 + (320k-280k)*20%
		{"400k_22pct", "400000", "62150.00"},            // 44550 + (400k-320k)*22%
		{"500k_boundary", "500000", "84150.00"},         // 44550 + (500k-320k)*22%
		{"750k_23pct", "750000", "141650.00"},           // 84150 + (750k-500k)*23%
		{"1M_boundary", "1000000", "199150.00"},         // 84150 + (1M-500k)*23%
		{"1.5M_24pct", "1500000", "319150.00"},          // 199150 + (1.5M-1M)*24%
		{"2M_24pct", "2000000", "439150.00"},            // 199150 + (2M-1M)*24%
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]interface{}{
				"employment_income": tt.income,
			}
			result, err := CalculateFormB(input)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedTax, result["tax_payable"],
				"Failed for %s", tt.name)
		})
	}
}

func TestCalculateFormB_WithReliefs(t *testing.T) {
	// S$100k income - S$30k reliefs = S$70k chargeable
	// Tax: 550 + (70k-40k)*7% = 550 + 2100 = 2650
	input := map[string]interface{}{
		"employment_income": "100000",
		"total_reliefs":     "30000",
	}
	result, err := CalculateFormB(input)
	require.NoError(t, err)

	assert.Equal(t, "70000.00", result["chargeable_income"])
	assert.Equal(t, "2650.00", result["tax_payable"])
}

func TestCalculateFormB_ReliefsReduceToBelowThreshold(t *testing.T) {
	// S$50k - S$35k = S$15k chargeable -> zero tax
	input := map[string]interface{}{
		"employment_income": "50000",
		"total_reliefs":     "35000",
	}
	result, err := CalculateFormB(input)
	require.NoError(t, err)

	assert.Equal(t, "15000.00", result["chargeable_income"])
	assert.Equal(t, "0.00", result["tax_payable"])
}

func TestCalculateFormB_MultipleIncomeTypes(t *testing.T) {
	input := map[string]interface{}{
		"employment_income": "120000",
		"trade_income":      "30000",
		"rental_income":     "24000",
		"other_income":      "6000",
	}
	result, err := CalculateFormB(input)
	require.NoError(t, err)

	assert.Equal(t, "180000.00", result["total_income"])
	assert.Equal(t, "180000.00", result["chargeable_income"])
	// Tax: 13950 + (180k-160k)*18% = 13950 + 3600 = 17550
	assert.Equal(t, "17550.00", result["tax_payable"])
}

func TestCalculateFormB_DonationDeduction(t *testing.T) {
	input := map[string]interface{}{
		"employment_income": "100000",
		"donations":         "5000",
	}
	result, err := CalculateFormB(input)
	require.NoError(t, err)

	assert.Equal(t, "12500.00", result["donation_deduction"]) // 5k * 250%
	assert.Equal(t, "87500.00", result["chargeable_income"])  // 100k - 12.5k
}

// ============================================================
// IR8A — Employer Remuneration
// ============================================================

func TestCalculateIR8A_BelowCPFCeiling(t *testing.T) {
	// S$48k/yr = S$4k/mo, below S$6,800 ceiling
	input := map[string]interface{}{
		"gross_salary": "48000",
	}
	result, err := CalculateIR8A(input)
	require.NoError(t, err)

	assert.Equal(t, "8160.00", result["employer_cpf"])  // 48000 * 17%
	assert.Equal(t, "9600.00", result["employee_cpf"])  // 48000 * 20%
	assert.Equal(t, "38400.00", result["net_salary"])    // 48000 - 9600
}

func TestCalculateIR8A_AboveCPFCeiling(t *testing.T) {
	// S$120k/yr = S$10k/mo, above S$6,800 ceiling
	// CPF on capped OW: 6800 * 12 = 81600
	input := map[string]interface{}{
		"gross_salary": "120000",
	}
	result, err := CalculateIR8A(input)
	require.NoError(t, err)

	assert.Equal(t, "13872.00", result["employer_cpf"])  // 81600 * 17%
	assert.Equal(t, "16320.00", result["employee_cpf"])  // 81600 * 20%
	assert.Equal(t, "103680.00", result["net_salary"])    // 120000 - 16320
}

func TestCalculateIR8A_ExactlyAtCeiling(t *testing.T) {
	// S$81,600/yr = S$6,800/mo exactly
	input := map[string]interface{}{
		"gross_salary": "81600",
	}
	result, err := CalculateIR8A(input)
	require.NoError(t, err)

	assert.Equal(t, "13872.00", result["employer_cpf"])
	assert.Equal(t, "16320.00", result["employee_cpf"])
}

func TestCalculateIR8A_WithBonusAndDirectorFees(t *testing.T) {
	input := map[string]interface{}{
		"gross_salary":     "60000",
		"bonus":            "12000",
		"director_fees":    "24000",
		"other_allowances": "6000",
		"benefits_in_kind": "3000",
	}
	result, err := CalculateIR8A(input)
	require.NoError(t, err)

	assert.Equal(t, "105000.00", result["total_gross"]) // 60k+12k+24k+6k+3k
}

func TestCalculateIR8A_ManualCPFOverride(t *testing.T) {
	input := map[string]interface{}{
		"gross_salary": "60000",
		"employer_cpf": "10000",
		"employee_cpf": "12000",
	}
	result, err := CalculateIR8A(input)
	require.NoError(t, err)

	assert.Equal(t, "10000.00", result["employer_cpf"])
	assert.Equal(t, "12000.00", result["employee_cpf"])
	assert.Equal(t, "48000.00", result["net_salary"]) // 60000 - 12000
}

func TestCalculateIR8A_CPFRates(t *testing.T) {
	assert.True(t, irasforms.CPFEmployer.Equal(decimal.NewFromFloat(0.17)),
		"CPF employer should be 17%%")
	assert.True(t, irasforms.CPFEmployee.Equal(decimal.NewFromFloat(0.20)),
		"CPF employee should be 20%%")
	assert.True(t, irasforms.CPFOWCeiling.Equal(decimal.NewFromInt(6800)),
		"CPF OW ceiling should be S$6,800")
}

// ============================================================
// S45 — Withholding Tax (Non-Resident Payments)
// ============================================================

func TestCalculateS45_AllIncomeTypes(t *testing.T) {
	tests := []struct {
		incomeType   string
		expectedRate string
		expectedTax  string
	}{
		{"INT", "0.15", "15000.00"},  // Interest: 15%
		{"ROY", "0.1", "10000.00"},   // Royalties: 10%
		{"TECH", "0.17", "17000.00"}, // Technical fees: 17%
		{"MGMT", "0.17", "17000.00"}, // Management fees: 17%
		{"DIR", "0.22", "22000.00"},  // Director fees: 22%
		{"RENT", "0.15", "15000.00"}, // Rent moveable: 15%
		{"SFC", "0.22", "22000.00"},  // SRS: 22%
	}

	for _, tt := range tests {
		t.Run(tt.incomeType, func(t *testing.T) {
			input := map[string]interface{}{
				"payment_amount": "100000",
				"income_type":    tt.incomeType,
			}
			result, err := CalculateS45(input)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedRate, result["wht_rate"],
				"%s rate mismatch", tt.incomeType)
			assert.Equal(t, tt.expectedTax, result["tax_withheld"],
				"%s tax mismatch", tt.incomeType)
			// Net payment = 100k - tax
			netExpected := decimal.NewFromInt(100000).Sub(
				decimal.RequireFromString(tt.expectedTax)).StringFixed(2)
			assert.Equal(t, netExpected, result["net_payment"])
		})
	}
}

func TestCalculateS45_TreatyRateOverride(t *testing.T) {
	// DTA with India: technical fees reduced to 10%
	input := map[string]interface{}{
		"payment_amount": "100000",
		"income_type":    "TECH",
		"custom_rate":    "0.10",
	}
	result, err := CalculateS45(input)
	require.NoError(t, err)

	assert.Equal(t, "0.1", result["wht_rate"])
	assert.Equal(t, "10000.00", result["tax_withheld"])
	assert.Equal(t, "90000.00", result["net_payment"])
}

func TestCalculateS45_InvalidIncomeType(t *testing.T) {
	input := map[string]interface{}{
		"payment_amount": "100000",
		"income_type":    "INVALID",
	}
	_, err := CalculateS45(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown income type")
}

func TestCalculateS45_ZeroAmount(t *testing.T) {
	input := map[string]interface{}{
		"payment_amount": "0",
		"income_type":    "INT",
	}
	_, err := CalculateS45(input)
	require.Error(t, err)
}

func TestCalculateS45_LargePayment(t *testing.T) {
	input := map[string]interface{}{
		"payment_amount": "5000000",
		"income_type":    "ROY",
	}
	result, err := CalculateS45(input)
	require.NoError(t, err)

	assert.Equal(t, "500000.00", result["tax_withheld"]) // 5M * 10%
	assert.Equal(t, "4500000.00", result["net_payment"])
}

// ============================================================
// ECI — Estimated Chargeable Income
// ============================================================

func TestCalculateECI_StandardFiling(t *testing.T) {
	input := map[string]interface{}{
		"revenue":            "2000000",
		"adjusted_profit":    "500000",
		"capital_allowances": "50000",
	}
	result, err := CalculateECI(input)
	require.NoError(t, err)

	assert.Equal(t, "450000.00", result["estimated_chargeable_income"])
	assert.Equal(t, "102500.00", result["partial_tax_exemption"])
	assert.Equal(t, "347500.00", result["taxable_after_exemption"])
	assert.Equal(t, "59075.00", result["estimated_tax"]) // 347500 * 0.17
}

func TestCalculateECI_SmallIncome(t *testing.T) {
	// CI below partial exemption threshold
	input := map[string]interface{}{
		"revenue":         "100000",
		"adjusted_profit": "5000",
	}
	result, err := CalculateECI(input)
	require.NoError(t, err)

	// CI = 5000, exempt: 75% of 5k = 3750, taxable: 1250, tax: 212.50
	assert.Equal(t, "5000.00", result["estimated_chargeable_income"])
	assert.Equal(t, "3750.00", result["partial_tax_exemption"])
	assert.Equal(t, "1250.00", result["taxable_after_exemption"])
	assert.Equal(t, "212.50", result["estimated_tax"])
}

func TestCalculateECI_NegativeProfitClamped(t *testing.T) {
	input := map[string]interface{}{
		"revenue":            "100000",
		"adjusted_profit":    "20000",
		"capital_allowances": "50000",
	}
	result, err := CalculateECI(input)
	require.NoError(t, err)

	assert.Equal(t, "0.00", result["estimated_chargeable_income"])
	assert.Equal(t, "0.00", result["estimated_tax"])
}

// ============================================================
// WHT Rate Consistency
// ============================================================

func TestSGWHTRates_ConsistentWithConstants(t *testing.T) {
	// Ensure sg_wht_rates.go local map matches irasforms.WHTNatureOfIncome
	for code, localRate := range SGWHTRates {
		irasRate, ok := irasforms.WHTNatureOfIncome[code]
		if !ok {
			continue // MGMT is separate in local map
		}
		assert.True(t, localRate.Rate.Equal(irasRate.Rate),
			"Rate mismatch for %s: local=%s, irasforms=%s",
			code, localRate.Rate, irasRate.Rate)
	}
}

func TestSGWHTRates_TECHIs17Percent(t *testing.T) {
	// CRITICAL: IRAS Section 45 - technical fees at prevailing corporate rate
	rate, ok := irasforms.WHTNatureOfIncome["TECH"]
	require.True(t, ok)
	assert.True(t, rate.Rate.Equal(decimal.NewFromFloat(0.17)),
		"TECH rate should be 17%% (prevailing corporate rate), got %s", rate.Rate)
}

func TestSGWHTRates_MGMTIs17Percent(t *testing.T) {
	rate, ok := irasforms.WHTNatureOfIncome["MGMT"]
	require.True(t, ok)
	assert.True(t, rate.Rate.Equal(decimal.NewFromFloat(0.17)),
		"MGMT rate should be 17%% (prevailing corporate rate), got %s", rate.Rate)
}

// ============================================================
// Partial Exemption Scheme
// ============================================================

func TestComputePartialExemption(t *testing.T) {
	tests := []struct {
		name     string
		ci       string
		expected string
	}{
		{"zero", "0", "0"},
		{"below_10k", "5000", "3750"},           // 5k * 75% = 3750
		{"exactly_10k", "10000", "7500"},         // 10k * 75% = 7500
		{"50k", "50000", "27500"},                // 7500 + (40k * 50%) = 27500
		{"200k_full", "200000", "102500"},         // 7500 + (190k * 50%) = 102500
		{"500k_capped", "500000", "102500"},       // Max exemption
		{"1M_capped", "1000000", "102500"},        // Max exemption
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ci := decimal.RequireFromString(tt.ci)
			expected := decimal.RequireFromString(tt.expected)
			result := computePartialExemption(ci)
			assert.True(t, expected.Equal(result),
				"expected %s, got %s", expected, result)
		})
	}
}

// ============================================================
// SG Progressive Tax
// ============================================================

func TestComputeSGProgressiveTax(t *testing.T) {
	tests := []struct {
		name     string
		income   string
		expected string
	}{
		{"zero", "0", "0"},
		{"10k", "10000", "0"},                    // Below S$20k
		{"20k", "20000", "0"},                    // At threshold
		{"30k", "30000", "200.00"},               // (30k-20k)*2%
		{"40k", "40000", "550.00"},               // 200 + (40k-30k)*3.5%
		{"80k", "80000", "3350.00"},              // 550 + (80k-40k)*7%
		{"120k", "120000", "7950.00"},            // 3350 + (120k-80k)*11.5%
		{"160k", "160000", "13950.00"},           // 7950 + (160k-120k)*15%
		{"200k", "200000", "21150.00"},           // 13950 + (200k-160k)*18%
		{"240k", "240000", "28750.00"},           // 21150 + (240k-200k)*19%
		{"280k", "280000", "36550.00"},           // 28750 + (280k-240k)*19.5%
		{"320k", "320000", "44550.00"},           // 36550 + (320k-280k)*20%
		{"500k", "500000", "84150.00"},           // 44550 + (500k-320k)*22%
		{"1M", "1000000", "199150.00"},           // 84150 + (1M-500k)*23%
		{"2M", "2000000", "439150.00"},           // 199150 + (2M-1M)*24%
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			income := decimal.RequireFromString(tt.income)
			expected := decimal.RequireFromString(tt.expected)
			result := computeSGProgressiveTax(income)
			assert.True(t, expected.Equal(result),
				"expected %s, got %s", expected, result)
		})
	}
}

// ============================================================
// Dispatcher — All SG Forms Route Correctly
// ============================================================

func TestCalculateReport_DispatchesSGForms(t *testing.T) {
	// GST F5
	result, err := CalculateReport("IRAS_GST_F5", map[string]interface{}{
		"standard_rated_supplies": "100000",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "box_6_output_tax")

	// Form C
	result, err = CalculateReport("IRAS_FORM_C", map[string]interface{}{
		"revenue": "1000000", "cost_of_sales": "500000",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "tax_payable")

	// Form C-S
	result, err = CalculateReport("IRAS_FORM_CS", map[string]interface{}{
		"revenue": "1000000", "total_expenses": "500000",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "tax_payable")

	// Form B
	result, err = CalculateReport("IRAS_FORM_B", map[string]interface{}{
		"employment_income": "100000",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "tax_payable")

	// IR8A
	result, err = CalculateReport("IRAS_IR8A", map[string]interface{}{
		"gross_salary": "60000",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "employer_cpf")

	// S45
	result, err = CalculateReport("IRAS_S45", map[string]interface{}{
		"payment_amount": "100000", "income_type": "INT",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "tax_withheld")

	// ECI
	result, err = CalculateReport("IRAS_ECI", map[string]interface{}{
		"revenue": "1000000", "adjusted_profit": "200000",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "estimated_tax")
}

// ============================================================
// Real Business Scenarios
// ============================================================

func TestScenario_FnBRestaurant_QuarterlyGST(t *testing.T) {
	// F&B restaurant: S$600k quarterly sales (standard-rated food)
	// Purchases: S$250k, input GST: S$22,500
	input := map[string]interface{}{
		"standard_rated_supplies": "600000",
		"zero_rated_supplies":     "0",
		"exempt_supplies":         "0",
		"taxable_purchases":       "250000",
		"input_tax_claimed":       "22500",
	}
	result, err := CalculateGSTF5(input)
	require.NoError(t, err)

	// Output: 600k * 9% = 54000, Input: 22500, Net: 31500
	assert.Equal(t, "54000.00", result["box_6_output_tax"])
	assert.Equal(t, "31500.00", result["box_8_net_gst"])
}

func TestScenario_TechStartup_FormC(t *testing.T) {
	// Tech startup: revenue S$8M (must use Form C, not C-S)
	// Expenses: S$6M, Other income: S$200k
	input := map[string]interface{}{
		"revenue":            "8000000",
		"cost_of_sales":      "3000000",
		"operating_expenses": "3000000",
		"other_income":       "200000",
	}
	result, err := CalculateFormC(input)
	require.NoError(t, err)

	// Gross: 8M-3M = 5M, Adjusted: 5M-3M+200k = 2.2M
	assert.Equal(t, "2200000.00", result["chargeable_income"])
	// Partial exemption: 102500 (capped)
	assert.Equal(t, "102500.00", result["partial_exemption"])
	// Tax: (2200000-102500)*0.17 = 356575
	assert.Equal(t, "356575.00", result["tax_payable"])
}

func TestScenario_TechStartup_FormCS_Rejected(t *testing.T) {
	// Revenue S$8M -> Form C-S should be rejected
	_, err := CalculateFormCS(map[string]interface{}{
		"revenue":        "8000000",
		"total_expenses": "6000000",
	})
	require.Error(t, err)
}

func TestScenario_Freelancer_FormB(t *testing.T) {
	// Consultant: trade S$180k + rental S$36k, reliefs S$6.4k
	input := map[string]interface{}{
		"trade_income":  "180000",
		"rental_income": "36000",
		"total_reliefs": "6400",
	}
	result, err := CalculateFormB(input)
	require.NoError(t, err)

	assert.Equal(t, "216000.00", result["total_income"])
	assert.Equal(t, "209600.00", result["chargeable_income"]) // 216k - 6.4k
	// Tax: 21150 + (209600-200000)*19% = 21150 + 1824 = 22974
	assert.Equal(t, "22974.00", result["tax_payable"])
}

func TestScenario_TradingCompany_S45_MultiplePayments(t *testing.T) {
	// Royalty to US: S$200k at 10%
	result, err := CalculateS45(map[string]interface{}{
		"payment_amount": "200000", "income_type": "ROY",
	})
	require.NoError(t, err)
	assert.Equal(t, "20000.00", result["tax_withheld"])

	// Management fee to Japan HQ: S$500k at 17%
	result, err = CalculateS45(map[string]interface{}{
		"payment_amount": "500000", "income_type": "MGMT",
	})
	require.NoError(t, err)
	assert.Equal(t, "85000.00", result["tax_withheld"])

	// Equipment rent: S$100k at 15%
	result, err = CalculateS45(map[string]interface{}{
		"payment_amount": "100000", "income_type": "RENT",
	})
	require.NoError(t, err)
	assert.Equal(t, "15000.00", result["tax_withheld"])
}
