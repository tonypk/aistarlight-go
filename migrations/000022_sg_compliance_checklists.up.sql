-- ============================================================
-- Seed: Singapore IRAS Compliance Checklists (jurisdiction = 'SG')
-- ============================================================

-- IRAS_GST_F5 checks
INSERT INTO compliance_checklists (id, form_type, check_id, check_name, severity, description, rule_ref, sort_order, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_GST_F5', 'required_fields', 'Required Fields', 'critical',
   'Verify mandatory fields: standard_rated_supplies, taxable_purchases, output_tax, input_tax', NULL, 1, 'SG'),
  (uuid_generate_v4(), 'IRAS_GST_F5', 'cross_field', 'Cross-field Consistency', 'critical',
   'Box 4 (total supplies) must equal Box 1 + Box 2 + Box 3', NULL, 2, 'SG'),
  (uuid_generate_v4(), 'IRAS_GST_F5', 'output_gst', 'Output GST Accuracy', 'high',
   'Output tax (Box 6) must be 9% of standard-rated supplies (Box 1)', 'GST Act Sec 16', 3, 'SG'),
  (uuid_generate_v4(), 'IRAS_GST_F5', 'input_gst', 'Input GST Accuracy', 'high',
   'Input tax (Box 7) must not exceed 9% of taxable purchases (Box 5)', 'GST Act Sec 20', 4, 'SG'),
  (uuid_generate_v4(), 'IRAS_GST_F5', 'amount_ranges', 'Amount Ranges', 'high',
   'All monetary fields must be non-negative', NULL, 5, 'SG'),
  (uuid_generate_v4(), 'IRAS_GST_F5', 'filing_deadline', 'Filing Deadline', 'medium',
   'GST F5 due 1 month after end of accounting period', 'GST Act Sec 19', 6, 'SG'),
  (uuid_generate_v4(), 'IRAS_GST_F5', 'period_anomaly', 'Period-over-Period Anomaly', 'medium',
   'Flag changes >50% vs prior quarter', NULL, 7, 'SG'),
  (uuid_generate_v4(), 'IRAS_GST_F5', 'zero_filing', 'Nil Return Warning', 'low',
   'Flag nil return filings for review', NULL, 8, 'SG');

-- IRAS_FORM_C / IRAS_FORM_CS checks
INSERT INTO compliance_checklists (id, form_type, check_id, check_name, severity, description, rule_ref, sort_order, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_FORM_C', 'required_fields', 'Required Fields', 'critical',
   'Verify mandatory: revenue, total_expenses, adjusted_profit, chargeable_income', NULL, 1, 'SG'),
  (uuid_generate_v4(), 'IRAS_FORM_C', 'cross_field', 'Cross-field Consistency', 'critical',
   'Adjusted profit = total income - total expenses; chargeable income = adjusted profit - capital allowances', NULL, 2, 'SG'),
  (uuid_generate_v4(), 'IRAS_FORM_C', 'tax_rate', 'Corporate Tax Rate', 'high',
   'Tax payable should be 17% of chargeable income (before partial exemption)', 'ITA Sec 43', 3, 'SG'),
  (uuid_generate_v4(), 'IRAS_FORM_C', 'partial_exemption', 'Partial Tax Exemption', 'medium',
   'Check if partial exemption applied: 75% on first S$10K, 50% on next S$190K', 'ITA Sec 43(6A)', 4, 'SG'),
  (uuid_generate_v4(), 'IRAS_FORM_C', 'filing_deadline', 'Filing Deadline', 'medium',
   'Form C due by 30 November', 'ITA Sec 94', 5, 'SG');

INSERT INTO compliance_checklists (id, form_type, check_id, check_name, severity, description, rule_ref, sort_order, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_FORM_CS', 'required_fields', 'Required Fields', 'critical',
   'Verify mandatory: revenue, adjusted_profit', NULL, 1, 'SG'),
  (uuid_generate_v4(), 'IRAS_FORM_CS', 'revenue_limit', 'Revenue Eligibility', 'high',
   'Form C-S only for companies with revenue ≤ S$5,000,000', 'IRAS e-Tax Guide', 2, 'SG'),
  (uuid_generate_v4(), 'IRAS_FORM_CS', 'filing_deadline', 'Filing Deadline', 'medium',
   'Form C-S due by 30 November', 'ITA Sec 94', 3, 'SG');

-- IRAS_IR8A checks
INSERT INTO compliance_checklists (id, form_type, check_id, check_name, severity, description, rule_ref, sort_order, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_IR8A', 'required_fields', 'Required Fields', 'critical',
   'Verify mandatory: employee_name, nric_fin, gross_salary, total_remuneration', NULL, 1, 'SG'),
  (uuid_generate_v4(), 'IRAS_IR8A', 'cpf_consistency', 'CPF Contribution Check', 'high',
   'Employee CPF should be ~20% and employer CPF ~17% of capped ordinary wages (S$6,800/month)', 'CPF Act Sec 7', 2, 'SG'),
  (uuid_generate_v4(), 'IRAS_IR8A', 'filing_deadline', 'Filing Deadline', 'medium',
   'IR8A must be submitted by 1 March', 'ITA Sec 68(2)', 3, 'SG');

-- IRAS_S45 checks
INSERT INTO compliance_checklists (id, form_type, check_id, check_name, severity, description, rule_ref, sort_order, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_S45', 'required_fields', 'Required Fields', 'critical',
   'Verify mandatory: payee_name, income_type, payment_amount, wht_rate, tax_withheld', NULL, 1, 'SG'),
  (uuid_generate_v4(), 'IRAS_S45', 'wht_rate_check', 'WHT Rate Verification', 'high',
   'WHT rate must match income type: INT 15%, ROY 10%, TECH 17%, DIR 22%, RENT 15%, SFC 22%', 'ITA Sec 45', 2, 'SG'),
  (uuid_generate_v4(), 'IRAS_S45', 'tax_calculation', 'Tax Calculation', 'high',
   'Tax withheld = payment amount × WHT rate', NULL, 3, 'SG'),
  (uuid_generate_v4(), 'IRAS_S45', 'filing_deadline', 'Filing Deadline', 'medium',
   'S45 due by 15th of 2nd month from payment date', 'ITA Sec 45(7)', 4, 'SG');

-- IRAS_ECI checks
INSERT INTO compliance_checklists (id, form_type, check_id, check_name, severity, description, rule_ref, sort_order, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_ECI', 'required_fields', 'Required Fields', 'critical',
   'Verify mandatory: revenue, estimated_chargeable_income', NULL, 1, 'SG'),
  (uuid_generate_v4(), 'IRAS_ECI', 'nil_eci_waiver', 'Nil ECI Waiver Check', 'medium',
   'Companies with revenue ≤ S$5M and nil ECI may be waived from filing', 'ITA Sec 43(3B)', 2, 'SG'),
  (uuid_generate_v4(), 'IRAS_ECI', 'filing_deadline', 'Filing Deadline', 'medium',
   'ECI due within 3 months of financial year-end', 'ITA Sec 43(3)', 3, 'SG');

-- IRAS_FORM_B checks
INSERT INTO compliance_checklists (id, form_type, check_id, check_name, severity, description, rule_ref, sort_order, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_FORM_B', 'required_fields', 'Required Fields', 'critical',
   'Verify mandatory: total_income, chargeable_income', NULL, 1, 'SG'),
  (uuid_generate_v4(), 'IRAS_FORM_B', 'relief_limits', 'Relief Limits', 'high',
   'Check personal reliefs do not exceed statutory caps (earned income, CPF, life insurance, etc.)', 'ITA Sec 39', 2, 'SG'),
  (uuid_generate_v4(), 'IRAS_FORM_B', 'filing_deadline', 'Filing Deadline', 'medium',
   'Form B due by 18 April (e-Filing)', 'ITA Sec 94', 3, 'SG');
