-- 000004_accounting_bridges: Add bridge columns for pipeline automation

-- Bridge 2: Link transactions to journal entries
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS journal_entry_id UUID REFERENCES journal_entries(id);
CREATE INDEX IF NOT EXISTS idx_transactions_journal_entry_id ON transactions(journal_entry_id) WHERE journal_entry_id IS NOT NULL;

-- Bridge 1: Link receipt batches to transactions (track which receipt generated which transaction)
ALTER TABLE receipt_batches ADD COLUMN IF NOT EXISTS transaction_ids UUID[] DEFAULT '{}';
