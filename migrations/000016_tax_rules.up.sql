-- ============================================================
-- Tax Rules (versioned rates, penalties, thresholds, deadlines)
-- ============================================================
CREATE TABLE IF NOT EXISTS tax_rules (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    rule_type      VARCHAR(50) NOT NULL,   -- 'rate', 'penalty', 'threshold', 'deadline'
    rule_key       VARCHAR(100) NOT NULL,  -- e.g. 'VAT_RATE', 'EWT_WI010', 'LATE_FILING_PENALTY'
    value          JSONB NOT NULL,         -- {rate: 0.12} or {base: 1000, per_day: 25, max: 25000}
    effective_from DATE NOT NULL,
    effective_to   DATE,                   -- NULL = currently active
    source_ref     VARCHAR(200),           -- e.g. "NIRC Sec 248", "RA 11976"
    description    TEXT,
    created_at     TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tax_rules_active
  ON tax_rules (rule_type, rule_key, effective_from);
CREATE INDEX IF NOT EXISTS idx_tax_rules_type ON tax_rules (rule_type);

-- ============================================================
-- Seed: VAT Rates
-- ============================================================
INSERT INTO tax_rules (id, rule_type, rule_key, value, effective_from, source_ref, description) VALUES
  (uuid_generate_v4(), 'rate', 'VAT_RATE',
   '{"rate": 0.12}', '2006-02-01', 'NIRC Sec 106', 'Standard VAT rate'),
  (uuid_generate_v4(), 'rate', 'GOVT_VAT_RATE',
   '{"rate": 0.05}', '2006-02-01', 'NIRC Sec 114(C)', 'Government VAT withholding rate');

-- ============================================================
-- Seed: Corporate Income Tax
-- ============================================================
INSERT INTO tax_rules (id, rule_type, rule_key, value, effective_from, source_ref, description) VALUES
  (uuid_generate_v4(), 'rate', 'RCIT',
   '{"rate": 0.25}', '2020-07-01', 'RA 11534 (CREATE)', 'Regular corporate income tax rate'),
  (uuid_generate_v4(), 'rate', 'RCIT_REDUCED',
   '{"rate": 0.20, "conditions": {"max_net_taxable": 5000000, "max_total_assets": 100000000}}',
   '2020-07-01', 'RA 11534 (CREATE)', 'Reduced RCIT for qualifying SMEs'),
  (uuid_generate_v4(), 'rate', 'MCIT',
   '{"rate": 0.01}', '2023-07-01', 'RA 11534 (CREATE)', 'Minimum corporate income tax (1% until June 2023 was 2%)');

-- ============================================================
-- Seed: TRAIN Progressive Tax Brackets (Effective 2023+)
-- ============================================================
INSERT INTO tax_rules (id, rule_type, rule_key, value, effective_from, source_ref, description) VALUES
  (uuid_generate_v4(), 'rate', 'TRAIN_BRACKET_1',
   '{"over": 0, "not_over": 250000, "base_tax": 0, "rate": 0}',
   '2023-01-01', 'RA 10963 (TRAIN) Sec 24(A)(2)', 'Exempt: 0-250K'),
  (uuid_generate_v4(), 'rate', 'TRAIN_BRACKET_2',
   '{"over": 250000, "not_over": 400000, "base_tax": 0, "rate": 0.15}',
   '2023-01-01', 'RA 10963 (TRAIN) Sec 24(A)(2)', '15% of excess over 250K'),
  (uuid_generate_v4(), 'rate', 'TRAIN_BRACKET_3',
   '{"over": 400000, "not_over": 800000, "base_tax": 22500, "rate": 0.20}',
   '2023-01-01', 'RA 10963 (TRAIN) Sec 24(A)(2)', '22,500 + 20% of excess over 400K'),
  (uuid_generate_v4(), 'rate', 'TRAIN_BRACKET_4',
   '{"over": 800000, "not_over": 2000000, "base_tax": 102500, "rate": 0.25}',
   '2023-01-01', 'RA 10963 (TRAIN) Sec 24(A)(2)', '102,500 + 25% of excess over 800K'),
  (uuid_generate_v4(), 'rate', 'TRAIN_BRACKET_5',
   '{"over": 2000000, "not_over": 8000000, "base_tax": 402500, "rate": 0.30}',
   '2023-01-01', 'RA 10963 (TRAIN) Sec 24(A)(2)', '402,500 + 30% of excess over 2M'),
  (uuid_generate_v4(), 'rate', 'TRAIN_BRACKET_6',
   '{"over": 8000000, "not_over": 0, "base_tax": 2202500, "rate": 0.35}',
   '2023-01-01', 'RA 10963 (TRAIN) Sec 24(A)(2)', '2,202,500 + 35% of excess over 8M');

-- ============================================================
-- Seed: EWT ATC Codes
-- ============================================================
INSERT INTO tax_rules (id, rule_type, rule_key, value, effective_from, source_ref, description) VALUES
  (uuid_generate_v4(), 'rate', 'EWT_WI010', '{"rate": 0.05, "category": "services", "keywords": ["professional","consultant","individual"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Professional fees - Individual <3M'),
  (uuid_generate_v4(), 'rate', 'EWT_WI020', '{"rate": 0.10, "category": "services", "keywords": ["professional","consultant","individual","high value"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Professional fees - Individual >=3M'),
  (uuid_generate_v4(), 'rate', 'EWT_WC010', '{"rate": 0.10, "category": "services", "keywords": ["professional","consultant","corporation","corporate"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Professional fees - Corporation'),
  (uuid_generate_v4(), 'rate', 'EWT_WC020', '{"rate": 0.15, "category": "services", "keywords": ["professional","consultant","corporation","high value"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Professional fees - Corp >720K'),
  (uuid_generate_v4(), 'rate', 'EWT_WI030', '{"rate": 0.05, "category": "services", "keywords": ["rent","lease","real property","office","space"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Rent - Real property'),
  (uuid_generate_v4(), 'rate', 'EWT_WI040', '{"rate": 0.05, "category": "services", "keywords": ["rent","lease","equipment","vehicle"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Rent - Personal property'),
  (uuid_generate_v4(), 'rate', 'EWT_WI050', '{"rate": 0.02, "category": "services", "keywords": ["contractor","subcontractor","individual","construction"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Contractors - Individual'),
  (uuid_generate_v4(), 'rate', 'EWT_WC050', '{"rate": 0.02, "category": "services", "keywords": ["contractor","subcontractor","corporation","construction"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Contractors - Corporation'),
  (uuid_generate_v4(), 'rate', 'EWT_WC060', '{"rate": 0.02, "category": "services", "keywords": ["advertising","promotion","marketing","media"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Advertising/Promotions'),
  (uuid_generate_v4(), 'rate', 'EWT_WI070', '{"rate": 0.10, "category": "services", "keywords": ["commission","agent","broker","individual"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Commission - Individual'),
  (uuid_generate_v4(), 'rate', 'EWT_WC070', '{"rate": 0.10, "category": "services", "keywords": ["commission","agent","broker","corporation"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Commission - Corporation'),
  (uuid_generate_v4(), 'rate', 'EWT_WI100', '{"rate": 0.01, "category": "goods", "keywords": ["purchase","goods","supplies","individual"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Purchase of goods - Individual >3M'),
  (uuid_generate_v4(), 'rate', 'EWT_WC100', '{"rate": 0.01, "category": "goods", "keywords": ["purchase","goods","supplies","corporation"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Purchase of goods - Corporation >3M'),
  (uuid_generate_v4(), 'rate', 'EWT_WI120', '{"rate": 0.02, "category": "services", "keywords": ["service","payment","individual"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Service payments - Individual'),
  (uuid_generate_v4(), 'rate', 'EWT_WC120', '{"rate": 0.02, "category": "services", "keywords": ["service","payment","corporation"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Service payments - Corporation'),
  (uuid_generate_v4(), 'rate', 'EWT_WI150', '{"rate": 0.02, "category": "services", "keywords": ["transport","delivery","freight","shipping","logistics"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Transport/Delivery/Freight'),
  (uuid_generate_v4(), 'rate', 'EWT_WI160', '{"rate": 0.01, "category": "services", "keywords": ["toll","expressway","highway"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Toll fees'),
  (uuid_generate_v4(), 'rate', 'EWT_WI170', '{"rate": 0.02, "category": "services", "keywords": ["insurance","premium","coverage"]}', '2018-01-01', 'RR 2-98 as amended by TRAIN', 'Insurance premiums');

-- ============================================================
-- Seed: Penalties (NIRC Sec 248-249, EOPT Act RA 11976)
-- ============================================================
INSERT INTO tax_rules (id, rule_type, rule_key, value, effective_from, source_ref, description) VALUES
  (uuid_generate_v4(), 'penalty', 'LATE_FILING_SURCHARGE',
   '{"rate": 0.25}', '1997-01-01', 'NIRC Sec 248(A)', '25% surcharge for late filing/payment'),
  (uuid_generate_v4(), 'penalty', 'FRAUD_SURCHARGE',
   '{"rate": 0.50}', '1997-01-01', 'NIRC Sec 248(B)', '50% surcharge for willful neglect/fraud'),
  (uuid_generate_v4(), 'penalty', 'INTEREST_RATE',
   '{"rate": 0.12}', '1997-01-01', 'NIRC Sec 249', 'Interest rate per annum (general)'),
  (uuid_generate_v4(), 'penalty', 'INTEREST_RATE_EOPT',
   '{"rate": 0.06, "conditions": {"taxpayer_type": "small"}}', '2024-01-22', 'RA 11976 (EOPT) Sec 249', 'Reduced interest 6% p.a. for EOPT-qualifying taxpayers'),
  (uuid_generate_v4(), 'penalty', 'COMPROMISE_LATE_FILING',
   '{"amount": 1000, "conditions": {"basic_tax_threshold": 0}}', '1997-01-01', 'RMO 7-2015', 'Compromise penalty for late filing (no tax due)'),
  (uuid_generate_v4(), 'penalty', 'COMPROMISE_LATE_PAYMENT',
   '{"tiers": [{"min": 0, "max": 5000, "penalty": 200}, {"min": 5001, "max": 20000, "penalty": 500}, {"min": 20001, "max": 50000, "penalty": 1000}, {"min": 50001, "max": 100000, "penalty": 2000}, {"min": 100001, "max": 250000, "penalty": 5000}, {"min": 250001, "max": 500000, "penalty": 10000}, {"min": 500001, "max": 0, "penalty": 25000}]}',
   '1997-01-01', 'RMO 7-2015 Annex A', 'Compromise penalty tiered by basic tax due');

-- ============================================================
-- Seed: Form Schemas (from hardcoded formSchemas map)
-- ============================================================
INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules) VALUES
  (uuid_generate_v4(), 'BIR_2550M', 1, 'Monthly Value-Added Tax Declaration', 'monthly', true,
   '{"sections":[{"id":"sales","name":"Sales/Receipts","fields":[{"id":"vatable_sales","line":"14A","label":"Vatable Sales","editable":true,"required":true},{"id":"vat_exempt_sales","line":"14B","label":"VAT-Exempt Sales","editable":true},{"id":"zero_rated_sales","line":"14C","label":"Zero-Rated Sales","editable":true}]},{"id":"purchases","name":"Purchases","fields":[{"id":"domestic_purchases","line":"18A","label":"Domestic Purchases of Goods","editable":true},{"id":"domestic_services","line":"18B","label":"Domestic Purchases of Services","editable":true},{"id":"importation","line":"18C","label":"Importation of Goods","editable":true}]},{"id":"tax","name":"Tax Computation","fields":[{"id":"output_tax","line":"15","label":"Output Tax","editable":false},{"id":"input_tax","line":"19","label":"Input Tax","editable":false},{"id":"vat_payable","line":"20","label":"VAT Payable","editable":false}]}]}',
   '{"output_tax":"vatable_sales * 0.12","input_tax":"(domestic_purchases + domestic_services + importation) * 0.12","vat_payable":"output_tax - input_tax"}')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules) VALUES
  (uuid_generate_v4(), 'BIR_2550Q', 1, 'Quarterly Value-Added Tax Return', 'quarterly', true,
   '{"sections":[{"id":"sales","name":"Sales/Receipts","fields":[{"id":"vatable_sales","line":"14A","label":"Vatable Sales","editable":true,"required":true},{"id":"vat_exempt_sales","line":"14B","label":"VAT-Exempt Sales","editable":true},{"id":"zero_rated_sales","line":"14C","label":"Zero-Rated Sales","editable":true}]},{"id":"purchases","name":"Purchases","fields":[{"id":"domestic_purchases_goods","line":"18A","label":"Domestic Purchases of Goods","editable":true},{"id":"domestic_purchases_services","line":"18B","label":"Domestic Purchases of Services","editable":true}]}]}',
   '{"output_tax":"vatable_sales * 0.12","input_tax":"(domestic_purchases_goods + domestic_purchases_services) * 0.12","vat_payable":"output_tax - input_tax"}')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules) VALUES
  (uuid_generate_v4(), 'BIR_1601C', 1, 'Monthly Remittance of Withholding Tax on Compensation', 'monthly', true,
   '{"sections":[{"id":"compensation","name":"Compensation","fields":[{"id":"total_compensation","line":"1","label":"Total Amount of Compensation","editable":true,"required":true},{"id":"non_taxable","line":"2","label":"Non-Taxable/Exempt Compensation","editable":true},{"id":"taxable_compensation","line":"3","label":"Taxable Compensation","editable":false},{"id":"tax_withheld","line":"4","label":"Tax Required to be Withheld","editable":false}]}]}',
   '{"taxable_compensation":"total_compensation - non_taxable"}')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules) VALUES
  (uuid_generate_v4(), 'BIR_0619E', 1, 'Monthly Remittance of Creditable Income Taxes Withheld (Expanded)', 'monthly', true,
   '{"sections":[{"id":"ewt","name":"Expanded Withholding Tax","fields":[{"id":"tax_base","line":"1","label":"Total Amount of Income Payments","editable":true,"required":true},{"id":"tax_rate","line":"2","label":"Applicable Tax Rate","editable":true},{"id":"tax_due","line":"3","label":"Tax Due","editable":false},{"id":"amount_remitted","line":"4","label":"Amount Remitted","editable":false}]}]}',
   '{"tax_due":"tax_base * tax_rate"}')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules) VALUES
  (uuid_generate_v4(), 'BIR_1701', 1, 'Annual Income Tax Return (Individuals)', 'annual', true,
   '{"sections":[{"id":"income","name":"Income","fields":[{"id":"gross_income","line":"1","label":"Gross Compensation Income","editable":true,"required":true},{"id":"business_income","line":"2","label":"Gross Business/Professional Income","editable":true},{"id":"total_income","line":"3","label":"Total Taxable Income","editable":false},{"id":"tax_due","line":"4","label":"Tax Due","editable":false}]}]}',
   '{"total_income":"gross_income + business_income"}')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules) VALUES
  (uuid_generate_v4(), 'BIR_1702', 1, 'Annual Income Tax Return (Corporations)', 'annual', true,
   '{"sections":[{"id":"income","name":"Income","fields":[{"id":"gross_income","line":"1","label":"Gross Income from Operations","editable":true,"required":true},{"id":"deductions","line":"2","label":"Total Allowable Deductions","editable":true},{"id":"net_taxable_income","line":"3","label":"Net Taxable Income","editable":false},{"id":"income_tax_due","line":"4","label":"Income Tax Due","editable":false}]}]}',
   '{"net_taxable_income":"gross_income - deductions"}')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules) VALUES
  (uuid_generate_v4(), 'BIR_2316', 1, 'Certificate of Compensation Payment / Tax Withheld', 'annual', true,
   '{"sections":[{"id":"compensation","name":"Compensation","fields":[{"id":"gross_compensation","line":"1","label":"Gross Compensation","editable":true,"required":true},{"id":"non_taxable_compensation","line":"2","label":"Non-Taxable Compensation","editable":true},{"id":"taxable_compensation","line":"3","label":"Taxable Compensation","editable":false},{"id":"tax_withheld","line":"4","label":"Tax Withheld","editable":false}]}]}',
   '{"taxable_compensation":"gross_compensation - non_taxable_compensation"}')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules) VALUES
  (uuid_generate_v4(), 'BIR_2307', 1, 'Certificate of Creditable Tax Withheld at Source', 'quarterly', true,
   '{"sections":[{"id":"payee","name":"Payee Information","fields":[{"id":"payee_tin","line":"1","label":"Payee TIN","editable":true,"required":true},{"id":"payee_name","line":"2","label":"Payee Name","editable":true,"required":true}]},{"id":"income","name":"Income Payments","fields":[{"id":"atc_code","line":"3","label":"ATC Code","editable":true,"required":true},{"id":"income_amount","line":"4","label":"Income Amount","editable":true,"required":true},{"id":"tax_withheld","line":"5","label":"Tax Withheld","editable":false}]}]}',
   '{}')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules) VALUES
  (uuid_generate_v4(), 'SAWT', 1, 'Summary Alphalist of Withholding Taxes', 'quarterly', true,
   '{"sections":[{"id":"summary","name":"Summary","fields":[{"id":"total_income","line":"1","label":"Total Income Payments","editable":false},{"id":"total_tax_withheld","line":"2","label":"Total Tax Withheld","editable":false}]}]}',
   '{}')
ON CONFLICT (form_type) DO NOTHING;
