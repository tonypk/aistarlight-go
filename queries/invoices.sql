-- name: CreateInvoice :one
INSERT INTO invoices (
    company_id, invoice_number, invoice_type, status,
    customer_name, customer_tin, customer_address,
    invoice_date, due_date,
    subtotal, vat_amount, discount_amount, total_amount,
    vatable_sales, vat_exempt_sales, zero_rated_sales,
    reference_number, notes, vendor_id, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
) RETURNING *;

-- name: GetInvoiceByID :one
SELECT * FROM invoices WHERE id = $1 AND company_id = $2;

-- name: ListInvoicesByCompany :many
SELECT * FROM invoices
WHERE company_id = $1
  AND ($4::varchar = '' OR $4::varchar IS NULL OR status = $4)
  AND ($5::varchar = '' OR $5::varchar IS NULL OR invoice_type = $5)
ORDER BY invoice_date DESC
LIMIT $2 OFFSET $3;

-- name: CountInvoicesByCompany :one
SELECT COUNT(*) FROM invoices
WHERE company_id = $1
  AND ($2::varchar = '' OR $2::varchar IS NULL OR status = $2)
  AND ($3::varchar = '' OR $3::varchar IS NULL OR invoice_type = $3);

-- name: UpdateInvoice :one
UPDATE invoices SET
    invoice_number = COALESCE($3, invoice_number),
    invoice_type = COALESCE($4, invoice_type),
    status = COALESCE($5, status),
    customer_name = COALESCE($6, customer_name),
    customer_tin = COALESCE($7, customer_tin),
    customer_address = COALESCE($8, customer_address),
    invoice_date = COALESCE($9, invoice_date),
    due_date = COALESCE($10, due_date),
    subtotal = COALESCE($11, subtotal),
    vat_amount = COALESCE($12, vat_amount),
    discount_amount = COALESCE($13, discount_amount),
    total_amount = COALESCE($14, total_amount),
    vatable_sales = COALESCE($15, vatable_sales),
    vat_exempt_sales = COALESCE($16, vat_exempt_sales),
    zero_rated_sales = COALESCE($17, zero_rated_sales),
    reference_number = COALESCE($18, reference_number),
    notes = COALESCE($19, notes),
    updated_at = NOW()
WHERE id = $1 AND company_id = $2
RETURNING *;

-- name: UpdateInvoiceStatus :exec
UPDATE invoices SET status = $3, updated_at = NOW()
WHERE id = $1 AND company_id = $2;

-- name: UpdateInvoiceEISStatus :exec
UPDATE invoices SET
    eis_submission_id = $3,
    eis_status = $4,
    eis_submitted_at = $5,
    eis_response = $6,
    updated_at = NOW()
WHERE id = $1 AND company_id = $2;

-- name: DeleteInvoice :exec
DELETE FROM invoices WHERE id = $1 AND company_id = $2;

-- name: CreateInvoiceItem :one
INSERT INTO invoice_items (
    invoice_id, line_number, description, quantity, unit_price,
    amount, vat_type, vat_rate, vat_amount, discount, atc_code
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: ListInvoiceItems :many
SELECT * FROM invoice_items WHERE invoice_id = $1 ORDER BY line_number;

-- name: DeleteInvoiceItems :exec
DELETE FROM invoice_items WHERE invoice_id = $1;

-- name: GetNextInvoiceNumber :one
SELECT COALESCE(MAX(CAST(REGEXP_REPLACE(invoice_number, '[^0-9]', '', 'g') AS INTEGER)), 0) + 1 AS next_number
FROM invoices WHERE company_id = $1;

-- name: CountInvoicesByEISStatus :many
SELECT COALESCE(eis_status, 'not_submitted') AS eis_status, COUNT(*) AS count
FROM invoices
WHERE company_id = $1
GROUP BY COALESCE(eis_status, 'not_submitted');
