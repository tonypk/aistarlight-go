-- name: CreateVendor :one
INSERT INTO vendors (id, company_id, tin, name, address, vendor_type, default_ewt_rate, default_atc_code, is_vat_registered,
    email, phone, payment_terms_days, currency_code, default_account_id, is_active, notes,
    created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14, $15, $16,
    NOW(), NOW())
RETURNING *;

-- name: GetVendorByID :one
SELECT * FROM vendors WHERE id = $1;

-- name: UpdateVendor :exec
UPDATE vendors SET
    tin = $2, name = $3, address = $4, vendor_type = $5,
    default_ewt_rate = $6, default_atc_code = $7, is_vat_registered = $8,
    email = $9, phone = $10, payment_terms_days = $11, currency_code = $12,
    default_account_id = $13, is_active = $14, notes = $15,
    updated_at = NOW()
WHERE id = $1;

-- name: ListVendorsByCompany :many
SELECT * FROM vendors WHERE company_id = $1
ORDER BY name LIMIT $2 OFFSET $3;

-- name: CountVendorsByCompany :one
SELECT COUNT(*) FROM vendors WHERE company_id = $1;

-- name: GetVendorByTIN :one
SELECT * FROM vendors WHERE company_id = $1 AND tin = $2;

-- name: SearchVendorsByCompany :many
SELECT * FROM vendors
WHERE company_id = $1
  AND (name ILIKE '%' || $2 || '%' OR tin ILIKE '%' || $2 || '%')
ORDER BY name LIMIT $3 OFFSET $4;

-- name: DeleteVendor :exec
DELETE FROM vendors WHERE id = $1 AND company_id = $2;
