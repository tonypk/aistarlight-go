ALTER TABLE receipt_batches DROP COLUMN IF EXISTS approval_status;
DROP TABLE IF EXISTS receipt_approvals;
DROP TABLE IF EXISTS company_approval_settings;
