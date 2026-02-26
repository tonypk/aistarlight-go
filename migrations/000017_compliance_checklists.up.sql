-- ============================================================
-- Compliance Checklists (DB-driven compliance rules)
-- ============================================================
CREATE TABLE IF NOT EXISTS compliance_checklists (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    form_type    VARCHAR(30) NOT NULL,
    check_id     VARCHAR(50) NOT NULL,
    check_name   VARCHAR(200) NOT NULL,
    severity     VARCHAR(20) NOT NULL,  -- critical, high, medium, low
    description  TEXT,
    rule_ref     VARCHAR(200),          -- reference to tax_rules or BIR regulation
    is_active    BOOLEAN DEFAULT true,
    sort_order   INT DEFAULT 0,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_compliance_checklists_form_check
  ON compliance_checklists (form_type, check_id);
CREATE INDEX IF NOT EXISTS idx_compliance_checklists_form
  ON compliance_checklists (form_type) WHERE is_active = true;

-- Seed: Existing 11 hardcoded checks for BIR_2550M
INSERT INTO compliance_checklists (id, form_type, check_id, check_name, severity, description, rule_ref, sort_order) VALUES
  (uuid_generate_v4(), 'BIR_2550M', 'required_fields', 'Required Fields', 'critical',
   'Verify all mandatory fields are present: vatable_sales, sales_to_government, zero_rated_sales, exempt_sales, output_vat, total_input_vat',
   NULL, 1),
  (uuid_generate_v4(), 'BIR_2550M', 'cross_field', 'Cross-field Consistency', 'critical',
   'Total sales must equal sum of vatable + government + zero-rated + exempt sales',
   NULL, 2),
  (uuid_generate_v4(), 'BIR_2550M', 'output_vat', 'Output VAT Accuracy', 'high',
   'Output VAT must be 12% of vatable sales',
   'NIRC Sec 106', 3),
  (uuid_generate_v4(), 'BIR_2550M', 'govt_vat', 'Government VAT Rate', 'high',
   'Government VAT withholding must be 5% of government sales',
   'NIRC Sec 114(C)', 4),
  (uuid_generate_v4(), 'BIR_2550M', 'amount_ranges', 'Amount Ranges', 'high',
   'All monetary fields must be non-negative and under PHP 999,999,999',
   NULL, 5),
  (uuid_generate_v4(), 'BIR_2550M', 'tin_format', 'TIN Format', 'medium',
   'TIN must follow ###-###-###-### format',
   'NIRC Sec 236', 6),
  (uuid_generate_v4(), 'BIR_2550M', 'filing_deadline', 'Filing Deadline', 'medium',
   'Monthly VAT due by 20th of following month',
   'NIRC Sec 114(A)', 7),
  (uuid_generate_v4(), 'BIR_2550M', 'period_anomaly', 'Period-over-Period Anomaly', 'medium',
   'Flag changes >50% vs prior period',
   NULL, 8),
  (uuid_generate_v4(), 'BIR_2550M', 'duplicate', 'Duplicate Report', 'medium',
   'Check for duplicate active reports for same period',
   NULL, 9),
  (uuid_generate_v4(), 'BIR_2550M', 'capital_goods', 'Capital Goods Threshold', 'low',
   'Input VAT on capital goods >PHP 1M requires 60-month amortization',
   'RR 16-2005', 10),
  (uuid_generate_v4(), 'BIR_2550M', 'zero_filing', 'Zero Filing Warning', 'low',
   'Flag nil return filings for review',
   NULL, 11);

-- Seed: BIR_2550Q checks (same as 2550M mostly)
INSERT INTO compliance_checklists (id, form_type, check_id, check_name, severity, description, rule_ref, sort_order) VALUES
  (uuid_generate_v4(), 'BIR_2550Q', 'required_fields', 'Required Fields', 'critical', 'Required fields for quarterly VAT', NULL, 1),
  (uuid_generate_v4(), 'BIR_2550Q', 'cross_field', 'Cross-field Consistency', 'critical', 'Sales components must sum correctly', NULL, 2),
  (uuid_generate_v4(), 'BIR_2550Q', 'output_vat', 'Output VAT Accuracy', 'high', 'Output VAT = 12% of vatable sales', 'NIRC Sec 106', 3),
  (uuid_generate_v4(), 'BIR_2550Q', 'govt_vat', 'Government VAT Rate', 'high', 'Government VAT = 5% of govt sales', 'NIRC Sec 114(C)', 4),
  (uuid_generate_v4(), 'BIR_2550Q', 'filing_deadline', 'Filing Deadline', 'medium', 'Quarterly VAT due 25th of month after quarter', 'NIRC Sec 114(A)', 5);

-- Seed: BIR_1601C checks
INSERT INTO compliance_checklists (id, form_type, check_id, check_name, severity, description, rule_ref, sort_order) VALUES
  (uuid_generate_v4(), 'BIR_1601C', 'required_fields', 'Required Fields', 'critical', 'Required: total_compensation, tax_withheld, tax_due, amount_remitted', NULL, 1),
  (uuid_generate_v4(), 'BIR_1601C', 'filing_deadline', 'Filing Deadline', 'medium', 'Monthly WHT due 10th of following month', 'NIRC Sec 58(A)', 2);

-- Seed: BIR_0619E checks
INSERT INTO compliance_checklists (id, form_type, check_id, check_name, severity, description, rule_ref, sort_order) VALUES
  (uuid_generate_v4(), 'BIR_0619E', 'required_fields', 'Required Fields', 'critical', 'Required: tax_base, tax_rate, tax_due', NULL, 1),
  (uuid_generate_v4(), 'BIR_0619E', 'filing_deadline', 'Filing Deadline', 'medium', 'Monthly EWT due 10th of following month', 'NIRC Sec 58(A)', 2);
