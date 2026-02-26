-- name: CreateReportApproval :one
INSERT INTO report_approvals (id, report_id, user_id, from_status, to_status, action, comment)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListReportApprovals :many
SELECT ra.*, u.full_name AS user_name, u.email AS user_email
FROM report_approvals ra
JOIN users u ON ra.user_id = u.id
WHERE ra.report_id = $1
ORDER BY ra.created_at DESC;

-- name: CountReportApprovals :one
SELECT COUNT(*) FROM report_approvals WHERE report_id = $1;
