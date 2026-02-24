-- name: CreateAccountingPeriod :one
INSERT INTO accounting_periods (id, company_id, name, period_type, start_date, end_date, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, 'open', NOW(), NOW())
RETURNING *;

-- name: GetAccountingPeriodByID :one
SELECT * FROM accounting_periods WHERE id = $1;

-- name: ListAccountingPeriods :many
SELECT * FROM accounting_periods
WHERE company_id = $1
ORDER BY start_date DESC;

-- name: CloseAccountingPeriod :exec
UPDATE accounting_periods SET
    status = 'closed',
    closed_by = $2,
    closed_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND status = 'open';

-- name: ReopenAccountingPeriod :exec
UPDATE accounting_periods SET
    status = 'open',
    closed_by = NULL,
    closed_at = NULL,
    updated_at = NOW()
WHERE id = $1 AND status = 'closed';

-- name: FindPeriodByDate :one
SELECT * FROM accounting_periods
WHERE company_id = $1
    AND start_date <= $2
    AND end_date >= $2
    AND status = 'open'
LIMIT 1;
