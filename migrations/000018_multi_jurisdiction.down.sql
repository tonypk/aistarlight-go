DROP INDEX IF EXISTS idx_compliance_checklists_jurisdiction;
ALTER TABLE compliance_checklists DROP COLUMN IF EXISTS jurisdiction;

DROP INDEX IF EXISTS idx_tax_rules_jurisdiction;
ALTER TABLE tax_rules DROP COLUMN IF EXISTS jurisdiction;

DROP INDEX IF EXISTS idx_form_schemas_jurisdiction;
ALTER TABLE form_schemas DROP COLUMN IF EXISTS jurisdiction;

DROP INDEX IF EXISTS idx_knowledge_chunks_jurisdiction;
ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS jurisdiction;

DROP INDEX IF EXISTS idx_companies_jurisdiction;
ALTER TABLE companies DROP COLUMN IF EXISTS jurisdiction;
