-- Remove seeded form_schemas (only those added by this migration)
DELETE FROM form_schemas WHERE form_type IN ('BIR_0619E', 'BIR_1701', 'BIR_1702', 'BIR_2316', 'BIR_2307', 'SAWT');

DROP INDEX IF EXISTS idx_tax_rules_type;
DROP INDEX IF EXISTS idx_tax_rules_active;
DROP TABLE IF EXISTS tax_rules;
