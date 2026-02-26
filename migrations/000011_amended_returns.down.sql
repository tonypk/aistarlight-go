DROP INDEX IF EXISTS idx_reports_original;
ALTER TABLE reports
    DROP COLUMN IF EXISTS amendment_number,
    DROP COLUMN IF EXISTS original_report_id;
