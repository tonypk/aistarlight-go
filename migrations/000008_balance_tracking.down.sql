ALTER TABLE reconciliation_sessions DROP COLUMN IF EXISTS opening_balance;
ALTER TABLE reconciliation_sessions DROP COLUMN IF EXISTS closing_balance;
ALTER TABLE reconciliation_sessions DROP COLUMN IF EXISTS bank_closing_balance;
