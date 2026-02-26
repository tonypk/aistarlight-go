ALTER TABLE reports
    ADD COLUMN IF NOT EXISTS amendment_number INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS original_report_id UUID REFERENCES reports(id);

CREATE INDEX idx_reports_original ON reports(original_report_id) WHERE original_report_id IS NOT NULL;
