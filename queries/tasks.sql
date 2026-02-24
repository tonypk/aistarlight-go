-- name: CreateAsyncTask :one
INSERT INTO async_tasks (company_id, created_by, task_type, payload)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAsyncTask :one
SELECT * FROM async_tasks WHERE id = $1;

-- name: UpdateAsyncTaskStatus :exec
UPDATE async_tasks
SET status = $2,
    started_at = CASE WHEN $2 = 'processing' THEN NOW() ELSE started_at END,
    completed_at = CASE WHEN $2 IN ('completed', 'failed') THEN NOW() ELSE completed_at END
WHERE id = $1;

-- name: UpdateAsyncTaskProgress :exec
UPDATE async_tasks SET progress = $2 WHERE id = $1;

-- name: UpdateAsyncTaskResult :exec
UPDATE async_tasks
SET status = 'completed', result = $2, progress = 100, completed_at = NOW()
WHERE id = $1;

-- name: UpdateAsyncTaskError :exec
UPDATE async_tasks
SET status = 'failed', error_message = $2, completed_at = NOW()
WHERE id = $1;

-- name: ListAsyncTasksByCompany :many
SELECT * FROM async_tasks
WHERE company_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountAsyncTasksByCompany :one
SELECT COUNT(*) FROM async_tasks WHERE company_id = $1;

-- name: CleanupOldTasks :execrows
DELETE FROM async_tasks
WHERE status IN ('completed', 'failed')
AND completed_at < NOW() - INTERVAL '30 days';
