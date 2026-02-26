-- Balance tracking for reconciliation sessions
ALTER TABLE reconciliation_sessions ADD COLUMN IF NOT EXISTS opening_balance NUMERIC(15,2);
ALTER TABLE reconciliation_sessions ADD COLUMN IF NOT EXISTS closing_balance NUMERIC(15,2);
ALTER TABLE reconciliation_sessions ADD COLUMN IF NOT EXISTS bank_closing_balance NUMERIC(15,2);
