-- Multi-jurisdiction support: add jurisdiction column to core tables
-- Default 'PH' so all existing data remains Philippine

ALTER TABLE companies ADD COLUMN jurisdiction VARCHAR(2) NOT NULL DEFAULT 'PH';
CREATE INDEX idx_companies_jurisdiction ON companies(jurisdiction);

ALTER TABLE knowledge_chunks ADD COLUMN jurisdiction VARCHAR(2) NOT NULL DEFAULT 'PH';
CREATE INDEX idx_knowledge_chunks_jurisdiction ON knowledge_chunks(jurisdiction);

ALTER TABLE form_schemas ADD COLUMN jurisdiction VARCHAR(2) NOT NULL DEFAULT 'PH';
CREATE INDEX idx_form_schemas_jurisdiction ON form_schemas(jurisdiction);

ALTER TABLE tax_rules ADD COLUMN jurisdiction VARCHAR(2) NOT NULL DEFAULT 'PH';
CREATE INDEX idx_tax_rules_jurisdiction ON tax_rules(jurisdiction);

ALTER TABLE compliance_checklists ADD COLUMN jurisdiction VARCHAR(2) NOT NULL DEFAULT 'PH';
CREATE INDEX idx_compliance_checklists_jurisdiction ON compliance_checklists(jurisdiction);
