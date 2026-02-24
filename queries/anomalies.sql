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

-- name: DeleteAnomaliesBySession :exec
DELETE FROM anomalies WHERE session_id = $1;

-- name: ListAnomaliesBySessionFiltered :many
SELECT * FROM anomalies
WHERE session_id = $1
  AND ($4::varchar = '' OR status = $4)
ORDER BY created_at
LIMIT $2 OFFSET $3;

-- name: CountAnomaliesBySession :one
SELECT COUNT(*) FROM anomalies WHERE session_id = $1;

-- name: CountAnomaliesBySessionFiltered :one
SELECT COUNT(*) FROM anomalies
WHERE session_id = $1
  AND ($2::varchar = '' OR status = $2);

-- name: GetAnomalyByID :one
SELECT * FROM anomalies WHERE id = $1;
