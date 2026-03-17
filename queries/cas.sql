-- name: NextCASSequence :one
-- Atomically increment and return the next sequence number for a company+type.
INSERT INTO cas_sequences (company_id, sequence_type, last_number, prefix)
VALUES ($1, $2, 1, $3)
ON CONFLICT (company_id, sequence_type)
DO UPDATE SET last_number = cas_sequences.last_number + 1
RETURNING last_number;

-- name: GetLastJournalHash :one
-- Get the hash of the most recent journal entry for a company (for hash chain).
SELECT entry_hash
FROM journal_entries
WHERE company_id = $1 AND entry_hash IS NOT NULL
ORDER BY company_seq_no DESC
LIMIT 1;

-- name: UpdateJournalCASFields :exec
-- Set the CAS fields on a journal entry after creation.
UPDATE journal_entries
SET company_seq_no = $2,
    entry_hash = $3,
    prev_hash = $4,
    journal_book = $5
WHERE id = $1;

-- name: GetLastAuditHash :one
-- Get the hash of the most recent audit log entry for a company.
SELECT entry_hash
FROM audit_logs
WHERE company_id = $1 AND entry_hash IS NOT NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: CreateAuditLogWithHash :one
INSERT INTO audit_logs (company_id, user_id, entity_type, entity_id, action, changes, comment, entry_hash, prev_hash, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, created_at;

-- name: VerifyJournalHashChain :many
-- Returns entries where the hash chain is broken (prev_hash doesn't match previous entry's entry_hash).
WITH ordered_entries AS (
    SELECT id, company_seq_no, entry_hash, prev_hash,
           LAG(entry_hash) OVER (ORDER BY company_seq_no) AS expected_prev_hash
    FROM journal_entries
    WHERE company_id = $1 AND entry_hash IS NOT NULL
    ORDER BY company_seq_no
)
SELECT id, company_seq_no, entry_hash, prev_hash, expected_prev_hash
FROM ordered_entries
WHERE company_seq_no > 1 AND (prev_hash IS DISTINCT FROM expected_prev_hash);

-- name: DetectSequenceGaps :many
-- Detect gaps in per-company journal entry sequence numbers.
WITH seq AS (
    SELECT company_seq_no,
           LEAD(company_seq_no) OVER (ORDER BY company_seq_no) AS next_seq
    FROM journal_entries
    WHERE company_id = $1 AND company_seq_no IS NOT NULL
    ORDER BY company_seq_no
)
SELECT company_seq_no AS gap_after, next_seq AS gap_before
FROM seq
WHERE next_seq IS NOT NULL AND next_seq - company_seq_no > 1;

-- name: CountUnpostedDraftEntries :one
SELECT COUNT(*) FROM journal_entries
WHERE company_id = $1 AND status = 'draft';

-- name: CountEntriesByJournalBook :many
SELECT journal_book, COUNT(*) AS entry_count
FROM journal_entries
WHERE company_id = $1
    AND ($2::date IS NULL OR entry_date >= $2)
    AND ($3::date IS NULL OR entry_date <= $3)
GROUP BY journal_book
ORDER BY journal_book;

-- name: GetSubsidiaryLedger :many
-- Get journal lines for a specific journal book type, with entry details.
SELECT je.id AS entry_id, je.entry_number, je.company_seq_no, je.entry_date,
       je.description AS entry_description, je.journal_book,
       jl.id AS line_id, jl.account_id,
       a.account_number AS account_code, a.name AS account_name,
       jl.debit, jl.credit, jl.description AS line_description,
       jl.tax_code, jl.tax_amount
FROM journal_entries je
JOIN journal_lines jl ON jl.journal_entry_id = je.id
JOIN accounts a ON a.id = jl.account_id
WHERE je.company_id = $1
    AND je.status = 'posted'
    AND je.journal_book = $2
    AND ($3::date IS NULL OR je.entry_date >= $3)
    AND ($4::date IS NULL OR je.entry_date <= $4)
ORDER BY je.entry_date, je.company_seq_no, jl.id;

-- name: InsertCASComplianceCheck :one
INSERT INTO cas_compliance_checks (
    company_id, overall_pass, sequential_numbering_ok, hash_chain_intact,
    double_entry_balanced, periods_properly_closed, audit_trail_complete,
    subsidiary_ledgers_ok, details, checked_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, check_date;

-- name: GetLatestCASCheck :one
SELECT * FROM cas_compliance_checks
WHERE company_id = $1
ORDER BY check_date DESC
LIMIT 1;

-- name: ListCASChecks :many
SELECT id, company_id, check_date, overall_pass,
       sequential_numbering_ok, hash_chain_intact, double_entry_balanced,
       periods_properly_closed, audit_trail_complete, subsidiary_ledgers_ok
FROM cas_compliance_checks
WHERE company_id = $1
ORDER BY check_date DESC
LIMIT $2 OFFSET $3;
