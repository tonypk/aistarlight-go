-- name: CreateQBOConnection :one
INSERT INTO qbo_connections (id, company_id, realm_id, access_token_enc, refresh_token_enc, token_expiry, refresh_expiry, scope, is_active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true, NOW(), NOW())
RETURNING *;

-- name: GetQBOConnectionByCompany :one
SELECT * FROM qbo_connections WHERE company_id = $1 AND is_active = true;

-- name: UpdateQBOTokens :exec
UPDATE qbo_connections SET
    access_token_enc = $2,
    refresh_token_enc = $3,
    token_expiry = $4,
    refresh_expiry = $5,
    updated_at = NOW()
WHERE id = $1;

-- name: DeactivateQBOConnection :exec
UPDATE qbo_connections SET
    is_active = false,
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateQBOSyncStatus :exec
UPDATE qbo_connections SET
    last_sync_at = NOW(),
    last_sync_status = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: CreateQBOSyncLog :one
INSERT INTO qbo_sync_logs (id, connection_id, company_id, entity_type, sync_type, sync_direction, started_at, status)
VALUES ($1, $2, $3, $4, $5, $6, NOW(), 'running')
RETURNING *;

-- name: CompleteQBOSyncLog :exec
UPDATE qbo_sync_logs SET
    completed_at = NOW(),
    records_synced = $2,
    records_failed = $3,
    error_details = $4,
    status = $5
WHERE id = $1;

-- name: ListQBOSyncLogs :many
SELECT * FROM qbo_sync_logs
WHERE company_id = $1
ORDER BY started_at DESC
LIMIT $2 OFFSET $3;

-- name: CountQBOSyncLogs :one
SELECT COUNT(*) FROM qbo_sync_logs WHERE company_id = $1;
