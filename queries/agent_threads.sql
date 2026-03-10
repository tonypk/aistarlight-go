-- name: CreateAgentThread :one
INSERT INTO agent_threads (id, company_id, user_id, agent_id, workflow_type, entity_type, entity_id, title, status, context_json)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', $9)
RETURNING *;

-- name: GetAgentThread :one
SELECT * FROM agent_threads WHERE id = $1;

-- name: FindAgentThread :one
SELECT * FROM agent_threads
WHERE company_id = $1
  AND agent_id = $2
  AND ($3 = '' OR entity_type = $3)
  AND ($4::UUID IS NULL OR entity_id = $4)
  AND status = 'active'
ORDER BY updated_at DESC
LIMIT 1;

-- name: ListAgentThreads :many
SELECT * FROM agent_threads
WHERE company_id = $1
  AND ($2 = '' OR agent_id = $2)
  AND status = 'active'
ORDER BY updated_at DESC
LIMIT $3 OFFSET $4;

-- name: UpdateAgentThread :exec
UPDATE agent_threads
SET title = COALESCE(NULLIF($2, ''), title),
    context_json = COALESCE($3, context_json),
    updated_at = NOW()
WHERE id = $1;

-- name: ArchiveAgentThread :exec
UPDATE agent_threads SET status = 'archived', updated_at = NOW() WHERE id = $1;

-- name: CreateAgentMessage :one
INSERT INTO chat_messages (id, company_id, user_id, role, content, tool_calls, thread_id, agent_id, message_type, citations_json, action_results_json, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
RETURNING *;

-- name: ListMessagesByThread :many
SELECT * FROM chat_messages
WHERE thread_id = $1
ORDER BY created_at ASC
LIMIT $2;

-- name: CreateAgentActionLog :one
INSERT INTO agent_action_logs (id, thread_id, company_id, user_id, agent_id, action_name, action_input, action_result, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;
