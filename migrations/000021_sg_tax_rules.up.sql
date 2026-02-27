-- ============================================================
-- Seed: Singapore Tax Rules (jurisdiction = 'SG')
-- ============================================================

-- GST Rate
INSERT INTO tax_rules (id, rule_type, rule_key, value, effective_from, source_ref, description, jurisdiction) VALUES
  (uuid_generate_v4(), 'rate', 'GST_RATE',
   '{"rate": 0.09}', '2024-01-01', 'GST Act Sec 16', 'Standard GST rate (9% from 2024)', 'SG'),
  (uuid_generate_v4(), 'rate', 'GST_RATE_PREV',
   '{"rate": 0.08}', '2023-01-01', 'GST Act Sec 16', 'GST rate 8% (2023)', 'SG');

-- Corporate Income Tax
INSERT INTO tax_rules (id, rule_type, rule_key, value, effective_from, source_ref, description, jurisdiction) VALUES
  (uuid_generate_v4(), 'rate', 'CORPORATE_TAX',
   '{"rate": 0.17}', '2010-01-01', 'ITA Sec 43', 'Corporate income tax rate 17%', 'SG'),
  (uuid_generate_v4(), 'rate', 'PARTIAL_EXEMPTION',
   '{"first_10k_exempt": 0.75, "next_190k_exempt": 0.50}', '2020-01-01', 'ITA Sec 43(6A)',
   'Partial tax exemption: 75% on first S$10K, 50% on next S$190K', 'SG'),
  (uuid_generate_v4(), 'rate', 'STARTUP_EXEMPTION',
   '{"first_100k_exempt": 0.75, "next_100k_exempt": 0.50, "max_years": 3}', '2020-01-01', 'ITA Sec 43(6)',
   'Start-up tax exemption: 75% on first S$100K, 50% on next S$100K (first 3 YAs)', 'SG');

-- Individual Progressive Tax Brackets (YA 2024+)
INSERT INTO tax_rules (id, rule_type, rule_key, value, effective_from, source_ref, description, jurisdiction) VALUES
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_1',
   '{"over": 0, "not_over": 20000, "base_tax": 0, "rate": 0}',
   '2024-01-01', 'ITA Sec 42', '0% on first S$20,000', 'SG'),
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_2',
   '{"over": 20000, "not_over": 30000, "base_tax": 0, "rate": 0.02}',
   '2024-01-01', 'ITA Sec 42', '2% on S$20,001 - S$30,000', 'SG'),
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_3',
   '{"over": 30000, "not_over": 40000, "base_tax": 200, "rate": 0.035}',
   '2024-01-01', 'ITA Sec 42', '3.5% on S$30,001 - S$40,000', 'SG'),
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_4',
   '{"over": 40000, "not_over": 80000, "base_tax": 550, "rate": 0.07}',
   '2024-01-01', 'ITA Sec 42', '7% on S$40,001 - S$80,000', 'SG'),
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_5',
   '{"over": 80000, "not_over": 120000, "base_tax": 3350, "rate": 0.115}',
   '2024-01-01', 'ITA Sec 42', '11.5% on S$80,001 - S$120,000', 'SG'),
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_6',
   '{"over": 120000, "not_over": 160000, "base_tax": 7950, "rate": 0.15}',
   '2024-01-01', 'ITA Sec 42', '15% on S$120,001 - S$160,000', 'SG'),
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_7',
   '{"over": 160000, "not_over": 200000, "base_tax": 13950, "rate": 0.18}',
   '2024-01-01', 'ITA Sec 42', '18% on S$160,001 - S$200,000', 'SG'),
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_8',
   '{"over": 200000, "not_over": 240000, "base_tax": 21150, "rate": 0.19}',
   '2024-01-01', 'ITA Sec 42', '19% on S$200,001 - S$240,000', 'SG'),
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_9',
   '{"over": 240000, "not_over": 280000, "base_tax": 28750, "rate": 0.195}',
   '2024-01-01', 'ITA Sec 42', '19.5% on S$240,001 - S$280,000', 'SG'),
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_10',
   '{"over": 280000, "not_over": 320000, "base_tax": 36550, "rate": 0.20}',
   '2024-01-01', 'ITA Sec 42', '20% on S$280,001 - S$320,000', 'SG'),
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_11',
   '{"over": 320000, "not_over": 500000, "base_tax": 44550, "rate": 0.22}',
   '2024-01-01', 'ITA Sec 42', '22% on S$320,001 - S$500,000', 'SG'),
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_12',
   '{"over": 500000, "not_over": 1000000, "base_tax": 84150, "rate": 0.23}',
   '2024-01-01', 'ITA Sec 42', '23% on S$500,001 - S$1,000,000', 'SG'),
  (uuid_generate_v4(), 'rate', 'SG_BRACKET_13',
   '{"over": 1000000, "not_over": 0, "base_tax": 199150, "rate": 0.24}',
   '2024-01-01', 'ITA Sec 42', '24% on above S$1,000,000', 'SG');

-- CPF Contribution Rates
INSERT INTO tax_rules (id, rule_type, rule_key, value, effective_from, source_ref, description, jurisdiction) VALUES
  (uuid_generate_v4(), 'rate', 'CPF_BELOW_55',
   '{"employee": 0.20, "employer": 0.17, "total": 0.37, "ow_ceiling": 6800}',
   '2024-01-01', 'CPF Act Sec 7', 'CPF rates: age ≤55', 'SG'),
  (uuid_generate_v4(), 'rate', 'CPF_55_TO_60',
   '{"employee": 0.15, "employer": 0.15, "total": 0.30, "ow_ceiling": 6800}',
   '2024-01-01', 'CPF Act Sec 7', 'CPF rates: age 55-60', 'SG'),
  (uuid_generate_v4(), 'rate', 'CPF_60_TO_65',
   '{"employee": 0.095, "employer": 0.115, "total": 0.21, "ow_ceiling": 6800}',
   '2024-01-01', 'CPF Act Sec 7', 'CPF rates: age 60-65', 'SG'),
  (uuid_generate_v4(), 'rate', 'CPF_65_TO_70',
   '{"employee": 0.07, "employer": 0.09, "total": 0.16, "ow_ceiling": 6800}',
   '2024-01-01', 'CPF Act Sec 7', 'CPF rates: age 65-70', 'SG'),
  (uuid_generate_v4(), 'rate', 'CPF_ABOVE_70',
   '{"employee": 0.05, "employer": 0.075, "total": 0.125, "ow_ceiling": 6800}',
   '2024-01-01', 'CPF Act Sec 7', 'CPF rates: age >70', 'SG');

-- S45 Withholding Tax Rates
INSERT INTO tax_rules (id, rule_type, rule_key, value, effective_from, source_ref, description, jurisdiction) VALUES
  (uuid_generate_v4(), 'rate', 'WHT_INT', '{"rate": 0.15, "category": "withholding", "keywords": ["interest","loan","deposit"]}',
   '2010-01-01', 'ITA Sec 45', 'WHT on interest to non-residents', 'SG'),
  (uuid_generate_v4(), 'rate', 'WHT_ROY', '{"rate": 0.10, "category": "withholding", "keywords": ["royalty","ip","intellectual","patent","copyright"]}',
   '2010-01-01', 'ITA Sec 45', 'WHT on royalties to non-residents', 'SG'),
  (uuid_generate_v4(), 'rate', 'WHT_TECH', '{"rate": 0.17, "category": "withholding", "keywords": ["technical","management","consulting","advisory"]}',
   '2010-01-01', 'ITA Sec 45', 'WHT on technical/management fees (prevailing corporate rate)', 'SG'),
  (uuid_generate_v4(), 'rate', 'WHT_DIR', '{"rate": 0.22, "category": "withholding", "keywords": ["director","board"]}',
   '2010-01-01', 'ITA Sec 45', 'WHT on non-resident director fees', 'SG'),
  (uuid_generate_v4(), 'rate', 'WHT_RENT', '{"rate": 0.15, "category": "withholding", "keywords": ["rent","lease","moveable","equipment"]}',
   '2010-01-01', 'ITA Sec 45', 'WHT on rental of moveable property', 'SG'),
  (uuid_generate_v4(), 'rate', 'WHT_SFC', '{"rate": 0.22, "category": "withholding", "keywords": ["srs","withdrawal","supplementary"]}',
   '2010-01-01', 'ITA Sec 45', 'WHT on SRS withdrawal by non-resident', 'SG');

-- SG Penalties
INSERT INTO tax_rules (id, rule_type, rule_key, value, effective_from, source_ref, description, jurisdiction) VALUES
  (uuid_generate_v4(), 'penalty', 'GST_LATE_PAYMENT',
   '{"rate": 0.05, "additional_monthly": 0.02, "max_total": 0.50}',
   '2010-01-01', 'GST Act Sec 47', 'GST late payment: 5% + 2% per month (max 50%)', 'SG'),
  (uuid_generate_v4(), 'penalty', 'FORM_C_LATE_FILING',
   '{"monthly_penalty": 200, "escalation": "estimated NOA"}',
   '2010-01-01', 'ITA Sec 94', 'Form C/C-S late filing: S$200/month, then estimated NOA', 'SG'),
  (uuid_generate_v4(), 'penalty', 'ECI_LATE_FILING',
   '{"monthly_penalty": 200, "max_penalty": 1000}',
   '2010-01-01', 'ITA Sec 94', 'ECI late filing: S$200/month, max S$1,000', 'SG'),
  (uuid_generate_v4(), 'penalty', 'S45_LATE_FILING',
   '{"rate": 0.05, "interest": "prevailing_rate"}',
   '2010-01-01', 'ITA Sec 45(6)', 'S45 late filing: 5% penalty + interest', 'SG'),
  (uuid_generate_v4(), 'penalty', 'IR8A_LATE_FILING',
   '{"max_fine": 5000, "imprisonment_months": 6}',
   '2010-01-01', 'ITA Sec 94(2)', 'IR8A failure: fine up to S$5,000 and/or 6 months', 'SG');

-- GST Registration Threshold
INSERT INTO tax_rules (id, rule_type, rule_key, value, effective_from, source_ref, description, jurisdiction) VALUES
  (uuid_generate_v4(), 'threshold', 'GST_REGISTRATION',
   '{"annual_turnover": 1000000, "currency": "SGD"}',
   '2019-01-01', 'GST Act Sec 8', 'Compulsory GST registration: S$1M annual turnover', 'SG');
