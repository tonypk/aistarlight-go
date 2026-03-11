-- name: CreateTransaction :one
INSERT INTO transactions (
    id, company_id, session_id, source_type, source_file_id, row_index,
    date, description, amount, vat_amount, vat_type, category, tin,
    confidence, classification_source, raw_data, match_group_id, match_status,
    ewt_rate, ewt_amount, atc_code, supplier_id, project_tag,
    from_currency, to_currency, exchange_rate, from_amount,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
    $14, $15, $16, $17, $18, $19, $20, $21, $22, $23,
    $24, $25, $26, $27,
    NOW(), NOW()
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

-- name: ListAllTransactionsBySession :many
SELECT * FROM transactions WHERE session_id = $1 ORDER BY row_index;

-- name: ListTransactionsBySessionFiltered :many
SELECT * FROM transactions
WHERE session_id = $1
  AND ($4::varchar = '' OR vat_type = $4)
  AND ($5::varchar = '' OR category = $5)
  AND ($6::varchar = '' OR source_type = $6)
  AND ($7::varchar = '' OR match_status = $7)
  AND ($8::numeric IS NULL OR confidence >= $8)
  AND ($9::text = '' OR description ILIKE '%' || $9 || '%')
ORDER BY row_index
LIMIT $2 OFFSET $3;

-- name: CountTransactionsBySessionFiltered :one
SELECT COUNT(*) FROM transactions
WHERE session_id = $1
  AND ($2::varchar = '' OR vat_type = $2)
  AND ($3::varchar = '' OR category = $3)
  AND ($4::varchar = '' OR source_type = $4)
  AND ($5::varchar = '' OR match_status = $5)
  AND ($6::numeric IS NULL OR confidence >= $6)
  AND ($7::text = '' OR description ILIKE '%' || $7 || '%');

-- name: BulkUpdateTransactionClassification :exec
UPDATE transactions SET
    vat_type = $2, category = $3, confidence = $4, classification_source = $5, updated_at = NOW()
WHERE id = $1;

-- name: UpdateTransactionMatch :exec
UPDATE transactions SET
    match_group_id = $2, match_status = $3, updated_at = NOW()
WHERE id = $1;

-- name: LinkTransactionToJournalEntry :exec
UPDATE transactions SET
    journal_entry_id = $2, updated_at = NOW()
WHERE id = $1;

-- name: GetUnlinkedTransactions :many
SELECT * FROM transactions
WHERE company_id = $1 AND journal_entry_id IS NULL
ORDER BY date ASC, row_index ASC
LIMIT $2 OFFSET $3;

-- name: GetTransactionsByIDs :many
SELECT * FROM transactions
WHERE id = ANY(@ids::uuid[]) AND company_id = @company_id
ORDER BY date ASC, row_index ASC;

-- name: UpdateTransactionDescription :exec
UPDATE transactions SET description = $2, updated_at = NOW() WHERE id = $1;

-- name: ListTransactionsByCompanyAndDateRange :many
SELECT * FROM transactions
WHERE company_id = $1
  AND date >= $2
  AND date <= $3
ORDER BY date DESC, created_at DESC
LIMIT 10000;
