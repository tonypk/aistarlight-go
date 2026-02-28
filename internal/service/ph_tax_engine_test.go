package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tonypk/aistarlight-go/pkg/birforms"
)

// ============================================================
// CPA Audit: BIR 2550M — Monthly VAT Declaration
// ============================================================

func TestPH_BIR2550M_StandardRetail(t *testing.T) {
	// Retail store: all vatable sales
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "500000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "200000", "vat_amount": "24000", "category": "goods"},
			map[string]interface{}{"amount": "50000", "vat_amount": "6000", "category": "services"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "500000", result["line_1_vatable_sales"])
	assert.Equal(t, "60000", result["line_6_output_vat"])        // 500K * 12%
	assert.Equal(t, "24000", result["line_7_input_vat_goods"])
	assert.Equal(t, "6000", result["line_9_input_vat_services"])
	assert.Equal(t, "30000", result["line_11_total_input_vat"])  // 24K + 6K
	assert.Equal(t, "30000", result["line_12_vat_payable"])      // 60K - 30K
	assert.Equal(t, "30000", result["line_14_net_vat_payable"])
	assert.Equal(t, "0", result["tax_credit_carried_forward"])
}

func TestPH_BIR2550M_GovernmentContractor(t *testing.T) {
	// Government supplier: 5% VAT withholding
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "1000000", "vat_type": "government"},
			map[string]interface{}{"amount": "200000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "300000", "vat_amount": "36000", "category": "goods"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "200000", result["line_1_vatable_sales"])
	assert.Equal(t, "1000000", result["line_2_sales_to_government"])
	assert.Equal(t, "24000", result["line_6_output_vat"])            // 200K * 12%
	assert.Equal(t, "50000", result["line_6a_output_vat_government"]) // 1M * 5%
	assert.Equal(t, "74000", result["line_6b_total_output_vat"])     // 24K + 50K
	assert.Equal(t, "38000", result["line_12_vat_payable"])          // 74K - 36K
}

func TestPH_BIR2550M_PureExporter_Refund(t *testing.T) {
	// 100% zero-rated exporter → input VAT refund
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "5000000", "vat_type": "zero_rated"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "1000000", "vat_amount": "120000", "category": "goods"},
			map[string]interface{}{"amount": "500000", "vat_amount": "60000", "category": "services"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "5000000", result["line_3_zero_rated_sales"])
	assert.Equal(t, "0", result["line_6_output_vat"])
	assert.Equal(t, "180000", result["line_11_total_input_vat"])
	assert.Equal(t, "-180000", result["line_12_vat_payable"])
	assert.Equal(t, "0", result["line_14_net_vat_payable"])
	assert.Equal(t, "180000", result["tax_credit_carried_forward"])
}

func TestPH_BIR2550M_MixedSupplies(t *testing.T) {
	// Financial institution: standard + exempt
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "300000", "vat_type": "vatable"},
			map[string]interface{}{"amount": "200000", "vat_type": "exempt"},
			map[string]interface{}{"amount": "100000", "vat_type": "zero_rated"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "100000", "vat_amount": "12000", "category": "goods"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "600000", result["line_5_total_sales"]) // 300K + 200K + 100K
	assert.Equal(t, "36000", result["line_6_output_vat"])   // 300K * 12%
	assert.Equal(t, "24000", result["line_12_vat_payable"]) // 36K - 12K
}

func TestPH_BIR2550M_WithTaxCreditsAndPenalties(t *testing.T) {
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "100000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{},
		"tax_credits":     "5000",
		"penalties":       "2000",
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	// Output VAT = 100K * 12% = 12K
	// Net = 12K - 0 - 5K credits = 7K
	assert.Equal(t, "7000", result["line_14_net_vat_payable"])
	// Total due = 7K + 2K penalties = 9K
	assert.Equal(t, "9000", result["line_16_total_amount_due"])
}

func TestPH_BIR2550M_PreAggregatedSummary(t *testing.T) {
	// When data comes from VATSummary (reconciliation session)
	input := map[string]interface{}{
		"vatable_sales":         "1000000",
		"sales_to_government":   "500000",
		"zero_rated_sales":      "200000",
		"vat_exempt_sales":      "100000",
		"output_vat":            "120000",
		"output_vat_government": "25000",
		"input_vat_goods":       "50000",
		"input_vat_capital":     "10000",
		"input_vat_services":    "15000",
		"input_vat_imports":     "5000",
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "1800000", result["line_5_total_sales"])
	assert.Equal(t, "145000", result["line_6b_total_output_vat"]) // 120K + 25K
	assert.Equal(t, "80000", result["line_11_total_input_vat"])   // 50K+10K+15K+5K
	assert.Equal(t, "65000", result["line_12_vat_payable"])       // 145K - 80K
}

func TestPH_BIR2550M_CapitalGoodsAllCategories(t *testing.T) {
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "1000000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "200000", "vat_amount": "24000", "category": "goods"},
			map[string]interface{}{"amount": "500000", "vat_amount": "60000", "category": "capital"},
			map[string]interface{}{"amount": "100000", "vat_amount": "12000", "category": "services"},
			map[string]interface{}{"amount": "50000", "vat_amount": "6000", "category": "imports"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "24000", result["line_7_input_vat_goods"])
	assert.Equal(t, "60000", result["line_8_input_vat_capital"])
	assert.Equal(t, "12000", result["line_9_input_vat_services"])
	assert.Equal(t, "6000", result["line_10_input_vat_imports"])
	assert.Equal(t, "102000", result["line_11_total_input_vat"])
}

func TestPH_BIR2550M_AutoCalcInputVAT(t *testing.T) {
	// When vat_amount is not provided, calculates amount * 12%
	input := map[string]interface{}{
		"sales_data":     []interface{}{},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "100000", "category": "goods"},
			map[string]interface{}{"amount": "50000", "category": "services"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "12000", result["line_7_input_vat_goods"])    // 100K * 12%
	assert.Equal(t, "6000", result["line_9_input_vat_services"])  // 50K * 12%
}

func TestPH_BIR2550M_ExplicitOutputTax(t *testing.T) {
	// When sales rows include explicit output_tax
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "100000", "vat_type": "vatable", "output_tax": "12000"},
			map[string]interface{}{"amount": "50000", "vat_type": "vatable", "output_tax": "6000"},
		},
		"purchases_data": []interface{}{},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "150000", result["line_1_vatable_sales"])
	assert.Equal(t, "18000", result["line_6_output_vat"])  // sum of explicit output_tax
}

// ============================================================
// CPA Audit: BIR 1601C — Monthly WHT Compensation
// ============================================================

func TestPH_BIR1601C_StandardPayroll(t *testing.T) {
	input := map[string]interface{}{
		"total_compensation":     "1000000",
		"statutory_minimum_wage": "100000",
		"nontaxable_13th_month":  "90000",  // at ₱90K limit
		"nontaxable_deminimis":   "10000",
		"sss_gsis_phic_hdmf":     "30000",
		"other_nontaxable":       "5000",
		"tax_withheld":           "50000",
		"adjustment":             "0",
	}
	result, err := CalculateBIR1601C(input)
	require.NoError(t, err)

	// Nontaxable = 100K + 90K + 10K + 30K + 5K = 235K
	assert.Equal(t, "235000", result["line_7_total_nontaxable"])
	// Taxable = 1M - 235K = 765K
	assert.Equal(t, "765000", result["line_8_taxable_compensation"])
	assert.Equal(t, "50000", result["line_11_total_tax_remitted"])
	assert.Equal(t, "50000", result["line_16_total_amount_due"])
}

func TestPH_BIR1601C_MWEOnly(t *testing.T) {
	// All employees are MWE → zero taxable
	input := map[string]interface{}{
		"total_compensation":     "300000",
		"statutory_minimum_wage": "300000",
		"tax_withheld":           "0",
	}
	result, err := CalculateBIR1601C(input)
	require.NoError(t, err)

	assert.Equal(t, "0", result["line_8_taxable_compensation"])
	assert.Equal(t, "0", result["line_16_total_amount_due"])
}

func TestPH_BIR1601C_WithPenalties(t *testing.T) {
	input := map[string]interface{}{
		"total_compensation": "500000",
		"tax_withheld":       "20000",
		"surcharge":          "5000",
		"interest":           "1200",
		"compromise":         "1000",
	}
	result, err := CalculateBIR1601C(input)
	require.NoError(t, err)

	assert.Equal(t, "7200", result["line_15_total_penalties"])   // 5K + 1.2K + 1K
	assert.Equal(t, "27200", result["line_16_total_amount_due"]) // 20K + 7.2K
}

// ============================================================
// CPA Audit: BIR 0619E — Monthly Expanded Withholding Tax
// ============================================================

func TestPH_BIR0619E_StandardRemittance(t *testing.T) {
	input := map[string]interface{}{
		"total_income_payments":  "2000000",
		"total_taxes_withheld":   "40000",
		"adjustment":             "0",
	}
	result, err := CalculateBIR0619E(input)
	require.NoError(t, err)

	assert.Equal(t, "2000000", result["line_1_total_amount_of_income_payments"])
	assert.Equal(t, "40000", result["line_4_tax_still_due"])
	assert.Equal(t, "40000", result["line_9_total_amount_due"])
}

func TestPH_BIR0619E_WithAdjustment(t *testing.T) {
	input := map[string]interface{}{
		"total_income_payments": "500000",
		"total_taxes_withheld":  "10000",
		"adjustment":            "3000",
	}
	result, err := CalculateBIR0619E(input)
	require.NoError(t, err)

	// Tax still due = max(10K - 3K, 0) = 7K
	assert.Equal(t, "7000", result["line_4_tax_still_due"])
}

func TestPH_BIR0619E_AdjustmentExceedsWithheld(t *testing.T) {
	input := map[string]interface{}{
		"total_income_payments": "100000",
		"total_taxes_withheld":  "2000",
		"adjustment":            "5000",
	}
	result, err := CalculateBIR0619E(input)
	require.NoError(t, err)

	// Tax still due = max(2K - 5K, 0) = 0 (clamped)
	assert.Equal(t, "0", result["line_4_tax_still_due"])
}

// ============================================================
// CPA Audit: BIR 1701 — Annual Individual Income Tax
// ============================================================

func TestPH_BIR1701_OSD_Professional(t *testing.T) {
	// Medical doctor: OSD = 40% of gross receipts
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts":       "3000000",
			"cost_of_sales":              "500000",
			"other_taxable_income":       "0",
			"deduction_method":           "osd",
			"creditable_withholding_tax": "150000",
		},
	}
	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	// Gross income = 3M - 500K = 2.5M
	assert.Equal(t, "2500000", result["gross_income_from_business"])
	// OSD = 3M * 40% = 1.2M (based on gross sales, not gross income)
	assert.Equal(t, "1200000", result["osd_amount"])
	// Net taxable = 2.5M - 1.2M = 1.3M
	assert.Equal(t, "1300000", result["net_taxable_income"])
	// Tax: 102500 + (1300000-800000)*0.25 = 102500 + 125000 = 227500
	assert.Equal(t, "227500", result["income_tax_due"])
	// Tax payable = 227500 - 150000 = 77500
	assert.Equal(t, "77500", result["tax_payable"])
}

func TestPH_BIR1701_Itemized_HighIncome(t *testing.T) {
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts": "10000000",
			"cost_of_sales":       "3000000",
			"other_taxable_income": "500000",
			"deduction_method":     "itemized",
			"itemized_deductions":  "2000000",
		},
	}
	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	// Gross income = 10M - 3M = 7M
	// Total gross = 7M + 500K = 7.5M
	// Net taxable = 7.5M - 2M = 5.5M
	assert.Equal(t, "5500000", result["net_taxable_income"])
	// Tax: 402500 + (5.5M-2M)*0.30 = 402500 + 1050000 = 1452500
	assert.Equal(t, "1452500", result["income_tax_due"])
}

func TestPH_BIR1701_BelowThreshold(t *testing.T) {
	// Income below ₱250K → zero tax
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts": "400000",
			"cost_of_sales":       "200000",
			"deduction_method":    "osd",
		},
	}
	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	// Gross income = 400K - 200K = 200K
	// OSD = 400K * 40% = 160K
	// Net taxable = 200K - 160K = 40K
	// TRAIN: 40K < 250K → tax = 0
	assert.Equal(t, "40000", result["net_taxable_income"])
	assert.Equal(t, "0", result["income_tax_due"])
}

func TestPH_BIR1701_TopBracket(t *testing.T) {
	// Income at ₱10M → 35% bracket
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts": "15000000",
			"cost_of_sales":       "0",
			"deduction_method":    "itemized",
			"itemized_deductions": "5000000",
		},
	}
	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	// Net taxable = 15M - 5M = 10M
	// Tax: 2202500 + (10M-8M)*0.35 = 2202500 + 700000 = 2902500
	assert.Equal(t, "2902500", result["income_tax_due"])
}

func TestPH_BIR1701_CreditsExceedTax(t *testing.T) {
	// CWT + quarterly > tax due → tax payable clamped to 0
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts":       "1000000",
			"cost_of_sales":              "300000",
			"deduction_method":           "osd",
			"creditable_withholding_tax": "50000",
			"quarterly_payments":         "30000",
			"other_credits":              "10000",
		},
	}
	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	// Gross income = 700K, OSD = 400K
	// Net taxable = 700K - 400K = 300K
	// Tax: 0 + (300K-250K)*0.15 = 7500
	// Credits: 50K + 30K + 10K = 90K
	// Tax payable = max(7500-90000, 0) = 0
	assert.Equal(t, "7500", result["income_tax_due"])
	assert.Equal(t, "0", result["tax_payable"])
}

// ============================================================
// CPA Audit: BIR 1702 — Annual Corporate Income Tax
// ============================================================

func TestPH_BIR1702_RCIT_StandardCorp(t *testing.T) {
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":        "50000000",
			"cost_of_sales":       "20000000",
			"other_income":        "1000000",
			"deduction_method":    "itemized",
			"itemized_deductions": "10000000",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	// Gross profit = 50M - 20M = 30M
	// Total gross = 30M + 1M = 31M
	// Net taxable = 31M - 10M = 21M
	assert.Equal(t, "21000000", result["net_taxable_income"])
	// RCIT = 21M * 25% = 5,250,000
	assert.Equal(t, "5250000", result["rcit_amount"])
	// MCIT = gross_profit * 2% = 30M * 2% = 600,000
	assert.Equal(t, "600000", result["mcit_amount"])
	// RCIT > MCIT → tax due = RCIT
	assert.Equal(t, "5250000", result["income_tax_due"])
}

func TestPH_BIR1702_MCIT_Applies(t *testing.T) {
	// High deductions make net taxable low, MCIT kicks in
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":        "30000000",
			"cost_of_sales":       "15000000",
			"other_income":        "0",
			"deduction_method":    "itemized",
			"itemized_deductions": "14500000",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	// Gross profit = 30M - 15M = 15M
	// Net taxable = 15M - 14.5M = 500K
	// RCIT = 500K * 25% = 125,000
	assert.Equal(t, "125000", result["rcit_amount"])
	// MCIT = 15M * 2% = 300,000
	assert.Equal(t, "300000", result["mcit_amount"])
	// MCIT > RCIT → tax due = MCIT
	assert.Equal(t, "300000", result["income_tax_due"])
	// Excess MCIT = 300K - 125K = 175K (for carryforward)
	assert.Equal(t, "175000", result["excess_mcit_current"])
}

func TestPH_BIR1702_SME_20Percent(t *testing.T) {
	// SME: net taxable ≤₱5M, total assets ≤₱100M
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":        "8000000",
			"cost_of_sales":       "3000000",
			"other_income":        "0",
			"deduction_method":    "itemized",
			"itemized_deductions": "1000000",
			"is_sme":              "true",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	assert.Equal(t, "0.2", result["rcit_rate"])
	// Gross profit = 5M, Net taxable = 4M
	// RCIT = 4M * 20% = 800K
	assert.Equal(t, "800000", result["rcit_amount"])
	// MCIT = 5M * 2% = 100K
	assert.Equal(t, "100000", result["mcit_amount"])
}

func TestPH_BIR1702_OSD_Corporate(t *testing.T) {
	// OSD for corporations = 40% of gross income (not gross sales!)
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":    "10000000",
			"cost_of_sales":   "4000000",
			"deduction_method": "osd",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	// Gross profit = 10M - 4M = 6M
	assert.Equal(t, "6000000", result["gross_profit"])
	// OSD = gross_profit * 40% = 6M * 40% = 2.4M (corporate OSD based on gross income per NIRC)
	assert.Equal(t, "2400000", result["osd_amount"])
	// Net taxable = 6M - 2.4M (OSD) = 3.6M
	assert.Equal(t, "3600000", result["net_taxable_income"])
}

func TestPH_BIR1702_ExcessMCIT_Carryforward(t *testing.T) {
	// Apply excess MCIT from prior years against RCIT
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":        "20000000",
			"cost_of_sales":       "8000000",
			"deduction_method":    "itemized",
			"itemized_deductions": "4000000",
			"excess_mcit_prior":   "50000",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	// Gross profit = 12M, Net taxable = 8M
	// RCIT = 8M * 25% = 2,000,000
	// MCIT = 12M * 1% = 120,000
	// RCIT > MCIT → tax due = RCIT - excess_mcit_prior
	// Tax due = 2,000,000 - 50,000 = 1,950,000
	assert.Equal(t, "1950000", result["income_tax_due"])
	assert.Equal(t, "50000", result["excess_mcit_prior"])
	// No current excess MCIT since RCIT > MCIT
	assert.Equal(t, "0", result["excess_mcit_current"])
}

func TestPH_BIR1702_LossMaking(t *testing.T) {
	// Loss-making: expenses > income → zero taxable
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":        "5000000",
			"cost_of_sales":       "4000000",
			"deduction_method":    "itemized",
			"itemized_deductions": "3000000",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	// Gross profit = 1M, Net taxable = max(1M-3M, 0) = 0
	assert.Equal(t, "0", result["net_taxable_income"])
	// RCIT = 0
	assert.Equal(t, "0", result["rcit_amount"])
	// MCIT = 1M * 2% = 20,000
	assert.Equal(t, "20000", result["mcit_amount"])
	// MCIT > RCIT → tax due = MCIT
	assert.Equal(t, "20000", result["income_tax_due"])
}

// ============================================================
// CPA Audit: BIR 2316 — Certificate of Compensation
// ============================================================

func TestPH_BIR2316_SingleEmployer(t *testing.T) {
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"employee_name":                "Maria Santos",
			"employee_tin":                 "123-456-789-000",
			"present_employer_compensation": "800000",
			"present_employer_nontaxable":   "200000",
			"tax_withheld_present":          "70000",
		},
	}
	result, err := CalculateBIR2316(input)
	require.NoError(t, err)

	// Taxable = 800K - 200K = 600K
	assert.Equal(t, "600000", result["present_employer_taxable"])
	assert.Equal(t, "600000", result["total_taxable_compensation"])
	// Tax: 22500 + (600K-400K)*0.20 = 22500 + 40000 = 62500
	assert.Equal(t, "62500", result["tax_due"])
	// Over-withheld: 70K - 62.5K = 7500
	assert.Equal(t, "7500", result["amount_refunded"])
	assert.Equal(t, "0", result["amount_still_due"])
}

func TestPH_BIR2316_MultipleEmployers(t *testing.T) {
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"present_employer_compensation":  "500000",
			"present_employer_nontaxable":    "80000",
			"previous_employer_compensation": "300000",
			"previous_employer_nontaxable":   "50000",
			"tax_withheld_present":           "40000",
			"tax_withheld_previous":          "15000",
		},
	}
	result, err := CalculateBIR2316(input)
	require.NoError(t, err)

	// Present taxable = 500K - 80K = 420K
	// Previous taxable = 300K - 50K = 250K
	// Total taxable = 670K
	assert.Equal(t, "670000", result["total_taxable_compensation"])
	// Tax: 22500 + (670K-400K)*0.20 = 22500 + 54000 = 76500
	assert.Equal(t, "76500", result["tax_due"])
	// Total withheld = 40K + 15K = 55K
	assert.Equal(t, "55000", result["total_tax_withheld"])
	// Under-withheld: 76.5K - 55K = 21.5K
	assert.Equal(t, "0", result["amount_refunded"])
	assert.Equal(t, "21500", result["amount_still_due"])
}

func TestPH_BIR2316_BelowThreshold(t *testing.T) {
	// Total taxable < ₱250K → zero tax
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"present_employer_compensation": "300000",
			"present_employer_nontaxable":   "100000",
			"tax_withheld_present":          "5000",
		},
	}
	result, err := CalculateBIR2316(input)
	require.NoError(t, err)

	assert.Equal(t, "200000", result["total_taxable_compensation"])
	assert.Equal(t, "0", result["tax_due"])
	assert.Equal(t, "5000", result["amount_refunded"]) // Full refund
}

// ============================================================
// CPA Audit: BIR 2307 — Creditable WHT Certificate
// ============================================================

func TestPH_BIR2307_MultipleATCCodes(t *testing.T) {
	input := map[string]interface{}{
		"payee_tin":  "123-456-789-000",
		"payee_name": "Professional Services Inc.",
		"payer_tin":  "987-654-321-000",
		"period":     "2024-03",
		"quarter":    "Q1",
		"items": []interface{}{
			map[string]interface{}{
				"atc_code":      "WI020",
				"income_amount": "500000",
				"tax_rate":      "0.10",
				"tax_withheld":  "50000",
			},
			map[string]interface{}{
				"atc_code":      "WI100",
				"income_amount": "120000",
				"tax_rate":      "0.05",
				"tax_withheld":  "6000",
			},
		},
	}
	result, err := CalculateBIR2307(input)
	require.NoError(t, err)

	assert.Equal(t, "2", result["total_items"])
	assert.Equal(t, "620000", result["total_income_amount"])
	assert.Equal(t, "56000", result["total_tax_withheld"])
}

func TestPH_BIR2307_AutoComputeTax(t *testing.T) {
	// tax_withheld not provided → auto-computed from rate
	input := map[string]interface{}{
		"payee_tin":     "111-222-333-000",
		"atc_code":      "WI157",
		"income_amount": "1000000",
		"tax_rate":      "0.01",
	}
	result, err := CalculateBIR2307(input)
	require.NoError(t, err)

	assert.Equal(t, "10000", result["total_tax_withheld"]) // 1M * 1%
}

// ============================================================
// CPA Audit: TRAIN Law Graduated Tax Brackets
// ============================================================

func TestPH_TRAINBrackets_AllBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		income      string
		expectedTax string
	}{
		{"zero", "0", "0"},
		{"100K_0pct", "100000", "0"},
		{"250K_exactly", "250000", "0"},
		{"250001_15pct", "250001", "0.15"},
		{"300K_15pct", "300000", "7500"},         // (300K-250K)*15%
		{"400K_boundary", "400000", "22500"},      // (400K-250K)*15%
		{"400001_20pct", "400001", "22500.2"},     // 22500 + 1*0.20
		{"500K_20pct", "500000", "42500"},          // 22500 + (500K-400K)*20%
		{"800K_boundary", "800000", "102500"},      // 22500 + (800K-400K)*20%
		{"800001_25pct", "800001", "102500.25"},    // 102500 + 1*0.25
		{"1M_25pct", "1000000", "152500"},          // 102500 + (1M-800K)*25%
		{"2M_boundary", "2000000", "402500"},       // 102500 + (2M-800K)*25%
		{"2000001_30pct", "2000001", "402500.3"},   // 402500 + 1*0.30
		{"5M_30pct", "5000000", "1302500"},         // 402500 + (5M-2M)*30%
		{"8M_boundary", "8000000", "2202500"},      // 402500 + (8M-2M)*30%
		{"8000001_35pct", "8000001", "2202500.35"}, // 2202500 + 1*0.35
		{"10M_35pct", "10000000", "2902500"},       // 2202500 + (10M-8M)*35%
		{"20M_35pct", "20000000", "6402500"},       // 2202500 + (20M-8M)*35%
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			income := decimal.RequireFromString(tt.income)
			expected := decimal.RequireFromString(tt.expectedTax)
			result := computeGraduatedTax(income)
			assert.True(t, expected.Equal(result),
				"income=%s: expected %s, got %s", tt.income, expected, result)
		})
	}
}

func TestPH_TRAINBrackets_ConstantsCorrect(t *testing.T) {
	// Verify all bracket constants match TRAIN Law RA 10963
	expected := []struct {
		over    int64
		notOver int64
		base    int64
		rate    float64
	}{
		{0, 250000, 0, 0.00},
		{250000, 400000, 0, 0.15},
		{400000, 800000, 22500, 0.20},
		{800000, 2000000, 102500, 0.25},
		{2000000, 8000000, 402500, 0.30},
		{8000000, 0, 2202500, 0.35}, // 0 = no upper limit
	}

	require.Len(t, birforms.TRAINBrackets, 6, "TRAIN law has 6 brackets")

	for i, exp := range expected {
		b := birforms.TRAINBrackets[i]
		assert.True(t, decimal.NewFromInt(exp.over).Equal(b.Over),
			"bracket %d Over: expected %d, got %s", i, exp.over, b.Over)
		assert.True(t, decimal.NewFromInt(exp.notOver).Equal(b.NotOver),
			"bracket %d NotOver: expected %d, got %s", i, exp.notOver, b.NotOver)
		assert.True(t, decimal.NewFromInt(exp.base).Equal(b.BaseTax),
			"bracket %d BaseTax: expected %d, got %s", i, exp.base, b.BaseTax)
		assert.True(t, decimal.NewFromFloat(exp.rate).Equal(b.Rate),
			"bracket %d Rate: expected %.2f, got %s", i, exp.rate, b.Rate)
	}
}

// ============================================================
// CPA Audit: Tax Rate Constants
// ============================================================

func TestPH_TaxRateConstants(t *testing.T) {
	assert.True(t, birforms.VATRate.Equal(decimal.NewFromFloat(0.12)),
		"VAT rate should be 12%%")
	assert.True(t, birforms.GovtVATRate.Equal(decimal.NewFromFloat(0.05)),
		"Government VAT rate should be 5%%")
	assert.True(t, birforms.RCIT.Equal(decimal.NewFromFloat(0.25)),
		"RCIT should be 25%%")
	assert.True(t, birforms.RCITReduced.Equal(decimal.NewFromFloat(0.20)),
		"RCIT Reduced (SME) should be 20%%")
	assert.True(t, birforms.MCITRate(2021).Equal(decimal.NewFromFloat(0.01)),
		"MCIT should be 1%% during CREATE Act (2020-2022)")
	assert.True(t, birforms.MCITRate(2024).Equal(decimal.NewFromFloat(0.02)),
		"MCIT should be 2%% for 2024+ (standard rate)")
	assert.True(t, birforms.MCITRate(2019).Equal(decimal.NewFromFloat(0.02)),
		"MCIT should be 2%% before CREATE Act")
}

// ============================================================
// CPA Audit: ATC Code Rates
// ============================================================

func TestPH_ATCCodeRates(t *testing.T) {
	// Canonical ATC rates from EWTRates (single source of truth)
	tests := []struct {
		code string
		rate float64
		desc string
	}{
		{"WI010", 0.05, "Professional fees - Individual <3M"},
		{"WI010A", 0.05, "Medical practitioner"},
		{"WI020", 0.10, "Professional fees - Individual >=3M"},
		{"WC010", 0.10, "Professional fees - Corporation"},
		{"WI030", 0.05, "Rent - Real property"},
		{"WI040", 0.05, "Rent - Personal property"},
		{"WI050", 0.02, "Contractors - Individual"},
		{"WC050", 0.02, "Contractors - Corporation"},
		{"WI070", 0.10, "Commission - Individual"},
		{"WC070", 0.10, "Commission - Corporation"},
		{"WB010", 0.10, "Broker fees"},
		{"WI100", 0.01, "Purchase of goods - Individual >3M"},
		{"WC100", 0.01, "Purchase of goods - Corporation >3M"},
		{"WI157", 0.01, "Income payment to supplier of goods"},
		{"WI158", 0.02, "Income payment to supplier of services"},
		{"WI700", 0.20, "Directors fees"},
		{"WI640", 0.10, "Insurance commissions"},
		{"WI160", 0.05, "Tolling/manufacturing fees"},
		{"WV010", 0.05, "VAT Withholding - government goods"},
		{"WV020", 0.05, "VAT Withholding - government services"},
		{"WI155", 0.01, "Gross purchase of agricultural products"},
		{"WI159", 0.15, "Income to beneficiaries of estates/trusts"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			rate, err := GetEWTRate(tt.code)
			require.NoError(t, err, "ATC code %s should exist in EWTRates", tt.code)
			assert.True(t, decimal.NewFromFloat(tt.rate).Equal(rate),
				"ATC %s rate: expected %.2f, got %s", tt.code, tt.rate, rate)
		})
	}
}

// ============================================================
// CPA Audit: Dispatcher
// ============================================================

func TestPH_Dispatcher_AllBIRForms(t *testing.T) {
	// BIR 2550M
	result, err := CalculateReport("BIR_2550M", map[string]interface{}{
		"sales_data": []interface{}{}, "purchases_data": []interface{}{},
	})
	require.NoError(t, err)
	assert.Contains(t, result, "line_6_output_vat")

	// BIR 2550Q
	result, err = CalculateReport("BIR_2550Q", map[string]interface{}{
		"sales_data": []interface{}{}, "purchases_data": []interface{}{},
	})
	require.NoError(t, err)
	assert.Equal(t, "BIR_2550Q", result["form_type"])

	// BIR 1601C
	result, err = CalculateReport("BIR_1601C", map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, result, "line_8_taxable_compensation")

	// BIR 0619E
	result, err = CalculateReport("BIR_0619E", map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, result, "line_4_tax_still_due")

	// BIR 1701
	result, err = CalculateReport("BIR_1701", map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, result, "income_tax_due")

	// BIR 1702
	result, err = CalculateReport("BIR_1702", map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, result, "rcit_amount")
	assert.Contains(t, result, "mcit_amount")
	assert.Contains(t, result, "excess_mcit_current")

	// BIR 2316
	result, err = CalculateReport("BIR_2316", map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, result, "tax_due")

	// BIR 2307
	result, err = CalculateReport("BIR_2307", map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, result, "total_tax_withheld")

	// SAWT
	result, err = CalculateReport("SAWT", map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, result, "total_tax_withheld")

	// Unknown form
	_, err = CalculateReport("UNKNOWN", map[string]interface{}{})
	assert.Error(t, err)
}

// ============================================================
// CPA Audit: Real Business Scenarios
// ============================================================

func TestScenarioPH_SariSariStore_MonthlyVAT(t *testing.T) {
	// Small retailer: ₱200K monthly vatable sales, ₱120K purchases
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "200000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "120000", "vat_amount": "14400", "category": "goods"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	// Output: 200K * 12% = 24K
	assert.Equal(t, "24000", result["line_6_output_vat"])
	// Input: 14.4K
	assert.Equal(t, "14400", result["line_7_input_vat_goods"])
	// Net VAT: 24K - 14.4K = 9.6K
	assert.Equal(t, "9600", result["line_14_net_vat_payable"])
}

func TestScenarioPH_BPO_ZeroRatedExporter(t *testing.T) {
	// BPO: ₱50M zero-rated + ₱5M domestic, high input VAT from operations
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "50000000", "vat_type": "zero_rated"},
			map[string]interface{}{"amount": "5000000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "10000000", "vat_amount": "1200000", "category": "goods"},
			map[string]interface{}{"amount": "5000000", "vat_amount": "600000", "category": "services"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "50000000", result["line_3_zero_rated_sales"])
	assert.Equal(t, "600000", result["line_6_output_vat"]) // 5M * 12%
	assert.Equal(t, "1800000", result["line_11_total_input_vat"])
	// Net VAT = 600K - 1.8M = -1.2M → refund
	assert.Equal(t, "-1200000", result["line_12_vat_payable"])
	assert.Equal(t, "0", result["line_14_net_vat_payable"])
	assert.Equal(t, "1200000", result["tax_credit_carried_forward"])
}

func TestScenarioPH_ConstructionFirm_RCIT_vs_MCIT(t *testing.T) {
	// Construction: Revenue ₱100M, COGS ₱85M, Expenses ₱12M
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":        "100000000",
			"cost_of_sales":       "85000000",
			"other_income":        "0",
			"deduction_method":    "itemized",
			"itemized_deductions": "12000000",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	// Gross profit = 100M - 85M = 15M
	assert.Equal(t, "15000000", result["gross_profit"])
	// Net taxable = 15M - 12M = 3M
	assert.Equal(t, "3000000", result["net_taxable_income"])
	// RCIT = 3M * 25% = 750K
	assert.Equal(t, "750000", result["rcit_amount"])
	// MCIT = 15M * 2% = 300K (NOT 100M * 2%!)
	assert.Equal(t, "300000", result["mcit_amount"])
	// RCIT > MCIT → RCIT applies
	assert.Equal(t, "750000", result["income_tax_due"])
}

func TestScenarioPH_MedicalDoctor_1701(t *testing.T) {
	// Doctor: ₱3M gross receipts, OSD method
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts":       "3000000",
			"cost_of_sales":              "500000",
			"deduction_method":           "osd",
			"creditable_withholding_tax": "150000",
			"quarterly_payments":         "50000",
		},
	}
	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	// OSD = 3M * 40% = 1.2M
	assert.Equal(t, "1200000", result["osd_amount"])
	// Gross income = 3M - 500K = 2.5M
	// Net taxable = 2.5M - 1.2M = 1.3M
	assert.Equal(t, "1300000", result["net_taxable_income"])
	// Tax: 102500 + (1.3M-800K)*25% = 102500 + 125000 = 227500
	assert.Equal(t, "227500", result["income_tax_due"])
	// Credits = 150K + 50K = 200K
	// Payable = 227.5K - 200K = 27.5K
	assert.Equal(t, "27500", result["tax_payable"])
}

func TestScenarioPH_RestaurantChain_MixedSales(t *testing.T) {
	// Restaurant: standard sales + government catering
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "8000000", "vat_type": "vatable"},
			map[string]interface{}{"amount": "2000000", "vat_type": "government"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "4000000", "vat_amount": "480000", "category": "goods"},
			map[string]interface{}{"amount": "500000", "vat_amount": "60000", "category": "services"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	// Output: 8M*12% + 2M*5% = 960K + 100K = 1.06M
	assert.Equal(t, "960000", result["line_6_output_vat"])
	assert.Equal(t, "100000", result["line_6a_output_vat_government"])
	assert.Equal(t, "1060000", result["line_6b_total_output_vat"])
	// Input: 480K + 60K = 540K
	assert.Equal(t, "540000", result["line_11_total_input_vat"])
	// Net: 1.06M - 540K = 520K
	assert.Equal(t, "520000", result["line_14_net_vat_payable"])
}

// ============================================================
// CPA Audit: MCIT Period-Aware Rate (C-1)
// ============================================================

func TestPH_MCITRate_PeriodAware(t *testing.T) {
	tests := []struct {
		year int
		rate string
		desc string
	}{
		{2019, "0.02", "Pre-CREATE Act: 2%"},
		{2020, "0.01", "CREATE Act year 1: 1%"},
		{2021, "0.01", "CREATE Act year 2: 1%"},
		{2022, "0.01", "CREATE Act year 3: 1%"},
		{2023, "0.02", "Post-CREATE temporary: 2%"},
		{2024, "0.02", "Current standard: 2%"},
		{2025, "0.02", "Future year: 2%"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			assert.Equal(t, tt.rate, birforms.MCITRate(tt.year).String())
		})
	}
}

func TestPH_BIR1702_MCIT_CreateActYear(t *testing.T) {
	// During CREATE Act (2020-2022), MCIT = 1%
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":        "10000000",
			"cost_of_sales":       "5000000",
			"deduction_method":    "itemized",
			"itemized_deductions": "4900000",
			"taxable_year":        2021,
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	// GP = 5M, MCIT = 5M × 1% = 50,000
	assert.Equal(t, "0.01", result["mcit_rate"])
	assert.Equal(t, "50000", result["mcit_amount"])
}

func TestPH_BIR1702_MCIT_Post2023(t *testing.T) {
	// After CREATE Act (2024+), MCIT = 2%
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":        "10000000",
			"cost_of_sales":       "5000000",
			"deduction_method":    "itemized",
			"itemized_deductions": "4900000",
			"taxable_year":        2024,
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	// GP = 5M, MCIT = 5M × 2% = 100,000
	assert.Equal(t, "0.02", result["mcit_rate"])
	assert.Equal(t, "100000", result["mcit_amount"])
}

// ============================================================
// CPA Audit: Corporate OSD Base (M-5)
// ============================================================

func TestPH_BIR1702_OSD_UsesGrossProfit(t *testing.T) {
	// Verify OSD = 40% of gross profit (not gross revenue)
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":    "20000000",
			"cost_of_sales":   "12000000",
			"deduction_method": "osd",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	// GP = 20M - 12M = 8M
	assert.Equal(t, "8000000", result["gross_profit"])
	// OSD = 8M × 40% = 3.2M (NOT 20M × 40% = 8M)
	assert.Equal(t, "3200000", result["osd_amount"])
	// Net taxable = 8M - 3.2M = 4.8M
	assert.Equal(t, "4800000", result["net_taxable_income"])
}

// ============================================================
// CPA Audit: Validation Warnings (H-3, H-4, H-5)
// ============================================================

func TestPH_BIR1601C_Warning_13thMonthExceedsCap(t *testing.T) {
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"total_compensation":    "500000",
			"nontaxable_13th_month": "120000", // Exceeds ₱90K cap
			"tax_withheld":          "50000",
		},
	}
	result, err := CalculateBIR1601C(input)
	require.NoError(t, err)

	assert.Contains(t, result["warnings"], "13th month pay")
	assert.Contains(t, result["warnings"], "90,000")
}

func TestPH_BIR1601C_Warning_DeMinimisExceedsCap(t *testing.T) {
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"total_compensation":  "500000",
			"nontaxable_deminimis": "150000", // Exceeds ₱90K ceiling
			"tax_withheld":        "50000",
		},
	}
	result, err := CalculateBIR1601C(input)
	require.NoError(t, err)

	assert.Contains(t, result["warnings"], "De minimis")
	assert.Contains(t, result["warnings"], "90,000")
}

func TestPH_BIR1601C_NoWarning_BelowCaps(t *testing.T) {
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"total_compensation":    "500000",
			"nontaxable_13th_month": "80000", // Below ₱90K
			"nontaxable_deminimis":  "50000", // Below ₱90K
			"tax_withheld":          "50000",
		},
	}
	result, err := CalculateBIR1601C(input)
	require.NoError(t, err)

	_, hasWarnings := result["warnings"]
	assert.False(t, hasWarnings, "No warnings expected when below caps")
}

func TestPH_BIR2550M_VATInclusive_InputVAT(t *testing.T) {
	// H-5: VAT-inclusive purchase should use extraction formula (amount * 12/112)
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "1000000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{
				"amount":           "112000", // VAT-inclusive amount
				"is_vat_inclusive": true,
				"category":        "goods",
			},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	// Input VAT = 112000 * 12/112 = 12000 (NOT 112000 * 12% = 13440)
	assert.Equal(t, "12000", result["line_7_input_vat_goods"])
}

func TestPH_BIR2550M_VATExclusive_InputVAT(t *testing.T) {
	// Default: VAT-exclusive purchase → amount * 12%
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "1000000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{
				"amount":   "100000",
				"category": "goods",
			},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	// Input VAT = 100000 * 12% = 12000
	assert.Equal(t, "12000", result["line_7_input_vat_goods"])
}
