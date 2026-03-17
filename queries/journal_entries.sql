-- name: CreateJournalEntry :one
INSERT INTO journal_entries (id, company_id, period_id, entry_date, reference, description, source_type, source_id, status, memo, created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'draft', $9, $10, NOW(), NOW())
RETURNING *;

-- name: CreateJournalLine :one
INSERT INTO journal_lines (id, journal_entry_id, account_id, line_number, description, debit, credit)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetJournalEntryByID :one
SELECT * FROM journal_entries WHERE id = $1;

-- name: ListJournalLinesByEntry :many
SELECT jl.*, a.name AS account_name, a.account_number
FROM journal_lines jl
JOIN accounts a ON a.id = jl.account_id
WHERE jl.journal_entry_id = $1
ORDER BY jl.line_number ASC;

-- name: ListJournalEntries :many
SELECT * FROM journal_entries
WHERE company_id = $1
ORDER BY entry_date DESC, entry_number DESC
LIMIT $2 OFFSET $3;

-- name: CountJournalEntries :one
SELECT COUNT(*) FROM journal_entries WHERE company_id = $1;

-- name: ListJournalEntriesByDateRange :many
SELECT * FROM journal_entries
WHERE company_id = $1
    AND entry_date >= $2
    AND entry_date <= $3
ORDER BY entry_date DESC, entry_number DESC
LIMIT $4 OFFSET $5;

-- name: CountJournalEntriesByDateRange :one
SELECT COUNT(*) FROM journal_entries
WHERE company_id = $1 AND entry_date >= $2 AND entry_date <= $3;

-- name: PostJournalEntry :exec
UPDATE journal_entries SET
    status = 'posted',
    posted_by = $2,
    posted_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND status = 'draft';

-- name: ReverseJournalEntry :exec
UPDATE journal_entries SET
    status = 'reversed',
    reversed_by_id = $2,
    updated_at = NOW()
WHERE id = $1 AND status = 'posted';

-- name: TrialBalance :many
SELECT
    a.id AS account_id,
    a.account_number,
    a.name AS account_name,
    a.account_type,
    COALESCE(SUM(jl.debit), 0)::NUMERIC(15,2) AS debit_balance,
    COALESCE(SUM(jl.credit), 0)::NUMERIC(15,2) AS credit_balance
FROM accounts a
LEFT JOIN journal_lines jl ON jl.account_id = a.id
LEFT JOIN journal_entries je ON je.id = jl.journal_entry_id
    AND je.company_id = $1
    AND je.status = 'posted'
    AND je.entry_date >= $2
    AND je.entry_date <= $3
WHERE a.company_id = $1 AND a.is_active = true
GROUP BY a.id, a.account_number, a.name, a.account_type
HAVING COALESCE(SUM(jl.debit), 0) > 0 OR COALESCE(SUM(jl.credit), 0) > 0
ORDER BY a.account_number ASC;

-- name: AccountLedger :many
SELECT
    je.id AS journal_entry_id,
    je.entry_number,
    je.entry_date,
    je.reference,
    jl.description,
    jl.debit,
    jl.credit
FROM journal_lines jl
JOIN journal_entries je ON je.id = jl.journal_entry_id
WHERE jl.account_id = $1
    AND je.company_id = $2
    AND je.status = 'posted'
    AND je.entry_date >= $3
    AND je.entry_date <= $4
ORDER BY je.entry_date ASC, je.entry_number ASC;

-- name: SumJournalLines :one
SELECT
    COALESCE(SUM(debit), 0)::NUMERIC(15,2) AS total_debit,
    COALESCE(SUM(credit), 0)::NUMERIC(15,2) AS total_credit
FROM journal_lines
WHERE journal_entry_id = $1;

-- name: CountDraftEntriesByPeriod :one
SELECT COUNT(*) FROM journal_entries
WHERE period_id = $1 AND status = 'draft';

-- name: AccountBalancesByPrefix :many
-- Returns aggregated debit/credit for all accounts matching a number prefix,
-- for posted entries up to a given date.
SELECT
    a.id AS account_id,
    a.account_number,
    a.name AS account_name,
    a.account_type,
    a.sub_type,
    a.normal_balance,
    a.cash_flow_category,
    COALESCE(SUM(jl.debit), 0)::NUMERIC(15,2) AS total_debit,
    COALESCE(SUM(jl.credit), 0)::NUMERIC(15,2) AS total_credit
FROM accounts a
LEFT JOIN journal_lines jl ON jl.account_id = a.id
LEFT JOIN journal_entries je ON je.id = jl.journal_entry_id
    AND je.company_id = @company_id
    AND je.status = 'posted'
    AND je.entry_date <= @as_of_date
WHERE a.company_id = @company_id
    AND a.is_active = true
    AND a.account_number LIKE @prefix || '%'
GROUP BY a.id, a.account_number, a.name, a.account_type, a.sub_type, a.normal_balance, a.cash_flow_category
HAVING COALESCE(SUM(jl.debit), 0) > 0 OR COALESCE(SUM(jl.credit), 0) > 0
ORDER BY a.account_number ASC;

-- name: PeriodAccountBalances :many
-- Returns aggregated debit/credit for all accounts matching a number prefix,
-- for posted entries within a date range.
SELECT
    a.id AS account_id,
    a.account_number,
    a.name AS account_name,
    a.account_type,
    a.sub_type,
    a.normal_balance,
    a.cash_flow_category,
    COALESCE(SUM(jl.debit), 0)::NUMERIC(15,2) AS total_debit,
    COALESCE(SUM(jl.credit), 0)::NUMERIC(15,2) AS total_credit
FROM accounts a
LEFT JOIN journal_lines jl ON jl.account_id = a.id
LEFT JOIN journal_entries je ON je.id = jl.journal_entry_id
    AND je.company_id = @company_id
    AND je.status = 'posted'
    AND je.entry_date >= @from_date
    AND je.entry_date <= @to_date
WHERE a.company_id = @company_id
    AND a.is_active = true
    AND a.account_number LIKE @prefix || '%'
GROUP BY a.id, a.account_number, a.name, a.account_type, a.sub_type, a.normal_balance, a.cash_flow_category
HAVING COALESCE(SUM(jl.debit), 0) > 0 OR COALESCE(SUM(jl.credit), 0) > 0
ORDER BY a.account_number ASC;

-- name: AllAccountBalancesAsOf :many
-- Returns aggregated balances for ALL active accounts up to a date (for balance sheet).
SELECT
    a.id AS account_id,
    a.account_number,
    a.name AS account_name,
    a.account_type,
    a.sub_type,
    a.normal_balance,
    a.cash_flow_category,
    COALESCE(SUM(jl.debit), 0)::NUMERIC(15,2) AS total_debit,
    COALESCE(SUM(jl.credit), 0)::NUMERIC(15,2) AS total_credit
FROM accounts a
LEFT JOIN journal_lines jl ON jl.account_id = a.id
LEFT JOIN journal_entries je ON je.id = jl.journal_entry_id
    AND je.company_id = @company_id
    AND je.status = 'posted'
    AND je.entry_date <= @as_of_date
WHERE a.company_id = @company_id AND a.is_active = true
GROUP BY a.id, a.account_number, a.name, a.account_type, a.sub_type, a.normal_balance, a.cash_flow_category
HAVING COALESCE(SUM(jl.debit), 0) > 0 OR COALESCE(SUM(jl.credit), 0) > 0
ORDER BY a.account_number ASC;

-- name: FindJournalEntryBySourceRef :one
SELECT * FROM journal_entries
WHERE company_id = $1
    AND source_type = $2
    AND reference = $3
    AND status IN ('draft', 'posted')
ORDER BY created_at DESC
LIMIT 1;

-- name: PeriodAllAccountBalances :many
-- Returns aggregated balances for ALL active accounts within a date range (for income statement).
SELECT
    a.id AS account_id,
    a.account_number,
    a.name AS account_name,
    a.account_type,
    a.sub_type,
    a.normal_balance,
    a.cash_flow_category,
    COALESCE(SUM(jl.debit), 0)::NUMERIC(15,2) AS total_debit,
    COALESCE(SUM(jl.credit), 0)::NUMERIC(15,2) AS total_credit
FROM accounts a
LEFT JOIN journal_lines jl ON jl.account_id = a.id
LEFT JOIN journal_entries je ON je.id = jl.journal_entry_id
    AND je.company_id = @company_id
    AND je.status = 'posted'
    AND je.entry_date >= @from_date
    AND je.entry_date <= @to_date
WHERE a.company_id = @company_id AND a.is_active = true
GROUP BY a.id, a.account_number, a.name, a.account_type, a.sub_type, a.normal_balance, a.cash_flow_category
HAVING COALESCE(SUM(jl.debit), 0) > 0 OR COALESCE(SUM(jl.credit), 0) > 0
ORDER BY a.account_number ASC;
