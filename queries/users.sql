-- name: CreateUser :one
INSERT INTO users (id, email, hashed_password, full_name, api_key, is_active, telegram_username, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
RETURNING id, email, hashed_password, full_name, api_key, is_active, telegram_username, created_at;

-- name: GetUserByID :one
SELECT id, email, hashed_password, full_name, api_key, is_active, telegram_username, created_at FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT id, email, hashed_password, full_name, api_key, is_active, telegram_username, created_at FROM users WHERE email = $1;

-- name: GetUserByAPIKey :one
SELECT id, email, hashed_password, full_name, api_key, is_active, telegram_username, created_at FROM users WHERE api_key = $1 AND is_active = true;

-- name: GetUserByTelegramUsername :one
SELECT u.id, u.email, u.full_name, u.api_key, u.is_active, u.telegram_username, cm.company_id
FROM users u
JOIN company_members cm ON u.id = cm.user_id
WHERE LOWER(u.telegram_username) = LOWER($1)
  AND u.is_active = true
ORDER BY cm.joined_at ASC
LIMIT 1;

-- name: UpdateUser :exec
UPDATE users SET
    full_name = COALESCE($2, full_name),
    api_key = COALESCE($3, api_key),
    is_active = COALESCE($4, is_active)
WHERE id = $1;

-- name: SetAPIKey :exec
UPDATE users SET api_key = $2 WHERE id = $1;
