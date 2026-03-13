-- ---- User Preferences ----

-- name: UpsertUserPreference :exec
INSERT INTO user_preferences (id, company_id, report_type, column_mappings, format_rules, auto_fill_rules, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
ON CONFLICT (company_id, report_type) DO UPDATE SET
    column_mappings = EXCLUDED.column_mappings,
    format_rules = EXCLUDED.format_rules,
    auto_fill_rules = EXCLUDED.auto_fill_rules,
    updated_at = NOW();

-- name: GetUserPreferenceByCompanyAndType :one
SELECT * FROM user_preferences
WHERE company_id = $1 AND report_type = $2;

-- name: ListUserPreferencesByCompany :many
SELECT * FROM user_preferences
WHERE company_id = $1
ORDER BY updated_at DESC;

-- name: DeleteUserPreference :exec
DELETE FROM user_preferences
WHERE company_id = $1 AND report_type = $2;

-- ---- Receipt Batches ----

-- name: CreateReceiptBatch :one
INSERT INTO receipt_batches (id, company_id, user_id, status, total_images, processed_count, session_id, report_id, report_type, period, results, image_path, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())
RETURNING *;

-- name: GetReceiptBatchByID :one
SELECT * FROM receipt_batches WHERE id = $1;

-- name: UpdateReceiptBatch :exec
UPDATE receipt_batches SET
    status = COALESCE($2, status),
    processed_count = COALESCE($3, processed_count),
    results = COALESCE($4, results),
    error_message = $5,
    image_path = COALESCE($6, image_path),
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateReceiptBatchStatus :exec
UPDATE receipt_batches SET
    status = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: CancelStalePendingBatches :exec
UPDATE receipt_batches SET
    status = 'cancelled',
    updated_at = NOW()
WHERE status = 'pending_confirmation'
  AND created_at < NOW() - INTERVAL '10 minutes';

-- name: ListReceiptBatchesByCompany :many
SELECT * FROM receipt_batches WHERE company_id = $1
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountReceiptBatchesByCompany :one
SELECT COUNT(*) FROM receipt_batches WHERE company_id = $1;

-- name: UpdateReceiptBatchImageHash :exec
UPDATE receipt_batches SET
    image_hash = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: FindReceiptBatchByImageHash :one
SELECT id, status, created_at FROM receipt_batches
WHERE company_id = $1
  AND image_hash = $2
  AND status NOT IN ('cancelled', 'failed')
ORDER BY created_at DESC
LIMIT 1;

-- ---- Bank Reconciliation Batches ----

-- name: CreateBankReconBatch :one
INSERT INTO bank_reconciliation_batches (id, company_id, created_by, session_id, status, source_files, total_entries, amount_tolerance, date_tolerance_days, period, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
RETURNING *;

-- name: GetBankReconBatchByID :one
SELECT * FROM bank_reconciliation_batches WHERE id = $1;

-- name: UpdateBankReconBatch :exec
UPDATE bank_reconciliation_batches SET
    status = COALESCE($2, status),
    parse_summary = $3,
    match_result = $4,
    ai_suggestions = $5,
    ai_explanations = $6,
    total_entries = COALESCE($7, total_entries),
    error_message = $8,
    updated_at = NOW()
WHERE id = $1;

-- name: ListBankReconBatchesByCompany :many
SELECT * FROM bank_reconciliation_batches WHERE company_id = $1
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountBankReconBatchesByCompany :one
SELECT COUNT(*) FROM bank_reconciliation_batches WHERE company_id = $1;

-- ---- Revoked Tokens ----

-- name: CreateRevokedToken :exec
INSERT INTO revoked_tokens (id, jti, user_id, revoked_at, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: IsTokenRevoked :one
SELECT EXISTS(SELECT 1 FROM revoked_tokens WHERE jti = $1) AS is_revoked;

-- ---- Receipt Bridge Queries ----

-- name: LinkReceiptToTransactions :exec
UPDATE receipt_batches SET
    transaction_ids = $2,
    updated_at = NOW()
WHERE id = $1;
