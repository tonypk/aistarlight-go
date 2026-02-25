-- name: UpsertTelegramUser :one
INSERT INTO telegram_users (telegram_id, user_id, company_id, chat_id, username, full_name)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (telegram_id) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    company_id = EXCLUDED.company_id,
    chat_id = EXCLUDED.chat_id,
    username = EXCLUDED.username,
    full_name = EXCLUDED.full_name
RETURNING *;

-- name: GetTelegramUser :one
SELECT * FROM telegram_users WHERE telegram_id = $1;

-- name: GetFirstCompanyByUser :one
SELECT c.* FROM companies c
JOIN company_members cm ON c.id = cm.company_id
WHERE cm.user_id = $1 AND c.is_active = true
ORDER BY cm.joined_at ASC
LIMIT 1;

-- name: GetActiveSessionByCompanyAndPeriod :one
SELECT * FROM reconciliation_sessions
WHERE company_id = $1 AND period = $2 AND status IN ('active', 'in_progress')
ORDER BY created_at DESC
LIMIT 1;

-- name: GetTransactionStatsSince :one
SELECT
    COUNT(*) AS count,
    COALESCE(SUM(amount), 0::numeric) AS total_amount
FROM transactions
WHERE company_id = $1 AND created_at >= $2;
