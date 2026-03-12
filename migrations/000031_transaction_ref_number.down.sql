DROP INDEX IF EXISTS idx_transactions_company_ref;
ALTER TABLE transactions DROP COLUMN IF EXISTS ref_number;
