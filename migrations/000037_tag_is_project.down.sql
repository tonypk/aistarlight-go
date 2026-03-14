DROP INDEX IF EXISTS idx_tags_company_name_project;
DROP INDEX IF EXISTS idx_tags_company_is_project;
ALTER TABLE tags DROP COLUMN IF EXISTS is_project;
