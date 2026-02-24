-- name: CreateAnomaly :one
INSERT INTO anomalies (id, company_id, session_id, transaction_id, anomaly_type, severity, description, details, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
RETURNING *;

-- name: UpdateAnomaly :exec
UPDATE anomalies SET
    status = COALESCE($2, status),
    resolved_by = $3,
    resolved_at = $4,
    resolution_note = $5,
    updated_at = NOW()
WHERE id = $1;

-- name: ListAnomaliesBySession :many
SELECT * FROM anomalies WHERE session_id = $1
ORDER BY created_at;
