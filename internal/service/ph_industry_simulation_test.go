package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tonypk/aistarlight-go/pkg/birforms"
)

// ============================================================
// INDUSTRY 1: Convenience Store / Sari-Sari Store (Retail)
// Monthly revenue ~₱800K, mixed VAT/exempt, 5 employees
// Forms: 2550M, 1601C, 0619E, 1702 (annual)
// ============================================================

func TestIndustry_ConvenienceStore_MonthlyVAT(t *testing.T) {
	// Monthly sales breakdown:
	// - Grocery items (vatable): ₱650,000
	// - Rice & basic necessities (exempt per RA 10963): ₱120,000
	// - Cigarettes & alcohol (vatable): ₱30,000
	// Purchases: goods ₱450,000 (VAT ₱54,000), utilities ₱15,000 (VAT ₱1,800)
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "650000", "vat_type": "vatable"},
			map[string]interface{}{"amount": "120000", "vat_type": "exempt"},
			map[string]interface{}{"amount": "30000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "450000", "vat_amount": "54000", "category": "goods"},
			map[string]interface{}{"amount": "15000", "vat_amount": "1800", "category": "services"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	// Vatable = 650K + 30K = 680K
	assert.Equal(t, "680000", result["line_1_vatable_sales"])
	assert.Equal(t, "120000", result["line_4_exempt_sales"])
	assert.Equal(t, "800000", result["line_5_total_sales"]) // 680K + 120K

	// Output VAT = 680K × 12% = 81,600
	assert.Equal(t, "81600", result["line_6_output_vat"])

	// Input VAT = 54K + 1.8K = 55,800
	assert.Equal(t, "55800", result["line_11_total_input_vat"])

	// VAT Payable = 81,600 - 55,800 = 25,800
	assert.Equal(t, "25800", result["line_12_vat_payable"])
	assert.Equal(t, "25800", result["line_16_total_amount_due"])
}

func TestIndustry_ConvenienceStore_MonthlyPayrollWHT(t *testing.T) {
	// 5 employees, total monthly compensation ₱120,000
	// 2 MWE (minimum wage earners, exempt): ₱33,000
	// 13th month (prorated monthly): ₱10,000
	// SSS+PhilHealth+PagIBIG: ₱4,800
	// Tax withheld per BIR table: ₱3,250
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"total_compensation":     "120000",
			"statutory_minimum_wage": "33000",
			"nontaxable_13th_month":  "10000",
			"nontaxable_deminimis":   "0",
			"sss_gsis_phic_hdmf":     "4800",
			"other_nontaxable":       "0",
			"tax_withheld":           "3250",
		},
	}
	result, err := CalculateBIR1601C(input)
	require.NoError(t, err)

	// Total nontaxable = 33K + 10K + 0 + 4.8K + 0 = 47,800
	assert.Equal(t, "47800", result["line_7_total_nontaxable"])
	// Taxable = 120K - 47.8K = 72,200
	assert.Equal(t, "72200", result["line_8_taxable_compensation"])
	assert.Equal(t, "3250", result["line_9_tax_withheld"])
	assert.Equal(t, "3250", result["line_11_total_tax_remitted"])
}

func TestIndustry_ConvenienceStore_MonthlyEWT(t *testing.T) {
	// Rent payment to individual landlord: ₱25,000/month
	// EWT on rent: 5% = ₱1,250
	// Supplier of goods (>₱3M annual): ₱450,000, EWT 1% = ₱4,500
	input := map[string]interface{}{
		"ewt_data": map[string]interface{}{
			"total_income_payments": "475000",
			"total_taxes_withheld":  "5750",
		},
	}
	result, err := CalculateBIR0619E(input)
	require.NoError(t, err)

	assert.Equal(t, "475000", result["line_1_total_amount_of_income_payments"])
	assert.Equal(t, "5750", result["line_2_total_taxes_withheld"])
	assert.Equal(t, "5750", result["line_4_tax_still_due"])
	assert.Equal(t, "5750", result["line_9_total_amount_due"])
}

func TestIndustry_ConvenienceStore_AnnualCorporateITR(t *testing.T) {
	// Annual financials:
	// Revenue: ₱9,600,000 (₱800K × 12)
	// COGS: ₱6,720,000 (70% of revenue)
	// Gross profit: ₱2,880,000
	// Operating expenses: ₱1,800,000 (rent, salaries, utilities, etc.)
	// Net taxable: ₱2,880,000 - ₱1,800,000 = ₱1,080,000
	// RCIT: ₱1,080,000 × 25% = ₱270,000
	// MCIT: ₱2,880,000 × 2% = ₱57,600 (2% for 2024+)
	// Tax due = higher of RCIT vs MCIT = ₱270,000 (RCIT)
	// CWT from suppliers: ₱48,000, Quarterly payments: ₱180,000
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":               "9600000",
			"cost_of_sales":              "6720000",
			"other_income":               "0",
			"deduction_method":           "itemized",
			"itemized_deductions":        "1800000",
			"creditable_withholding_tax": "48000",
			"quarterly_payments":         "180000",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	assert.Equal(t, "2880000", result["gross_profit"])
	assert.Equal(t, "1080000", result["net_taxable_income"])
	assert.Equal(t, "270000", result["rcit_amount"])  // 1.08M × 25%
	assert.Equal(t, "57600", result["mcit_amount"])    // 2.88M × 2% (2024+)
	assert.Equal(t, "270000", result["income_tax_due"]) // RCIT > MCIT
	// Tax payable = 270K - 48K - 180K = 42,000
	assert.Equal(t, "42000", result["tax_payable"])
}

// ============================================================
// INDUSTRY 2: IT/BPO Company (Zero-Rated Exporter)
// PEZA-registered, 200 employees, exports to US clients
// Forms: 2550M (zero-rated), 1601C, 1702
// ============================================================

func TestIndustry_BPO_MonthlyVAT_ZeroRated(t *testing.T) {
	// BPO revenue all zero-rated (export services under PEZA):
	// - Export services to US client: ₱5,000,000
	// - Local vatable services (small): ₱200,000
	// Purchases: office supplies ₱100K (VAT ₱12K), IT equipment ₱500K (VAT ₱60K),
	//            office rent ₱300K (VAT ₱36K)
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "5000000", "vat_type": "zero_rated"},
			map[string]interface{}{"amount": "200000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "100000", "vat_amount": "12000", "category": "goods"},
			map[string]interface{}{"amount": "500000", "vat_amount": "60000", "category": "capital"},
			map[string]interface{}{"amount": "300000", "vat_amount": "36000", "category": "services"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "200000", result["line_1_vatable_sales"])
	assert.Equal(t, "5000000", result["line_3_zero_rated_sales"])
	assert.Equal(t, "5200000", result["line_5_total_sales"])

	// Output VAT only on vatable: 200K × 12% = 24,000
	assert.Equal(t, "24000", result["line_6_output_vat"])

	// Total input VAT = 12K + 60K + 36K = 108,000
	assert.Equal(t, "108000", result["line_11_total_input_vat"])

	// VAT payable = 24K - 108K = -84,000 (excess input VAT)
	assert.Equal(t, "-84000", result["line_12_vat_payable"])

	// Net VAT payable = max(0, -84K) = 0
	assert.Equal(t, "0", result["line_14_net_vat_payable"])

	// Tax credit carried forward = 84,000 (for VAT refund claim)
	assert.Equal(t, "84000", result["tax_credit_carried_forward"])
}

func TestIndustry_BPO_LargePayrollWHT(t *testing.T) {
	// 200 employees, total monthly payroll: ₱8,000,000
	// No MWE (all professional staff)
	// 13th month prorated: ₱666,667
	// SSS+PhilHealth+PagIBIG: ₱150,000
	// De minimis: ₱100,000 (rice, clothing, medical allowances)
	// Tax withheld: ₱920,000
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"total_compensation":     "8000000",
			"statutory_minimum_wage": "0",
			"nontaxable_13th_month":  "666667",
			"nontaxable_deminimis":   "100000",
			"sss_gsis_phic_hdmf":     "150000",
			"other_nontaxable":       "0",
			"tax_withheld":           "920000",
		},
	}
	result, err := CalculateBIR1601C(input)
	require.NoError(t, err)

	// Total nontaxable = 666,667 + 100K + 150K = 916,667
	assert.Equal(t, "916667", result["line_7_total_nontaxable"])
	// Taxable = 8M - 916,667 = 7,083,333
	assert.Equal(t, "7083333", result["line_8_taxable_compensation"])
	assert.Equal(t, "920000", result["line_11_total_tax_remitted"])
}

func TestIndustry_BPO_AnnualCorpITR_SME(t *testing.T) {
	// Small BPO, qualifies for CREATE MORE 20% reduced rate
	// Revenue: ₱15,000,000 (all export)
	// COGS: ₱9,000,000 (mostly salaries)
	// GP: ₱6,000,000
	// Deductions: ₱4,500,000
	// Net taxable: ₱1,500,000 (≤ ₱5M → SME eligible if assets ≤ ₱100M)
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":               "15000000",
			"cost_of_sales":              "9000000",
			"other_income":               "0",
			"deduction_method":           "itemized",
			"itemized_deductions":        "4500000",
			"is_sme":                     true,
			"creditable_withholding_tax": "75000",
			"quarterly_payments":         "200000",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	assert.Equal(t, "6000000", result["gross_profit"])
	assert.Equal(t, "1500000", result["net_taxable_income"])
	assert.Equal(t, "0.2", result["rcit_rate"]) // SME 20%
	assert.Equal(t, "300000", result["rcit_amount"])  // 1.5M × 20%
	assert.Equal(t, "120000", result["mcit_amount"])  // 6M × 2%
	assert.Equal(t, "300000", result["income_tax_due"]) // RCIT > MCIT
	// Payable = 300K - 75K - 200K = 25,000
	assert.Equal(t, "25000", result["tax_payable"])
}

// ============================================================
// INDUSTRY 3: Construction Company (Government Contractor)
// Mix of government and private projects
// Forms: 2550M (govt VAT), 0619E, 1702, 2307
// ============================================================

func TestIndustry_Construction_MonthlyVAT_GovtProjects(t *testing.T) {
	// Progress billing:
	// - Government road project: ₱8,000,000 (5% VAT withholding)
	// - Private condo building: ₱3,000,000 (vatable 12%)
	// - Low-cost housing (VAT exempt per RA 9994): ₱1,500,000
	// Purchases: materials ₱5M (VAT ₱600K), subcon ₱2M (VAT ₱240K),
	//            equipment rental ₱500K (VAT ₱60K)
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "8000000", "vat_type": "government"},
			map[string]interface{}{"amount": "3000000", "vat_type": "vatable"},
			map[string]interface{}{"amount": "1500000", "vat_type": "exempt"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "5000000", "vat_amount": "600000", "category": "goods"},
			map[string]interface{}{"amount": "2000000", "vat_amount": "240000", "category": "services"},
			map[string]interface{}{"amount": "500000", "vat_amount": "60000", "category": "capital"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "3000000", result["line_1_vatable_sales"])
	assert.Equal(t, "8000000", result["line_2_sales_to_government"])
	assert.Equal(t, "1500000", result["line_4_exempt_sales"])
	assert.Equal(t, "12500000", result["line_5_total_sales"])

	// Output VAT: vatable 3M × 12% = 360K, govt 8M × 5% = 400K
	assert.Equal(t, "360000", result["line_6_output_vat"])
	assert.Equal(t, "400000", result["line_6a_output_vat_government"])
	assert.Equal(t, "760000", result["line_6b_total_output_vat"])

	// Input VAT = 600K + 60K + 240K = 900K
	assert.Equal(t, "900000", result["line_11_total_input_vat"])

	// VAT payable = 760K - 900K = -140K (refundable)
	assert.Equal(t, "-140000", result["line_12_vat_payable"])
	assert.Equal(t, "0", result["line_14_net_vat_payable"])
	assert.Equal(t, "140000", result["tax_credit_carried_forward"])
}

func TestIndustry_Construction_AnnualCorp_MCIT_Scenario(t *testing.T) {
	// Construction company with thin margins (heavy subcontracting):
	// Revenue: ₱150,000,000
	// COGS: ₱140,000,000 (93% — lots of materials + subcontractors)
	// Gross profit: ₱10,000,000
	// Deductions: ₱9,500,000 (salaries, rent, insurance, depreciation)
	// Net taxable: ₱500,000
	// RCIT: ₱500K × 25% = ₱125,000
	// MCIT: ₱10M × 2% = ₱200,000
	// MCIT > RCIT, so MCIT applies
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":               "150000000",
			"cost_of_sales":              "140000000",
			"other_income":               "0",
			"deduction_method":           "itemized",
			"itemized_deductions":        "9500000",
			"creditable_withholding_tax": "3000000",
			"quarterly_payments":         "0",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	assert.Equal(t, "10000000", result["gross_profit"])
	assert.Equal(t, "500000", result["net_taxable_income"])
	assert.Equal(t, "125000", result["rcit_amount"])
	assert.Equal(t, "200000", result["mcit_amount"])           // 10M × 2%
	assert.Equal(t, "200000", result["income_tax_due"])        // MCIT wins at 2%
	assert.Equal(t, "75000", result["excess_mcit_current"])    // 200K - 125K
	// CWT exceeds tax due → payable = 0
	assert.Equal(t, "0", result["tax_payable"])
}

func TestIndustry_Construction_MCIT_Exceeds_RCIT(t *testing.T) {
	// Loss-making year due to project delays:
	// Revenue: ₱80,000,000
	// COGS: ₱70,000,000
	// GP: ₱10,000,000
	// Deductions: ₱9,900,000
	// Net taxable: ₱100,000
	// RCIT: ₱100K × 25% = ₱25,000
	// MCIT: ₱10M × 2% = ₱200,000 > RCIT
	// Excess MCIT = 200K - 25K = ₱175,000 (carryforward)
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":               "80000000",
			"cost_of_sales":              "70000000",
			"other_income":               "0",
			"deduction_method":           "itemized",
			"itemized_deductions":        "9900000",
			"creditable_withholding_tax": "0",
			"quarterly_payments":         "0",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	assert.Equal(t, "10000000", result["gross_profit"])
	assert.Equal(t, "100000", result["net_taxable_income"])
	assert.Equal(t, "25000", result["rcit_amount"])
	assert.Equal(t, "200000", result["mcit_amount"])           // 10M × 2%
	assert.Equal(t, "200000", result["income_tax_due"])        // MCIT wins
	assert.Equal(t, "175000", result["excess_mcit_current"])   // 200K - 25K
}

func TestIndustry_Construction_BIR2307_SubcontractorWHT(t *testing.T) {
	// Withholding certificate for subcontractor:
	// Subcon TIN: 123-456-789-000
	// ATC: WI050 (Contractor - individual, 2%)
	// Income: ₱500,000
	// Tax withheld: ₱500,000 × 0.02 = ₱10,000
	input := map[string]interface{}{
		"certificate_data": map[string]interface{}{
			"payee_tin":  "123-456-789-000",
			"payee_name": "Juan dela Cruz Construction",
			"payer_tin":  "987-654-321-000",
			"payer_name": "ABC Construction Corp",
			"period":     "2025-01",
			"quarter":    "Q1",
			"items": []interface{}{
				map[string]interface{}{
					"atc_code":      "WI158",
					"income_amount": "500000",
					"tax_rate":      "0.02",
				},
				map[string]interface{}{
					"atc_code":      "WI100",
					"income_amount": "300000",
					"tax_rate":      "0.05",
				},
			},
		},
	}
	result, err := CalculateBIR2307(input)
	require.NoError(t, err)

	assert.Equal(t, "2", result["total_items"])
	assert.Equal(t, "WI158", result["item_1_atc_code"])
	assert.Equal(t, "500000", result["item_1_income_amount"])
	assert.Equal(t, "10000", result["item_1_tax_withheld"]) // 500K × 2%

	assert.Equal(t, "WI100", result["item_2_atc_code"])
	assert.Equal(t, "300000", result["item_2_income_amount"])
	assert.Equal(t, "15000", result["item_2_tax_withheld"]) // 300K × 5%

	assert.Equal(t, "800000", result["total_income_amount"])
	assert.Equal(t, "25000", result["total_tax_withheld"])
}

// ============================================================
// INDUSTRY 4: Medical Doctor (Solo Practice)
// Self-employed professional, ₱3M annual gross
// Forms: 1701, 2316 (from hospital employment)
// ============================================================

func TestIndustry_MedicalDoctor_AnnualITR_OSD(t *testing.T) {
	// Doctor with clinic income + hospital compensation:
	// Clinic gross receipts: ₱3,000,000
	// Cost of services (medicines, supplies): ₱400,000
	// Other income (speaking fees): ₱200,000
	// Uses OSD (40% of gross sales/receipts)
	// OSD = 3M × 40% = ₱1,200,000
	// Net taxable = (3M - 400K + 200K) - 1.2M = 1,600,000
	// Tax due (TRAIN): bracket 800K-2M, base = 102,500 + (1.6M - 800K) × 25% = 302,500
	// CWT from hospital: ₱150,000
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts":       "3000000",
			"cost_of_sales":              "400000",
			"other_taxable_income":       "200000",
			"deduction_method":           "osd",
			"creditable_withholding_tax": "150000",
			"quarterly_payments":         "100000",
		},
	}
	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	assert.Equal(t, "2600000", result["gross_income_from_business"]) // 3M - 400K
	assert.Equal(t, "2800000", result["total_gross_income"])         // 2.6M + 200K
	assert.Equal(t, "1200000", result["osd_amount"])                 // 3M × 40%
	assert.Equal(t, "1600000", result["net_taxable_income"])         // 2.8M - 1.2M

	// Tax = 102,500 + (1,600,000 - 800,000) × 25% = 102,500 + 200,000 = 302,500
	assert.Equal(t, "302500", result["income_tax_due"])

	// Total credits = 150K + 100K = 250K
	assert.Equal(t, "250000", result["total_tax_credits"])
	// Payable = 302,500 - 250,000 = 52,500
	assert.Equal(t, "52500", result["tax_payable"])
}

func TestIndustry_MedicalDoctor_AnnualITR_Itemized(t *testing.T) {
	// Same doctor, itemized deductions instead:
	// Clinic rent: ₱360,000, Staff salary: ₱480,000, Supplies: ₱200,000
	// Professional development: ₱100,000, Insurance: ₱60,000
	// Total itemized: ₱1,200,000
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts":       "3000000",
			"cost_of_sales":              "400000",
			"other_taxable_income":       "200000",
			"deduction_method":           "itemized",
			"itemized_deductions":        "1200000",
			"creditable_withholding_tax": "150000",
			"quarterly_payments":         "100000",
		},
	}
	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	assert.Equal(t, "1200000", result["total_deductions"])
	assert.Equal(t, "1600000", result["net_taxable_income"]) // 2.8M - 1.2M
	assert.Equal(t, "302500", result["income_tax_due"])       // Same as OSD scenario
}

func TestIndustry_MedicalDoctor_BIR2316_HospitalComp(t *testing.T) {
	// Doctor also employed as hospital consultant:
	// Hospital compensation: ₱1,200,000/year
	// Non-taxable: SSS/PhilHealth/PagIBIG ₱30,000
	// Tax withheld by hospital: ₱85,000
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"present_employer_compensation": "1200000",
			"present_employer_nontaxable":   "30000",
			"tax_withheld_present":          "85000",
		},
	}
	result, err := CalculateBIR2316(input)
	require.NoError(t, err)

	assert.Equal(t, "1170000", result["present_employer_taxable"])  // 1.2M - 30K
	assert.Equal(t, "1170000", result["total_taxable_compensation"])
	// TRAIN tax on 1,170,000: bracket 800K-2M → 102,500 + (1,170K - 800K) × 25% = 195,000
	assert.Equal(t, "195000", result["tax_due"])
	assert.Equal(t, "85000", result["total_tax_withheld"])
	// Amount still due: 195K - 85K = 110,000
	assert.Equal(t, "110000", result["amount_still_due"])
	assert.Equal(t, "0", result["amount_refunded"])
}

// ============================================================
// INDUSTRY 5: Restaurant Chain (Food Service)
// 3 branches, mix of dine-in (vatable) and delivery (vatable)
// Forms: 2550M, 1601C, 0619E, 1702
// ============================================================

func TestIndustry_Restaurant_MonthlyVAT_MultiBranch(t *testing.T) {
	// 3 branches combined monthly:
	// - Dine-in sales: ₱2,500,000 (vatable)
	// - Delivery/take-out: ₱1,200,000 (vatable)
	// - Catering to government: ₱800,000 (govt 5%)
	// Purchases: food ingredients ₱1.5M (VAT ₱180K), packaging ₱200K (VAT ₱24K),
	//            utilities ₱150K (VAT ₱18K), kitchen equipment ₱300K (VAT ₱36K)
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "2500000", "vat_type": "vatable"},
			map[string]interface{}{"amount": "1200000", "vat_type": "vatable"},
			map[string]interface{}{"amount": "800000", "vat_type": "government"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "1500000", "vat_amount": "180000", "category": "goods"},
			map[string]interface{}{"amount": "200000", "vat_amount": "24000", "category": "goods"},
			map[string]interface{}{"amount": "150000", "vat_amount": "18000", "category": "services"},
			map[string]interface{}{"amount": "300000", "vat_amount": "36000", "category": "capital"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	// Vatable = 2.5M + 1.2M = 3,700,000
	assert.Equal(t, "3700000", result["line_1_vatable_sales"])
	assert.Equal(t, "800000", result["line_2_sales_to_government"])
	assert.Equal(t, "4500000", result["line_5_total_sales"])

	// Output VAT: 3.7M × 12% = 444K, Govt: 800K × 5% = 40K
	assert.Equal(t, "444000", result["line_6_output_vat"])
	assert.Equal(t, "40000", result["line_6a_output_vat_government"])
	assert.Equal(t, "484000", result["line_6b_total_output_vat"])

	// Input VAT: goods 204K, capital 36K, services 18K = 258K
	assert.Equal(t, "204000", result["line_7_input_vat_goods"])
	assert.Equal(t, "36000", result["line_8_input_vat_capital"])
	assert.Equal(t, "18000", result["line_9_input_vat_services"])
	assert.Equal(t, "258000", result["line_11_total_input_vat"])

	// VAT payable = 484K - 258K = 226K
	assert.Equal(t, "226000", result["line_12_vat_payable"])
}

// ============================================================
// INDUSTRY 6: Law Firm (Professional Services)
// Partnership with 5 partners, individual ITR
// Forms: 1701 (each partner), 2307
// ============================================================

func TestIndustry_LawFirm_PartnerITR_HighIncome(t *testing.T) {
	// Senior partner's share of firm income:
	// Gross receipts from legal fees: ₱12,000,000
	// Cost (paralegal, research): ₱2,000,000
	// OSD = 12M × 40% = ₱4,800,000
	// Gross income from business = 12M - 2M = 10M
	// Total gross = 10M
	// Net taxable = 10M - 4.8M = 5,200,000
	// Tax (TRAIN): bracket 2M-8M → 402,500 + (5.2M - 2M) × 30% = 1,362,500
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts":       "12000000",
			"cost_of_sales":              "2000000",
			"other_taxable_income":       "0",
			"deduction_method":           "osd",
			"creditable_withholding_tax": "600000",
			"quarterly_payments":         "500000",
		},
	}
	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	assert.Equal(t, "10000000", result["gross_income_from_business"])
	assert.Equal(t, "4800000", result["osd_amount"])           // 12M × 40%
	assert.Equal(t, "5200000", result["net_taxable_income"])   // 10M - 4.8M

	// Tax: 402,500 + (5,200,000 - 2,000,000) × 30% = 402,500 + 960,000 = 1,362,500
	assert.Equal(t, "1362500", result["income_tax_due"])

	// Credits: 600K + 500K = 1.1M
	assert.Equal(t, "1100000", result["total_tax_credits"])
	// Payable: 1,362,500 - 1,100,000 = 262,500
	assert.Equal(t, "262500", result["tax_payable"])
}

func TestIndustry_LawFirm_JuniorPartner_BelowThreshold(t *testing.T) {
	// Junior associate (not yet partner, low income):
	// Gross receipts: ₱400,000
	// No COGS
	// OSD = 400K × 40% = 160K
	// Net taxable = 400K - 160K = 240,000
	// TRAIN: 240K < 250K → tax = 0
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts": "400000",
			"deduction_method":     "osd",
		},
	}
	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	assert.Equal(t, "160000", result["osd_amount"])
	assert.Equal(t, "240000", result["net_taxable_income"])
	assert.Equal(t, "0", result["income_tax_due"]) // Below 250K threshold
}

// ============================================================
// INDUSTRY 7: Manufacturing Company (Heavy Industry)
// Forms: 2550M (mixed), 1702 with excess MCIT, SAWT
// ============================================================

func TestIndustry_Manufacturing_AnnualCorp_ExcessMCIT_Carryforward(t *testing.T) {
	// Manufacturing with high COGS, loss year with MCIT carryforward:
	// Revenue: ₱50,000,000
	// COGS: ₱45,000,000
	// GP: ₱5,000,000
	// Deductions: ₱4,950,000
	// Net taxable: ₱50,000
	// RCIT: 50K × 25% = ₱12,500
	// MCIT: 5M × 2% = ₱100,000 > RCIT
	// Excess MCIT from prior years: ₱30,000
	// Since MCIT > RCIT, prior excess MCIT cannot be used this year
	// Tax due = ₱100,000 (MCIT)
	// New excess MCIT = 100K - 12.5K = ₱87,500
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":               "50000000",
			"cost_of_sales":              "45000000",
			"other_income":               "0",
			"deduction_method":           "itemized",
			"itemized_deductions":        "4950000",
			"excess_mcit_prior":          "30000",
			"creditable_withholding_tax": "25000",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	assert.Equal(t, "5000000", result["gross_profit"])
	assert.Equal(t, "50000", result["net_taxable_income"])
	assert.Equal(t, "12500", result["rcit_amount"])
	assert.Equal(t, "100000", result["mcit_amount"])           // 5M × 2%
	assert.Equal(t, "100000", result["income_tax_due"])        // MCIT wins
	assert.Equal(t, "87500", result["excess_mcit_current"])    // 100K - 12.5K
	// When MCIT wins, prior excess MCIT is NOT applied
	assert.Equal(t, "30000", result["excess_mcit_prior"])
	// Tax payable = 100K - 25K CWT = 75,000
	assert.Equal(t, "75000", result["tax_payable"])
}

func TestIndustry_Manufacturing_AnnualCorp_RCIT_WithPriorMCIT(t *testing.T) {
	// Good year: RCIT > MCIT, can use prior excess MCIT
	// Revenue: ₱50,000,000
	// COGS: ₱35,000,000
	// GP: ₱15,000,000
	// Deductions: ₱10,000,000
	// Net taxable: ₱5,000,000
	// RCIT: 5M × 25% = ₱1,250,000
	// MCIT: 15M × 2% = ₱300,000
	// RCIT > MCIT, so RCIT applies
	// Prior excess MCIT: ₱100,000 (creditable against RCIT)
	// Tax due = 1,250,000 - 100,000 = 1,150,000
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":               "50000000",
			"cost_of_sales":              "35000000",
			"other_income":               "0",
			"deduction_method":           "itemized",
			"itemized_deductions":        "10000000",
			"excess_mcit_prior":          "100000",
			"creditable_withholding_tax": "250000",
			"quarterly_payments":         "700000",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	assert.Equal(t, "15000000", result["gross_profit"])
	assert.Equal(t, "5000000", result["net_taxable_income"])
	assert.Equal(t, "1250000", result["rcit_amount"])
	assert.Equal(t, "300000", result["mcit_amount"])  // 15M × 2%
	// Tax due = RCIT - prior excess MCIT = 1,250K - 100K = 1,150K
	assert.Equal(t, "1150000", result["income_tax_due"])
	assert.Equal(t, "0", result["excess_mcit_current"]) // No new excess
	// Payable = 1,150K - 250K CWT - 700K quarterly = 200K
	assert.Equal(t, "200000", result["tax_payable"])
}

func TestIndustry_Manufacturing_SAWT(t *testing.T) {
	// Summary alphalist for Q1 withholding:
	// 3 suppliers with different ATC codes
	input := map[string]interface{}{
		"period": "2025Q1",
		"entries": []interface{}{
			map[string]interface{}{
				"tin":             "111-222-333-000",
				"registered_name": "Raw Materials Corp",
				"atc_code":        "WI157",
				"income_payment":  "2000000",
				"tax_withheld":    "20000",
			},
			map[string]interface{}{
				"tin":             "444-555-666-000",
				"registered_name": "Equipment Rental Inc",
				"atc_code":        "WI120",
				"income_payment":  "500000",
				"tax_withheld":    "25000",
			},
			map[string]interface{}{
				"tin":             "777-888-999-000",
				"registered_name": "Atty. Maria Santos",
				"atc_code":        "WI010",
				"income_payment":  "300000",
				"tax_withheld":    "15000",
			},
		},
	}
	result, err := CalculateSAWT(input)
	require.NoError(t, err)

	assert.Equal(t, "2025Q1", result["period"])
	assert.Equal(t, "3", result["total_entries"])
	assert.Equal(t, "2800000", result["total_income_payment"]) // 2M + 500K + 300K
	assert.Equal(t, "60000", result["total_tax_withheld"])     // 20K + 25K + 15K

	// Verify individual entries
	assert.Equal(t, "111-222-333-000", result["entry_1_tin"])
	assert.Equal(t, "WI157", result["entry_1_atc_code"])
	assert.Equal(t, "WI010", result["entry_3_atc_code"])
}

// ============================================================
// INDUSTRY 8: Real Estate Developer
// Mix of VAT-exempt (socialized housing) and vatable
// Forms: 2550M, 1702 (OSD corporate)
// ============================================================

func TestIndustry_RealEstate_MonthlyVAT_MixedHousing(t *testing.T) {
	// Sales:
	// - Socialized housing (≤₱450K per unit, VAT exempt): ₱4,500,000 (10 units)
	// - Economic housing (₱450K-₱2M, vatable): ₱8,000,000 (5 units)
	// - Commercial lots (vatable): ₱15,000,000
	// Purchases: construction materials ₱8M (VAT ₱960K), services ₱2M (VAT ₱240K)
	input := map[string]interface{}{
		"sales_data": []interface{}{
			map[string]interface{}{"amount": "4500000", "vat_type": "exempt"},
			map[string]interface{}{"amount": "8000000", "vat_type": "vatable"},
			map[string]interface{}{"amount": "15000000", "vat_type": "vatable"},
		},
		"purchases_data": []interface{}{
			map[string]interface{}{"amount": "8000000", "vat_amount": "960000", "category": "goods"},
			map[string]interface{}{"amount": "2000000", "vat_amount": "240000", "category": "services"},
		},
	}
	result, err := CalculateBIR2550M(input)
	require.NoError(t, err)

	assert.Equal(t, "23000000", result["line_1_vatable_sales"])   // 8M + 15M
	assert.Equal(t, "4500000", result["line_4_exempt_sales"])
	assert.Equal(t, "27500000", result["line_5_total_sales"])     // 23M + 4.5M

	// Output VAT = 23M × 12% = 2,760,000
	assert.Equal(t, "2760000", result["line_6_output_vat"])

	// Input VAT = 960K + 240K = 1,200,000
	assert.Equal(t, "1200000", result["line_11_total_input_vat"])

	// VAT payable = 2,760K - 1,200K = 1,560,000
	assert.Equal(t, "1560000", result["line_12_vat_payable"])
}

func TestIndustry_RealEstate_AnnualCorp_OSD(t *testing.T) {
	// Real estate company using OSD:
	// Revenue: ₱200,000,000
	// COGS: ₱130,000,000
	// GP: ₱70,000,000
	// Corporate OSD = 40% of gross income (= grossProfit per NIRC Sec 34(L))
	// OSD = 70M × 40% = ₱28,000,000
	// Net taxable = 70M - 28M = 42,000,000
	// RCIT: 42M × 25% = ₱10,500,000
	// MCIT: 70M × 2% = ₱1,400,000
	// RCIT > MCIT → tax due = RCIT
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":     "200000000",
			"cost_of_sales":    "130000000",
			"other_income":     "0",
			"deduction_method": "osd",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	assert.Equal(t, "70000000", result["gross_profit"])
	assert.Equal(t, "28000000", result["osd_amount"])          // 70M × 40% (GP-based OSD)
	assert.Equal(t, "42000000", result["net_taxable_income"])  // 70M - 28M
	assert.Equal(t, "10500000", result["rcit_amount"])         // 42M × 25%
	assert.Equal(t, "1400000", result["mcit_amount"])          // 70M × 2%
	assert.Equal(t, "10500000", result["income_tax_due"])      // RCIT wins
}

// ============================================================
// INDUSTRY 9: Freelance Digital Artist (Gig Economy)
// Very low income, below TRAIN threshold
// Forms: 1701
// ============================================================

func TestIndustry_Freelancer_BelowTRAINThreshold(t *testing.T) {
	// Young freelancer, just starting:
	// Gross receipts: ₱350,000
	// No COGS (digital services)
	// OSD = 350K × 40% = 140K
	// Net taxable = 350K - 140K = 210,000
	// TRAIN: 210K < 250K → tax = 0
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts": "350000",
			"deduction_method":     "osd",
		},
	}
	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	assert.Equal(t, "350000", result["gross_income_from_business"])
	assert.Equal(t, "140000", result["osd_amount"])
	assert.Equal(t, "210000", result["net_taxable_income"])
	assert.Equal(t, "0", result["income_tax_due"]) // Below 250K
	assert.Equal(t, "0", result["tax_payable"])
}

// ============================================================
// INDUSTRY 10: Accounting Firm (Multi-Employee WHT)
// Multiple employees with varying compensation
// Forms: 2316 (per employee), 1601C
// ============================================================

func TestIndustry_AccountingFirm_BIR2316_SeniorManager(t *testing.T) {
	// Senior manager with previous employer (transferred mid-year):
	// Present employer: ₱1,800,000 compensation, ₱50,000 nontaxable
	// Previous employer: ₱600,000, ₱15,000 nontaxable
	// Total taxable: (1.8M - 50K) + (600K - 15K) = 2,335,000
	// Tax (TRAIN): bracket 2M-8M → 402,500 + (2.335M - 2M) × 30% = 502,000
	// Tax withheld present: ₱350,000, previous: ₱50,000
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"present_employer_compensation":  "1800000",
			"present_employer_nontaxable":    "50000",
			"previous_employer_compensation": "600000",
			"previous_employer_nontaxable":   "15000",
			"tax_withheld_present":           "350000",
			"tax_withheld_previous":          "50000",
		},
	}
	result, err := CalculateBIR2316(input)
	require.NoError(t, err)

	assert.Equal(t, "1750000", result["present_employer_taxable"])  // 1.8M - 50K
	assert.Equal(t, "585000", result["previous_employer_taxable"])  // 600K - 15K
	assert.Equal(t, "2335000", result["total_taxable_compensation"])

	// Tax: 402,500 + (2,335,000 - 2,000,000) × 30% = 402,500 + 100,500 = 503,000
	assert.Equal(t, "503000", result["tax_due"])

	// Withheld: 350K + 50K = 400K
	assert.Equal(t, "400000", result["total_tax_withheld"])

	// Still due: 503K - 400K = 103,000
	assert.Equal(t, "103000", result["amount_still_due"])
}

func TestIndustry_AccountingFirm_BIR2316_MWE(t *testing.T) {
	// Office helper (MWE - Minimum Wage Earner):
	// Compensation: ₱396,000/year (₱33K × 12)
	// All nontaxable (MWE exempt): ₱396,000
	// Taxable: 0
	// Tax due: 0
	input := map[string]interface{}{
		"compensation_data": map[string]interface{}{
			"present_employer_compensation": "396000",
			"present_employer_nontaxable":   "396000",
			"tax_withheld_present":          "0",
		},
	}
	result, err := CalculateBIR2316(input)
	require.NoError(t, err)

	assert.Equal(t, "0", result["total_taxable_compensation"])
	assert.Equal(t, "0", result["tax_due"])
	assert.Equal(t, "0", result["amount_still_due"])
	assert.Equal(t, "0", result["amount_refunded"])
}

// ============================================================
// EDGE CASES: Cross-Industry Validation
// ============================================================

func TestEdge_ZeroRevenue_AllForms(t *testing.T) {
	// Company with zero revenue (pre-operations)
	tests := []struct {
		name     string
		formType string
		input    map[string]interface{}
	}{
		{"2550M zero", birforms.FormBIR2550M, map[string]interface{}{}},
		{"1601C zero", birforms.FormBIR1601C, map[string]interface{}{}},
		{"0619E zero", birforms.FormBIR0619E, map[string]interface{}{}},
		{"1701 zero", birforms.FormBIR1701, map[string]interface{}{}},
		{"1702 zero", birforms.FormBIR1702, map[string]interface{}{}},
		{"2316 zero", birforms.FormBIR2316, map[string]interface{}{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CalculateReport(tt.formType, tt.input)
			require.NoError(t, err)
			require.NotNil(t, result)
		})
	}
}

func TestEdge_VeryHighIncome_TopBracket(t *testing.T) {
	// Extremely wealthy individual: ₱50M income
	// Tax: 2,202,500 + (50M - 8M) × 35% = 2,202,500 + 14,700,000 = 16,902,500
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_sales_receipts": "50000000",
			"deduction_method":     "osd",
		},
	}
	result, err := CalculateBIR1701(input)
	require.NoError(t, err)

	// OSD = 50M × 40% = 20M
	// Net taxable = 50M - 20M = 30M
	assert.Equal(t, "30000000", result["net_taxable_income"])
	// Tax: 2,202,500 + (30M - 8M) × 35% = 2,202,500 + 7,700,000 = 9,902,500
	assert.Equal(t, "9902500", result["income_tax_due"])
}

func TestEdge_LossMaking_Corporation(t *testing.T) {
	// Revenue < COGS → gross profit = 0
	// Revenue: ₱10,000,000
	// COGS: ₱12,000,000 (overrun)
	// GP: max(10M - 12M, 0) = 0
	// Net taxable: 0
	// RCIT: 0
	// MCIT: 0 × 2% = 0
	input := map[string]interface{}{
		"income_data": map[string]interface{}{
			"gross_income":        "10000000",
			"cost_of_sales":       "12000000",
			"deduction_method":    "itemized",
			"itemized_deductions": "500000",
		},
	}
	result, err := CalculateBIR1702(input)
	require.NoError(t, err)

	assert.Equal(t, "0", result["gross_profit"])
	assert.Equal(t, "0", result["net_taxable_income"])
	assert.Equal(t, "0", result["rcit_amount"])
	assert.Equal(t, "0", result["mcit_amount"])
	assert.Equal(t, "0", result["income_tax_due"])
}

func TestEdge_ExactBracketBoundaries_TRAIN(t *testing.T) {
	// Test exact bracket boundaries for TRAIN tax calculation
	tests := []struct {
		name     string
		taxable  string
		expected string
	}{
		// Exactly at 250K boundary → 0
		{"250K", "250000", "0"},
		// Exactly at 400K → 0 + (400K - 250K) × 15% = 22,500
		{"400K", "400000", "22500"},
		// Exactly at 800K → 22,500 + (800K - 400K) × 20% = 102,500
		{"800K", "800000", "102500"},
		// Exactly at 2M → 102,500 + (2M - 800K) × 25% = 402,500
		{"2M", "2000000", "402500"},
		// Exactly at 8M → 402,500 + (8M - 2M) × 30% = 2,202,500
		{"8M", "8000000", "2202500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taxable := decimal.RequireFromString(tt.taxable)
			tax := computeGraduatedTax(taxable)
			assert.Equal(t, tt.expected, tax.String(),
				"Tax on %s should be %s", tt.taxable, tt.expected)
		})
	}
}

// ============================================================
// COMPLIANCE RULE SIMULATION: Run compliance on industry data
// ============================================================

func TestCompliance_ConvenienceStore_VATReport(t *testing.T) {
	// Simulate compliance check on convenience store VAT data
	data := map[string]interface{}{
		"vatable_sales":        680000.0,
		"sales_to_government":  0.0,
		"zero_rated_sales":     0.0,
		"vat_exempt_sales":     120000.0,
		"total_sales":          800000.0,
		"output_vat":           81600.0,
		"total_input_vat":      55800.0,
		"total_amount_due":     25800.0,
		"tin_number":           "001-234-567-000",
		"period":               "2030-06",
	}

	results := RunAllChecks(data, birforms.FormBIR2550M, nil, nil)

	// Count failures
	var criticalFails, highFails int
	for _, r := range results {
		if !r.Passed {
			switch r.Severity {
			case "critical":
				criticalFails++
			case "high":
				highFails++
			}
		}
	}

	// Should have no critical failures
	assert.Equal(t, 0, criticalFails, "No critical compliance failures expected")
}

func TestCompliance_BPO_ZeroRatedExporter(t *testing.T) {
	// BPO with mostly zero-rated sales - VAT refund scenario
	data := map[string]interface{}{
		"vatable_sales":        200000.0,
		"sales_to_government":  0.0,
		"zero_rated_sales":     5000000.0,
		"vat_exempt_sales":     0.0,
		"total_sales":          5200000.0,
		"output_vat":           24000.0,
		"total_input_vat":      108000.0,
		"total_amount_due":     0.0,
		"tin_number":           "100-200-300-000",
		"period":               "2030-03",
	}

	results := RunAllChecks(data, birforms.FormBIR2550M, nil, nil)

	var criticalFails int
	for _, r := range results {
		if !r.Passed && r.Severity == "critical" {
			criticalFails++
			t.Logf("Critical fail: %s - %s", r.CheckID, r.Message)
		}
	}
	assert.Equal(t, 0, criticalFails, "BPO zero-rated should pass critical compliance")
}

// ============================================================
// DISPATCHER VALIDATION: All 9 forms route correctly
// ============================================================

func TestDispatcher_AllPHForms_NoError(t *testing.T) {
	forms := []string{
		birforms.FormBIR2550M, birforms.FormBIR2550Q,
		birforms.FormBIR1601C, birforms.FormBIR0619E,
		birforms.FormBIR1701, birforms.FormBIR1702,
		birforms.FormBIR2316, birforms.FormBIR2307,
		birforms.FormSAWT,
	}

	for _, form := range forms {
		t.Run(form, func(t *testing.T) {
			result, err := CalculateReport(form, map[string]interface{}{})
			require.NoError(t, err, "Form %s should not error with empty input", form)
			require.NotNil(t, result)
		})
	}
}
