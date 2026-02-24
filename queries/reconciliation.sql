-- name: CreateReconciliationSession :one
INSERT INTO reconciliation_sessions (id, company_id, created_by, period, status, report_id, source_files, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
RETURNING *;

-- name: GetReconciliationSessionByID :one
SELECT * FROM reconciliation_sessions WHERE id = $1;

-- name: UpdateReconciliationSession :exec
UPDATE reconciliation_sessions SET
    status = COALESCE($2, status),
    report_id = $3,
    source_files = COALESCE($4, source_files),
    summary = $5,
    reconciliation_result = $6,
    completed_at = $7,
    updated_at = NOW()
WHERE id = $1;

-- name: ListReconciliationSessionsByCompany :many
SELECT * FROM reconciliation_sessions
WHERE company_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountReconciliationSessionsByCompany :one
SELECT COUNT(*) FROM reconciliation_sessions WHERE company_id = $1;
