DROP TABLE IF EXISTS cas_compliance_checks;
DROP INDEX IF EXISTS idx_audit_logs_hash_chain;
ALTER TABLE audit_logs
    DROP COLUMN IF EXISTS entry_hash,
    DROP COLUMN IF EXISTS prev_hash,
    DROP COLUMN IF EXISTS ip_address,
    DROP COLUMN IF EXISTS user_agent;
DROP INDEX IF EXISTS idx_journal_entries_journal_book;
DROP INDEX IF EXISTS idx_journal_entries_hash;
DROP INDEX IF EXISTS idx_journal_entries_cas_seq;
ALTER TABLE journal_entries
    DROP COLUMN IF EXISTS company_seq_no,
    DROP COLUMN IF EXISTS entry_hash,
    DROP COLUMN IF EXISTS prev_hash,
    DROP COLUMN IF EXISTS journal_book;
DROP TABLE IF EXISTS cas_sequences;
