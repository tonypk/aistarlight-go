-- BIR CAS (Computerized Accounting System) certification support
-- Adds hash chain for tamper detection, per-company sequential numbering,
-- journal book classification, and enhanced audit logging.

-- Per-company sequence counter for gap-free document numbering
CREATE TABLE IF NOT EXISTS cas_sequences (
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    sequence_type VARCHAR(30) NOT NULL, -- 'journal_entry', 'invoice', etc.
    last_number BIGINT NOT NULL DEFAULT 0,
    prefix VARCHAR(10) NOT NULL DEFAULT '',
    PRIMARY KEY (company_id, sequence_type)
);

-- Add CAS columns to journal_entries
ALTER TABLE journal_entries
    ADD COLUMN IF NOT EXISTS company_seq_no BIGINT,
    ADD COLUMN IF NOT EXISTS entry_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS prev_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS journal_book VARCHAR(30) DEFAULT 'general_journal';

-- Index for hash chain verification (ordered by company + sequence)
CREATE INDEX IF NOT EXISTS idx_journal_entries_cas_seq
    ON journal_entries (company_id, company_seq_no)
    WHERE company_seq_no IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_journal_entries_hash
    ON journal_entries (entry_hash)
    WHERE entry_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_journal_entries_journal_book
    ON journal_entries (company_id, journal_book, entry_date);

-- Add CAS columns to audit_logs
ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS entry_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS prev_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS ip_address VARCHAR(45),
    ADD COLUMN IF NOT EXISTS user_agent VARCHAR(256);

CREATE INDEX IF NOT EXISTS idx_audit_logs_hash_chain
    ON audit_logs (company_id, created_at)
    WHERE entry_hash IS NOT NULL;

-- CAS compliance check results
CREATE TABLE IF NOT EXISTS cas_compliance_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    check_date TIMESTAMPTZ NOT NULL DEFAULT now(),
    overall_pass BOOLEAN NOT NULL DEFAULT false,
    sequential_numbering_ok BOOLEAN NOT NULL DEFAULT false,
    hash_chain_intact BOOLEAN NOT NULL DEFAULT false,
    double_entry_balanced BOOLEAN NOT NULL DEFAULT false,
    periods_properly_closed BOOLEAN NOT NULL DEFAULT false,
    audit_trail_complete BOOLEAN NOT NULL DEFAULT false,
    subsidiary_ledgers_ok BOOLEAN NOT NULL DEFAULT false,
    details JSONB NOT NULL DEFAULT '{}',
    checked_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_cas_compliance_company
    ON cas_compliance_checks (company_id, check_date DESC);

-- Comment for documentation
COMMENT ON COLUMN journal_entries.company_seq_no IS 'Per-company sequential number, gap-free, for BIR CAS compliance';
COMMENT ON COLUMN journal_entries.entry_hash IS 'SHA-256 hash of entry data for tamper detection';
COMMENT ON COLUMN journal_entries.prev_hash IS 'Hash of the previous entry in the chain (per company)';
COMMENT ON COLUMN journal_entries.journal_book IS 'BIR journal classification: sales_journal, purchases_journal, cash_receipts, cash_disbursements, general_journal';
