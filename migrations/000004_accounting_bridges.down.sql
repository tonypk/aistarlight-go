-- Reverse 000004_accounting_bridges

DROP INDEX IF EXISTS idx_transactions_journal_entry_id;
ALTER TABLE transactions DROP COLUMN IF EXISTS journal_entry_id;
ALTER TABLE receipt_batches DROP COLUMN IF EXISTS transaction_ids;
