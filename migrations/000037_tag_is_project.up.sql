-- Add is_project flag to tags table so projects can be managed as special tags.
ALTER TABLE tags ADD COLUMN is_project BOOLEAN NOT NULL DEFAULT false;

-- Partial index: quickly find project tags per company.
CREATE INDEX idx_tags_company_is_project ON tags (company_id, is_project) WHERE is_project = true;

-- Unique constraint: one project name per company.
CREATE UNIQUE INDEX idx_tags_company_name_project ON tags (company_id, name) WHERE is_project = true;
