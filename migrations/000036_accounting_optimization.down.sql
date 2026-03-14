-- 000036 down: Revert accounting optimization

BEGIN;

DROP INDEX IF EXISTS idx_exchange_rates_lookup;
DROP INDEX IF EXISTS idx_exchange_rates_company;
DROP TABLE IF EXISTS exchange_rates;

DROP INDEX IF EXISTS idx_tax_codes_company;
DROP TABLE IF EXISTS tax_codes;

ALTER TABLE journal_lines DROP COLUMN IF EXISTS tax_amount;
ALTER TABLE journal_lines DROP COLUMN IF EXISTS tax_code;
ALTER TABLE journal_lines DROP COLUMN IF EXISTS home_credit;
ALTER TABLE journal_lines DROP COLUMN IF EXISTS home_debit;
ALTER TABLE journal_lines DROP COLUMN IF EXISTS exchange_rate;
ALTER TABLE journal_lines DROP COLUMN IF EXISTS currency_code;
ALTER TABLE journal_entries DROP COLUMN IF EXISTS currency_code;

ALTER TABLE accounts DROP COLUMN IF EXISTS cash_flow_category;
ALTER TABLE accounts DROP COLUMN IF EXISTS default_tax_code;
ALTER TABLE accounts DROP COLUMN IF EXISTS currency_code;

ALTER TABLE companies DROP COLUMN IF EXISTS base_currency;

ALTER TABLE vendors DROP COLUMN IF EXISTS notes;
ALTER TABLE vendors DROP COLUMN IF EXISTS is_active;
ALTER TABLE vendors DROP COLUMN IF EXISTS default_account_id;
ALTER TABLE vendors DROP COLUMN IF EXISTS currency_code;
ALTER TABLE vendors DROP COLUMN IF EXISTS payment_terms_days;
ALTER TABLE vendors DROP COLUMN IF EXISTS phone;
ALTER TABLE vendors DROP COLUMN IF EXISTS email;

ALTER TABLE withholding_certificates RENAME COLUMN vendor_id TO supplier_id;
ALTER TABLE transactions RENAME COLUMN vendor_id TO supplier_id;

ALTER TABLE vendors RENAME COLUMN vendor_type TO supplier_type;
ALTER TABLE vendors RENAME TO suppliers;

COMMIT;
