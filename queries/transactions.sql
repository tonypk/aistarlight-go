-- name: CreateTransaction :one
INSERT INTO transactions (
    id, company_id, session_id, source_type, source_file_id, row_index,
    date, description, amount, vat_amount, vat_type, category, tin,
    confidence, classification_source, raw_data, match_group_id, match_status,
    ewt_rate, ewt_amount, atc_code, vendor_id, project_tag,
    from_currency, to_currency, exchange_rate, from_amount,
    submitted_by,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
    $14, $15, $16, $17, $18, $19, $20, $21, $22, $23,
    $24, $25, $26, $27,
    $28,
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
    vendor_id = $11,
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

-- name: GetSpendingSummaryByCategory :many
SELECT category, COUNT(*) AS count, COALESCE(SUM(amount), 0)::text AS total
FROM transactions
WHERE company_id = $1 AND date >= $2 AND date <= $3
GROUP BY category ORDER BY SUM(amount) DESC NULLS LAST;

-- name: GetSpendingSummaryByMonth :many
SELECT TO_CHAR(date, 'YYYY-MM') AS month, COUNT(*) AS count, COALESCE(SUM(amount), 0)::text AS total
FROM transactions
WHERE company_id = $1 AND date >= $2 AND date <= $3
GROUP BY TO_CHAR(date, 'YYYY-MM') ORDER BY month;

-- name: SearchTransactionsByCompany :many
SELECT * FROM transactions
WHERE company_id = $1
  AND ($2::text = '' OR description ILIKE '%' || $2 || '%')
  AND ($3::date IS NULL OR date >= $3)
  AND ($4::date IS NULL OR date <= $4)
ORDER BY date DESC
LIMIT $5 OFFSET $6;

-- name: GetRecentTransactionsByCompany :many
SELECT * FROM transactions
WHERE company_id = $1
ORDER BY COALESCE(date, created_at) DESC
LIMIT $2;

-- name: GetIncomeExpenseSummary :many
SELECT
  CASE WHEN source_type IN ('sales_record', 'sales') THEN 'income' ELSE 'expense' END AS type,
  COUNT(*) AS count,
  COALESCE(SUM(amount), 0)::text AS total
FROM transactions
WHERE company_id = $1 AND date >= $2 AND date <= $3
GROUP BY CASE WHEN source_type IN ('sales_record', 'sales') THEN 'income' ELSE 'expense' END;

-- name: UpdateTransactionFields :one
UPDATE transactions SET
    amount = CASE WHEN @set_amount::boolean THEN @new_amount::numeric ELSE amount END,
    description = CASE WHEN @set_description::boolean THEN @new_description::text ELSE description END,
    date = CASE WHEN @set_date::boolean THEN @new_date::date ELSE date END,
    category = CASE WHEN @set_category::boolean THEN @new_category::varchar ELSE category END,
    vat_amount = CASE WHEN @set_vat_amount::boolean THEN @new_vat_amount::numeric ELSE vat_amount END,
    updated_at = NOW()
WHERE id = @id AND company_id = @company_id
RETURNING *;

-- name: DeleteTransactionByIDAndCompany :exec
DELETE FROM transactions WHERE id = $1 AND company_id = $2;

-- name: ListTransactionsWithSubmitter :many
SELECT t.*, COALESCE(NULLIF(u.full_name, ''), tu.full_name, tu.username, u.email) AS submitted_by_name
FROM transactions t
LEFT JOIN users u ON t.submitted_by = u.id
LEFT JOIN telegram_users tu ON tu.user_id = t.submitted_by
WHERE t.company_id = $1
  AND t.date >= $2
  AND t.date <= $3
ORDER BY t.date DESC, t.created_at DESC
LIMIT 10000;

-- name: ListCompanyTransactionsFiltered :many
SELECT t.*,
  COALESCE(NULLIF(u.full_name, ''), tu.full_name, tu.username, u.email) AS submitted_by_name,
  rb.image_path AS receipt_image_path,
  je.entry_number AS journal_entry_number
FROM transactions t
LEFT JOIN users u ON t.submitted_by = u.id
LEFT JOIN telegram_users tu ON tu.user_id = t.submitted_by
LEFT JOIN receipt_batches rb ON t.source_file_id = rb.id::text AND t.source_type = 'receipt'
LEFT JOIN journal_entries je ON t.journal_entry_id = je.id
WHERE t.company_id = $1
  AND ($4::varchar = '' OR $4::varchar IS NULL OR t.source_type = $4)
  AND ($5::varchar = '' OR $5::varchar IS NULL OR t.category = $5)
  AND ($6::text = '' OR $6::text IS NULL OR t.description ILIKE '%' || $6 || '%')
  AND ($7::date IS NULL OR t.date >= $7)
  AND ($8::date IS NULL OR t.date <= $8)
ORDER BY t.created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountCompanyTransactionsFiltered :one
SELECT COUNT(*) FROM transactions t
WHERE t.company_id = $1
  AND ($2::varchar = '' OR $2::varchar IS NULL OR t.source_type = $2)
  AND ($3::varchar = '' OR $3::varchar IS NULL OR t.category = $3)
  AND ($4::text = '' OR $4::text IS NULL OR t.description ILIKE '%' || $4 || '%')
  AND ($5::date IS NULL OR t.date >= $5)
  AND ($6::date IS NULL OR t.date <= $6);

-- name: GetSpendingSummaryByVendor :many
SELECT
  COALESCE(NULLIF(description, ''), 'Unknown')::text AS vendor,
  COUNT(*) AS count,
  COALESCE(SUM(amount), 0)::text AS total
FROM transactions
WHERE company_id = $1 AND date >= $2 AND date <= $3
GROUP BY COALESCE(NULLIF(description, ''), 'Unknown')::text
ORDER BY SUM(amount) DESC NULLS LAST
LIMIT 20;

-- name: GetTopVendorsByCount :many
SELECT
  COALESCE(NULLIF(description, ''), 'Unknown')::text AS vendor,
  COUNT(*) AS count,
  COALESCE(SUM(amount), 0)::text AS total
FROM transactions
WHERE company_id = $1 AND date >= $2 AND date <= $3
GROUP BY COALESCE(NULLIF(description, ''), 'Unknown')::text
ORDER BY count DESC
LIMIT 10;

-- name: CountUnmatchedTransactionsByCompany :one
SELECT COUNT(*) FROM transactions
WHERE company_id = $1 AND match_status = 'unmatched';

-- name: CountLowConfidenceTransactionsByCompany :one
SELECT COUNT(*) FROM transactions
WHERE company_id = $1 AND confidence < 0.5;
