package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateBIR2550M_BasicVAT(t *testing.T) {
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "100000", "vat_type": "vatable"},
			map[string]interface{}{"amount": "50000", "vat_type": "government"},
			map[string]interface{}{"amount": "20000", "vat_type": "zero_rated"},
			map[string]interface{}{"amount": "10000", "vat_type": "exempt"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "30000", "vat_amount": "3600", "category": "goods"},
			map[string]interface{}{"amount": "10000", "vat_amount": "1200", "category": "services"},
		},
		"tax_credits": "0",
		"penalties":   "0",
	}

	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	// Part II - Sales
	assert.Equal(t, "100000", result["line_1_vatable_sales"])
	assert.Equal(t, "50000", result["line_2_sales_to_government"])
	assert.Equal(t, "20000", result["line_3_zero_rated_sales"])
	assert.Equal(t, "10000", result["line_4_exempt_sales"])
	assert.Equal(t, "180000", result["line_5_total_sales"])

	// Part III - Output Tax
	// 100000 * 0.12 = 12000
	assert.Equal(t, "12000", result["line_6_output_vat"])
	// 50000 * 0.05 = 2500
	assert.Equal(t, "2500", result["line_6a_output_vat_government"])
	// 12000 + 2500 = 14500
	assert.Equal(t, "14500", result["line_6b_total_output_vat"])

	// Part IV - Input Tax
	assert.Equal(t, "3600", result["line_7_input_vat_goods"])
	assert.Equal(t, "1200", result["line_9_input_vat_services"])
	// 3600 + 1200 = 4800
	assert.Equal(t, "4800", result["line_11_total_input_vat"])

	// Part V - Tax Due
	// 14500 - 4800 = 9700
	assert.Equal(t, "9700", result["line_12_vat_payable"])
	assert.Equal(t, "9700", result["line_14_net_vat_payable"])
	assert.Equal(t, "9700", result["line_16_total_amount_due"])
	assert.Equal(t, "0", result["tax_credit_carried_forward"])
}

func TestCalculateBIR2550M_ExcessInputVAT(t *testing.T) {
	// Input VAT exceeds output VAT → tax credit carried forward
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "10000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "50000", "vat_amount": "6000", "category": "goods"},
		},
	}

	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	// Output: 10000 * 0.12 = 1200
	// Input: 6000
	// Payable: 1200 - 6000 = -4800
	// Net VAT Payable: 0 (clamped)
	// Credit carried forward: 4800
	assert.Equal(t, "1200", result["line_6_output_vat"])
	assert.Equal(t, "6000", result["line_7_input_vat_goods"])
	assert.Equal(t, "-4800", result["line_12_vat_payable"])
	assert.Equal(t, "0", result["line_14_net_vat_payable"])
	assert.Equal(t, "4800", result["tax_credit_carried_forward"])
}

func TestCalculateBIR2550M_AutoCalcInputVAT(t *testing.T) {
	// When vat_amount is 0 or missing, calculate from amount * 12%
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "100000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "50000", "category": "goods"},
		},
	}

	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	// Input VAT auto-calculated: 50000 * 0.12 = 6000
	assert.Equal(t, "6000", result["line_7_input_vat_goods"])
}

func TestCalculateBIR2550Q_DelegatesToM(t *testing.T) {
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "100000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{},
	}

	result, err := CalculateBIR2550Q(input)
	require.NoError(t, err)
	assert.Equal(t, "BIR_2550Q", result["form_type"])
	assert.Equal(t, "100000", result["line_1_vatable_sales"])
}

func TestCalculateBIR1601C(t *testing.T) {
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"total_compensation":     "500000",
			"statutory_minimum_wage": "50000",
			"nontaxable_13th_month":  "25000",
			"nontaxable_deminimis":   "5000",
			"sss_gsis_phic_hdmf":     "10000",
			"other_nontaxable":       "0",
			"tax_withheld":           "20000",
			"adjustment":             "0",
			"surcharge":              "0",
			"interest":               "0",
			"compromise":             "0",
		},
	}

	result, err := CalculateBIR1601C(input)
	require.NoError(t, err)

	assert.Equal(t, "500000", result["line_1_total_compensation"])
	// Total nontaxable = 50000 + 25000 + 5000 + 10000 + 0 = 90000
	assert.Equal(t, "90000", result["line_7_total_nontaxable"])
	// Taxable = 500000 - 90000 = 410000
	assert.Equal(t, "410000", result["line_8_taxable_compensation"])
	assert.Equal(t, "20000", result["line_11_total_tax_remitted"])
	assert.Equal(t, "20000", result["line_16_total_amount_due"])
}

func TestCalculateBIR0619E(t *testing.T) {
	input := map[string]interface{}{
		"ewt_data": map[string]interface{}{
			"total_income_payments":  "1000000",
			"total_taxes_withheld":   "50000",
			"adjustment":             "5000",
			"surcharge":              "1000",
			"interest":               "500",
			"compromise":             "0",
		},
	}

	result, err := CalculateBIR0619E(input)
	require.NoError(t, err)

	assert.Equal(t, "1000000", result["line_1_total_amount_of_income_payments"])
	// Tax still due = max(50000 - 5000, 0) = 45000
	assert.Equal(t, "45000", result["line_4_tax_still_due"])
	assert.Equal(t, "1500", result["line_8_total_penalties"])
	// Total = 45000 + 1500 = 46500
	assert.Equal(t, "46500", result["line_9_total_amount_due"])
}

func TestComputeGraduatedTax(t *testing.T) {
	tests := []struct {
		name     string
		income   string
		expected string
	}{
		{"zero income", "0", "0"},
		{"below 250k", "200000", "0"},
		{"exactly 250k", "250000", "0"},
		{"300k (15% bracket)", "300000", "7500"},         // (300000-250000)*0.15 = 7500
		{"400k (boundary)", "400000", "22500"},            // (400000-250000)*0.15 = 22500
		{"500k (20% bracket)", "500000", "42500"},         // 22500 + (500000-400000)*0.20 = 42500
		{"800k (boundary)", "800000", "102500"},           // 22500 + (800000-400000)*0.20 = 102500
		{"1M (25% bracket)", "1000000", "152500"},         // 102500 + (1000000-800000)*0.25 = 152500
		{"2M (boundary)", "2000000", "402500"},            // 102500 + (2000000-800000)*0.25 = 402500
		{"5M (30% bracket)", "5000000", "1302500"},        // 402500 + (5000000-2000000)*0.30 = 1302500
		{"8M (boundary)", "8000000", "2202500"},           // 402500 + (8000000-2000000)*0.30 = 2202500
		{"10M (35% bracket)", "10000000", "2902500"},      // 2202500 + (10000000-8000000)*0.35 = 2902500
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			income, _ := decimal.NewFromString(tt.income)
			expected, _ := decimal.NewFromString(tt.expected)
			result := computeGraduatedTax(income)
			assert.True(t, expected.Equal(result), "expected %s, got %s", expected, result)
		})
	}
}

func TestCalculateBIR1701_OSD(t *testing.T) {
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts":       "2000000",
			"cost_of_sales":              "800000",
			"other_taxable_income":       "100000",
			"deduction_method":           "osd",
			"creditable_withholding_tax": "50000",
			"quarterly_payments":         "20000",
			"other_credits":              "0",
			"surcharge":                  "0",
			"interest":                   "0",
			"compromise":                 "0",
		},
	}

	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	// Gross income from biz = 2000000 - 800000 = 1200000
	assert.Equal(t, "1200000", result["gross_income_from_business"])
	// Total gross = 1200000 + 100000 = 1300000
	assert.Equal(t, "1300000", result["total_gross_income"])
	// OSD = 2000000 * 0.40 = 800000
	assert.Equal(t, "800000", result["osd_amount"])
	assert.Equal(t, "osd", result["deduction_method"])
	// Net taxable = 1300000 - 800000 = 500000
	assert.Equal(t, "500000", result["net_taxable_income"])
	// Tax due from TRAIN brackets: 22500 + (500000-400000)*0.20 = 42500
	assert.Equal(t, "42500", result["income_tax_due"])
	// Credits = 50000 + 20000 = 70000
	// Tax payable = max(42500 - 70000, 0) = 0
	assert.Equal(t, "0", result["tax_payable"])
	assert.Equal(t, "0", result["total_amount_due"])
}

func TestCalculateBIR1701_Itemized(t *testing.T) {
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts":       "5000000",
			"cost_of_sales":              "1000000",
			"other_taxable_income":       "0",
			"deduction_method":           "itemized",
			"itemized_deductions":        "500000",
			"creditable_withholding_tax": "100000",
			"quarterly_payments":         "0",
			"other_credits":              "0",
		},
	}

	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	// Gross income = 5000000 - 1000000 = 4000000
	assert.Equal(t, "4000000", result["gross_income_from_business"])
	// Net taxable = 4000000 - 500000 = 3500000
	assert.Equal(t, "3500000", result["net_taxable_income"])
	// Tax: 402500 + (3500000-2000000)*0.30 = 402500 + 450000 = 852500
	assert.Equal(t, "852500", result["income_tax_due"])
	// Tax payable = 852500 - 100000 = 752500
	assert.Equal(t, "752500", result["tax_payable"])
}

func TestCalculateBIR1702_RCIT_vs_MCIT(t *testing.T) {
	// RCIT > MCIT scenario
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":               "10000000",
			"cost_of_sales":              "3000000",
			"other_income":               "500000",
			"deduction_method":           "itemized",
			"itemized_deductions":        "2000000",
			"creditable_withholding_tax": "200000",
			"quarterly_payments":         "100000",
			"other_credits":              "0",
			"is_sme":                     "false",
		},
	}

	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	// Gross profit = 10000000 - 3000000 = 7000000
	assert.Equal(t, "7000000", result["gross_profit"])
	// Total gross = 7000000 + 500000 = 7500000
	assert.Equal(t, "7500000", result["total_gross_income"])
	// Net taxable = 7500000 - 2000000 = 5500000
	assert.Equal(t, "5500000", result["net_taxable_income"])
	// RCIT = 5500000 * 0.25 = 1375000
	assert.Equal(t, "1375000", result["rcit_amount"])
	// MCIT = 10000000 * 0.01 = 100000
	assert.Equal(t, "100000", result["mcit_amount"])
	// Tax due = RCIT (higher) = 1375000
	assert.Equal(t, "1375000", result["income_tax_due"])
	// Tax payable = 1375000 - (200000 + 100000) = 1075000
	assert.Equal(t, "1075000", result["tax_payable"])
}

func TestCalculateBIR1702_MCIT_Higher(t *testing.T) {
	// MCIT > RCIT scenario (low profit margin, high gross income)
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":               "50000000",
			"cost_of_sales":              "45000000",
			"other_income":               "0",
			"deduction_method":           "itemized",
			"itemized_deductions":        "3000000",
			"creditable_withholding_tax": "0",
			"quarterly_payments":         "0",
			"other_credits":              "0",
		},
	}

	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	// Gross profit = 50M - 45M = 5M
	// Net taxable = 5M - 3M = 2M
	// RCIT = 2M * 0.25 = 500000
	assert.Equal(t, "500000", result["rcit_amount"])
	// MCIT = 50M * 0.01 = 500000
	assert.Equal(t, "500000", result["mcit_amount"])
	// MCIT == RCIT, so RCIT branch applies (MCIT is NOT strictly greater)
	assert.Equal(t, "500000", result["income_tax_due"])
}

func TestCalculateBIR1702_SME_Rate(t *testing.T) {
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":    "5000000",
			"cost_of_sales":   "2000000",
			"other_income":    "0",
			"deduction_method": "itemized",
			"itemized_deductions": "1000000",
			"is_sme":          "true",
		},
	}

	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	assert.Equal(t, "0.2", result["rcit_rate"])
	// Net taxable = 3000000 - 1000000 = 2000000
	// RCIT = 2000000 * 0.20 = 400000
	assert.Equal(t, "400000", result["rcit_amount"])
}

func TestCalculateBIR2316(t *testing.T) {
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"employee_name":                "Juan Dela Cruz",
			"employee_tin":                 "123-456-789-000",
			"employer_name":                "ABC Corp",
			"employer_tin":                 "999-888-777-000",
			"present_employer_compensation": "600000",
			"present_employer_nontaxable":   "100000",
			"previous_employer_compensation": "200000",
			"previous_employer_nontaxable":   "50000",
			"tax_withheld_present":          "50000",
			"tax_withheld_previous":         "10000",
		},
	}

	result, err := CalculateBIR2316(input)
	require.NoError(t, err)

	assert.Equal(t, "Juan Dela Cruz", result["employee_name"])
	// Present taxable = 600000 - 100000 = 500000
	assert.Equal(t, "500000", result["present_employer_taxable"])
	// Previous taxable = 200000 - 50000 = 150000
	assert.Equal(t, "150000", result["previous_employer_taxable"])
	// Total taxable = 500000 + 150000 = 650000
	assert.Equal(t, "650000", result["total_taxable_compensation"])
	// Tax due: 22500 + (650000-400000)*0.20 = 22500 + 50000 = 72500
	assert.Equal(t, "72500", result["tax_due"])
	// Total withheld = 50000 + 10000 = 60000
	assert.Equal(t, "60000", result["total_tax_withheld"])
	// Diff = 60000 - 72500 = -12500 → still due
	assert.Equal(t, "0", result["amount_refunded"])
	assert.Equal(t, "12500", result["amount_still_due"])
}

func TestCalculateReport_Dispatcher(t *testing.T) {
	// Test the dispatcher routes to correct calculator
	input := map[string]interface{}{
		"sales_data":     []interface{}{},
		"purchases_data": []interface{}{},
	}

	result, err := CalculateReport("BIR_2550M", input)
	require.NoError(t, err)
	assert.Contains(t, result, "line_1_vatable_sales")

	_, err = CalculateReport("UNKNOWN_FORM", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no calculator available")
}

func TestCalculateBIR2307_WithItems(t *testing.T) {
	input := map[string]interface{}{
		"payee_tin":  "123-456-789-000",
		"payee_name": "Test Supplier Inc.",
		"payer_tin":  "987-654-321-000",
		"payer_name": "Test Company Inc.",
		"period":     "2024-01",
		"quarter":    "Q1",
		"items": []interface{}{
			map[string]interface{}{
				"atc_code":      "WI157",
				"income_amount": "100000",
				"tax_rate":      "0.01",
				"tax_withheld":  "1000",
			},
			map[string]interface{}{
				"atc_code":      "WI158",
				"income_amount": "50000",
				"tax_rate":      "0.02",
				"tax_withheld":  "1000",
			},
		},
	}

	result, err := CalculateBIR2307(input)
	require.NoError(t, err)
	assert.Equal(t, "123-456-789-000", result["payee_tin"])
	assert.Equal(t, "Test Supplier Inc.", result["payee_name"])
	assert.Equal(t, "2", result["total_items"])
	assert.Equal(t, "150000", result["total_income_amount"])
	assert.Equal(t, "2000", result["total_tax_withheld"])
	assert.Equal(t, "WI157", result["item_1_atc_code"])
	assert.Equal(t, "WI158", result["item_2_atc_code"])
}

func TestCalculateBIR2307_FlatFields(t *testing.T) {
	input := map[string]interface{}{
		"payee_tin":     "111-222-333-000",
		"payee_name":    "Vendor A",
		"atc_code":      "WI010",
		"income_amount": "200000",
		"tax_rate":      "0.05",
	}

	result, err := CalculateBIR2307(input)
	require.NoError(t, err)
	assert.Equal(t, "200000", result["total_income_amount"])
	assert.Equal(t, "10000", result["total_tax_withheld"])
	assert.Equal(t, "WI010", result["item_1_atc_code"])
}

func TestCalculateSAWT(t *testing.T) {
	input := map[string]interface{}{
		"period": "2024-01",
		"entries": []interface{}{
			map[string]interface{}{
				"tin":             "111-222-333-000",
				"registered_name": "Supplier A",
				"atc_code":        "WI157",
				"income_payment":  "100000",
				"tax_withheld":    "1000",
			},
			map[string]interface{}{
				"tin":             "444-555-666-000",
				"registered_name": "Supplier B",
				"atc_code":        "WI158",
				"income_payment":  "50000",
				"tax_withheld":    "1000",
			},
		},
	}

	result, err := CalculateSAWT(input)
	require.NoError(t, err)
	assert.Equal(t, "2024-01", result["period"])
	assert.Equal(t, "2", result["total_entries"])
	assert.Equal(t, "150000", result["total_income_payment"])
	assert.Equal(t, "2000", result["total_tax_withheld"])
	assert.Equal(t, "111-222-333-000", result["entry_1_tin"])
	assert.Equal(t, "Supplier A", result["entry_1_registered_name"])
	assert.Equal(t, "WI157", result["entry_1_atc_code"])
}

func TestCalculateReport_Dispatches2307AndSAWT(t *testing.T) {
	// Test 2307 dispatch
	input2307 := map[string]interface{}{
		"payee_tin":     "123-456-789-000",
		"atc_code":      "WI157",
		"income_amount": "100000",
		"tax_rate":      "0.01",
	}
	result, err := CalculateReport("BIR_2307", input2307)
	require.NoError(t, err)
	assert.Contains(t, result, "total_tax_withheld")

	// Test SAWT dispatch
	inputSAWT := map[string]interface{}{
		"period":  "2024-01",
		"entries": []interface{}{},
	}
	result, err = CalculateReport("SAWT", inputSAWT)
	require.NoError(t, err)
	assert.Equal(t, "0", result["total_entries"])
}

func TestCalculateBIR2550M_EmptyData(t *testing.T) {
	input := map[string]interface{}{
		"sales_data":     []interface{}{},
		"purchases_data": []interface{}{},
	}

	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "0", result["line_1_vatable_sales"])
	assert.Equal(t, "0", result["line_5_total_sales"])
	assert.Equal(t, "0", result["line_6b_total_output_vat"])
	assert.Equal(t, "0", result["line_11_total_input_vat"])
	assert.Equal(t, "0", result["line_16_total_amount_due"])
}
