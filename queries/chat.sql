-- name: CreateChatMessage :one
INSERT INTO chat_messages (id, company_id, user_id, role, content, tool_calls, created_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
RETURNING *;

-- name: ListChatMessagesByCompany :many
SELECT * FROM chat_messages
WHERE company_id = $1
ORDER BY created_at DESC
LIMIT $2;
