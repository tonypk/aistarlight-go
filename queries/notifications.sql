-- name: CreateNotification :one
INSERT INTO notifications (id, company_id, user_id, notification_type, title, message, metadata, dedup_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (company_id, dedup_key) DO NOTHING
RETURNING *;

-- name: ListNotificationsByCompany :many
SELECT * FROM notifications
WHERE company_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountNotificationsByCompany :one
SELECT COUNT(*) FROM notifications WHERE company_id = $1;

-- name: CountUnreadNotifications :one
SELECT COUNT(*) FROM notifications WHERE company_id = $1 AND NOT is_read;

-- name: MarkNotificationRead :exec
UPDATE notifications SET is_read = true, read_at = NOW()
WHERE id = $1 AND company_id = $2;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET is_read = true, read_at = NOW()
WHERE company_id = $1 AND NOT is_read;

-- name: CheckNotificationExists :one
SELECT EXISTS(
    SELECT 1 FROM notifications WHERE company_id = $1 AND dedup_key = $2
) AS exists;
