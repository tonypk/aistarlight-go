-- ============================================================
-- Seed: Singapore IRAS Form Schemas (jurisdiction = 'SG')
-- ============================================================

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_GST_F5', 1, 'GST Return (F5)', 'quarterly', true,
   '{"sections":[{"id":"supplies","name":"Supplies","fields":[{"id":"standard_rated_supplies","line":"Box 1","label":"Total Value of Standard-Rated Supplies","editable":true,"required":true},{"id":"zero_rated_supplies","line":"Box 2","label":"Total Value of Zero-Rated Supplies","editable":true},{"id":"exempt_supplies","line":"Box 3","label":"Total Value of Exempt Supplies","editable":true},{"id":"total_supplies","line":"Box 4","label":"Total Value of Supplies","editable":false}]},{"id":"purchases","name":"Purchases","fields":[{"id":"taxable_purchases","line":"Box 5","label":"Total Value of Taxable Purchases","editable":true,"required":true}]},{"id":"tax","name":"GST Computation","fields":[{"id":"output_tax","line":"Box 6","label":"Output Tax Due","editable":false},{"id":"input_tax","line":"Box 7","label":"Input Tax Claimable","editable":false},{"id":"net_gst","line":"Box 8","label":"Net GST Payable/Refundable","editable":false}]}]}',
   '{"total_supplies":"standard_rated_supplies + zero_rated_supplies + exempt_supplies","output_tax":"standard_rated_supplies * 0.09","input_tax":"taxable_purchases * 0.09","net_gst":"output_tax - input_tax"}',
   'SG')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_FORM_C', 1, 'Corporate Income Tax Return (Form C)', 'annual', true,
   '{"sections":[{"id":"income","name":"Income","fields":[{"id":"revenue","line":"A1","label":"Revenue/Turnover","editable":true,"required":true},{"id":"other_income","line":"A2","label":"Other Income","editable":true},{"id":"total_income","line":"A3","label":"Total Income","editable":false}]},{"id":"expenses","name":"Expenses","fields":[{"id":"total_expenses","line":"B1","label":"Total Allowable Expenses","editable":true},{"id":"capital_allowances","line":"B2","label":"Capital Allowances","editable":true}]},{"id":"tax","name":"Tax Computation","fields":[{"id":"adjusted_profit","line":"C1","label":"Adjusted Profit/Loss","editable":false},{"id":"chargeable_income","line":"C2","label":"Chargeable Income","editable":false},{"id":"tax_payable","line":"C3","label":"Tax Payable","editable":false}]}]}',
   '{"total_income":"revenue + other_income","adjusted_profit":"total_income - total_expenses","chargeable_income":"adjusted_profit - capital_allowances","tax_payable":"chargeable_income * 0.17"}',
   'SG')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_FORM_CS', 1, 'Corporate Income Tax Return — Simplified (Form C-S)', 'annual', true,
   '{"sections":[{"id":"income","name":"Income","fields":[{"id":"revenue","line":"1","label":"Revenue","editable":true,"required":true},{"id":"adjusted_profit","line":"2","label":"Adjusted Profit Before Donations","editable":true,"required":true}]},{"id":"tax","name":"Tax Computation","fields":[{"id":"chargeable_income","line":"3","label":"Chargeable Income","editable":false},{"id":"tax_payable","line":"4","label":"Tax Payable @ 17%","editable":false}]}]}',
   '{"chargeable_income":"adjusted_profit","tax_payable":"chargeable_income * 0.17"}',
   'SG')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_FORM_B', 1, 'Income Tax Return for Individuals (Form B)', 'annual', true,
   '{"sections":[{"id":"income","name":"Income","fields":[{"id":"employment_income","line":"1","label":"Employment Income","editable":true},{"id":"trade_income","line":"2","label":"Trade/Business Income","editable":true},{"id":"other_income","line":"3","label":"Other Income","editable":true},{"id":"total_income","line":"4","label":"Total Income","editable":false}]},{"id":"reliefs","name":"Reliefs","fields":[{"id":"earned_income_relief","line":"R1","label":"Earned Income Relief","editable":true},{"id":"cpf_relief","line":"R2","label":"CPF Relief","editable":true},{"id":"total_reliefs","line":"R3","label":"Total Reliefs","editable":false}]},{"id":"tax","name":"Tax Computation","fields":[{"id":"chargeable_income","line":"T1","label":"Chargeable Income","editable":false},{"id":"tax_payable","line":"T2","label":"Tax Payable","editable":false}]}]}',
   '{"total_income":"employment_income + trade_income + other_income","total_reliefs":"earned_income_relief + cpf_relief","chargeable_income":"total_income - total_reliefs"}',
   'SG')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_IR8A', 1, 'Return of Employee''s Remuneration (IR8A)', 'annual', true,
   '{"sections":[{"id":"employee","name":"Employee Info","fields":[{"id":"employee_name","line":"1","label":"Employee Name","editable":true,"required":true},{"id":"nric_fin","line":"2","label":"NRIC/FIN","editable":true,"required":true}]},{"id":"remuneration","name":"Remuneration","fields":[{"id":"gross_salary","line":"A1","label":"Gross Salary","editable":true,"required":true},{"id":"bonus","line":"A2","label":"Bonus","editable":true},{"id":"total_remuneration","line":"A3","label":"Total Remuneration","editable":false}]},{"id":"cpf","name":"CPF Contributions","fields":[{"id":"employee_cpf","line":"B1","label":"Employee CPF","editable":true},{"id":"employer_cpf","line":"B2","label":"Employer CPF","editable":true}]}]}',
   '{"total_remuneration":"gross_salary + bonus"}',
   'SG')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_S45', 1, 'Withholding Tax on Non-Resident Payments (S45)', 'per-payment', true,
   '{"sections":[{"id":"payee","name":"Payee Information","fields":[{"id":"payee_name","line":"1","label":"Payee Name","editable":true,"required":true},{"id":"payee_country","line":"2","label":"Country of Residence","editable":true,"required":true}]},{"id":"payment","name":"Payment Details","fields":[{"id":"income_type","line":"3","label":"Income Type (INT/ROY/TECH/DIR/RENT/SFC)","editable":true,"required":true},{"id":"payment_amount","line":"4","label":"Gross Payment Amount","editable":true,"required":true},{"id":"wht_rate","line":"5","label":"WHT Rate","editable":false},{"id":"tax_withheld","line":"6","label":"Tax Withheld","editable":false},{"id":"net_payment","line":"7","label":"Net Payment","editable":false}]}]}',
   '{"tax_withheld":"payment_amount * wht_rate","net_payment":"payment_amount - tax_withheld"}',
   'SG')
ON CONFLICT (form_type) DO NOTHING;

INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, jurisdiction) VALUES
  (uuid_generate_v4(), 'IRAS_ECI', 1, 'Estimated Chargeable Income (ECI)', 'annual', true,
   '{"sections":[{"id":"income","name":"Estimated Income","fields":[{"id":"revenue","line":"1","label":"Revenue","editable":true,"required":true},{"id":"adjusted_profit","line":"2","label":"Adjusted Profit After Deductions","editable":true,"required":true},{"id":"capital_allowances","line":"3","label":"Capital Allowances","editable":true}]},{"id":"tax","name":"Tax Estimation","fields":[{"id":"estimated_chargeable_income","line":"4","label":"Estimated Chargeable Income","editable":false},{"id":"estimated_tax","line":"5","label":"Estimated Tax @ 17%","editable":false}]}]}',
   '{"estimated_chargeable_income":"adjusted_profit - capital_allowances","estimated_tax":"estimated_chargeable_income * 0.17"}',
   'SG')
ON CONFLICT (form_type) DO NOTHING;
