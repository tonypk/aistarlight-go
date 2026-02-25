-- name: CreateLinkToken :one
INSERT INTO link_tokens (user_id, company_id, token, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetValidLinkToken :one
SELECT * FROM link_tokens
WHERE token = $1 AND used = false AND expires_at > NOW();

-- name: MarkLinkTokenUsed :exec
UPDATE link_tokens SET used = true WHERE id = $1;

-- name: DeleteExpiredTokens :execrows
DELETE FROM link_tokens WHERE expires_at < NOW() OR used = true;
