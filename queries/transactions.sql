-- name: CreateTransaction :one
INSERT INTO transactions (
    id, company_id, session_id, source_type, source_file_id, row_index,
    date, description, amount, vat_amount, vat_type, category, tin,
    confidence, classification_source, raw_data, match_group_id, match_status,
    ewt_rate, ewt_amount, atc_code, supplier_id, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
    $14, $15, $16, $17, $18, $19, $20, $21, $22, NOW(), NOW()
) RETURNING *;

-- name: GetTransactionByID :one
SELECT * FROM transactions WHERE id = $1;

-- name: UpdateTransaction :exec
UPDATE transactions SET
    vat_type = COALESCE($2, vat_type),
    category = COALESCE($3, category),
    confidence = COALESCE($4, confidence),
    classification_source = COALESCE($5, classification_source),
    match_group_id = $6,
    match_status = COALESCE($7, match_status),
    ewt_rate = $8,
    ewt_amount = $9,
    atc_code = $10,
    supplier_id = $11,
    updated_at = NOW()
WHERE id = $1;

-- name: ListTransactionsBySession :many
SELECT * FROM transactions
WHERE session_id = $1
ORDER BY row_index
LIMIT $2 OFFSET $3;

-- name: CountTransactionsBySession :one
SELECT COUNT(*) FROM transactions WHERE session_id = $1;

-- name: ListTransactionsByCompany :many
SELECT * FROM transactions
WHERE company_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountTransactionsByCompany :one
SELECT COUNT(*) FROM transactions WHERE company_id = $1;

-- name: DeleteTransactionsBySession :exec
DELETE FROM transactions WHERE session_id = $1;
