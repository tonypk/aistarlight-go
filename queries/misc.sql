-- ---- Form Schemas ----

-- name: GetFormSchemaByType :one
SELECT * FROM form_schemas WHERE form_type = $1 AND is_active = true;

-- name: ListActiveFormSchemas :many
SELECT * FROM form_schemas WHERE is_active = true ORDER BY form_type;

-- name: UpsertFormSchema :exec
INSERT INTO form_schemas (id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
ON CONFLICT (form_type) DO UPDATE SET
    version = EXCLUDED.version,
    name = EXCLUDED.name,
    frequency = EXCLUDED.frequency,
    is_active = EXCLUDED.is_active,
    schema_def = EXCLUDED.schema_def,
    calculation_rules = EXCLUDED.calculation_rules,
    updated_at = NOW();

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
INSERT INTO receipt_batches (id, company_id, user_id, status, total_images, processed_count, session_id, report_id, report_type, period, results, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
RETURNING *;

-- name: GetReceiptBatchByID :one
SELECT * FROM receipt_batches WHERE id = $1;

-- name: UpdateReceiptBatch :exec
UPDATE receipt_batches SET
    status = COALESCE($2, status),
    processed_count = COALESCE($3, processed_count),
    results = COALESCE($4, results),
    error_message = $5,
    updated_at = NOW()
WHERE id = $1;

-- name: ListReceiptBatchesByCompany :many
SELECT * FROM receipt_batches WHERE company_id = $1
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountReceiptBatchesByCompany :one
SELECT COUNT(*) FROM receipt_batches WHERE company_id = $1;

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

-- name: CleanExpiredTokens :exec
DELETE FROM revoked_tokens WHERE expires_at < NOW();

-- ---- Correction History (legacy) ----

-- name: CreateCorrectionHistory :exec
INSERT INTO correction_history (id, company_id, report_type, field_name, old_value, new_value, reason, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW());
