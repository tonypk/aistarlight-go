-- name: CreateUser :one
INSERT INTO users (id, email, hashed_password, full_name, api_key, is_active, created_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByAPIKey :one
SELECT * FROM users WHERE api_key = $1 AND is_active = true;

-- name: UpdateUser :exec
UPDATE users SET
    full_name = COALESCE($2, full_name),
    api_key = COALESCE($3, api_key),
    is_active = COALESCE($4, is_active)
WHERE id = $1;

-- name: SetAPIKey :exec
UPDATE users SET api_key = $2 WHERE id = $1;
