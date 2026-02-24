-- name: CreateAuditLog :one
INSERT INTO audit_logs (id, company_id, user_id, entity_type, entity_id, action, changes, comment, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
RETURNING *;

-- name: ListAuditLogsByCompany :many
SELECT * FROM audit_logs WHERE company_id = $1
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountAuditLogsByCompany :one
SELECT COUNT(*) FROM audit_logs WHERE company_id = $1;

-- name: ListAuditLogsByEntity :many
SELECT * FROM audit_logs WHERE entity_type = $1 AND entity_id = $2
ORDER BY created_at DESC;
