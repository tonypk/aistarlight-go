-- name: CreateActionPlan :one
INSERT INTO action_plans (id, thread_id, agent_id, company_id, user_id, tool_name, tool_args, summary, impact)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetActionPlan :one
SELECT * FROM action_plans WHERE id = $1;

-- name: GetActionPlanForUpdate :one
SELECT * FROM action_plans WHERE id = $1 FOR UPDATE;

-- name: UpdateActionPlanStatus :exec
UPDATE action_plans
SET status = @status, updated_at = NOW(),
    confirmed_at = CASE WHEN @status = 'confirmed' THEN NOW() ELSE confirmed_at END,
    executed_at = CASE WHEN @status = 'executed' THEN NOW() ELSE executed_at END,
    result = COALESCE(@result, result),
    error_message = COALESCE(@error_message, error_message)
WHERE id = @id;

-- name: ListPendingActionsByThread :many
SELECT * FROM action_plans
WHERE thread_id = $1 AND status = 'pending'
ORDER BY created_at DESC;

-- name: CountPendingActionsByThread :one
SELECT COUNT(*) FROM action_plans
WHERE thread_id = $1 AND status = 'pending';

-- name: TimeoutExpiredActions :execrows
UPDATE action_plans
SET status = 'timeout', updated_at = NOW()
WHERE status = 'pending' AND created_at < NOW() - INTERVAL '30 minutes';
