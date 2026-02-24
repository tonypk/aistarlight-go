-- name: CreateSupplier :one
INSERT INTO suppliers (id, company_id, tin, name, address, supplier_type, default_ewt_rate, default_atc_code, is_vat_registered, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
RETURNING *;

-- name: GetSupplierByID :one
SELECT * FROM suppliers WHERE id = $1;

-- name: UpdateSupplier :exec
UPDATE suppliers SET
    tin = $2, name = $3, address = $4, supplier_type = $5,
    default_ewt_rate = $6, default_atc_code = $7, is_vat_registered = $8,
    updated_at = NOW()
WHERE id = $1;

-- name: ListSuppliersByCompany :many
SELECT * FROM suppliers WHERE company_id = $1
ORDER BY name LIMIT $2 OFFSET $3;

-- name: CountSuppliersByCompany :one
SELECT COUNT(*) FROM suppliers WHERE company_id = $1;

-- name: GetSupplierByTIN :one
SELECT * FROM suppliers WHERE company_id = $1 AND tin = $2;

-- name: SearchSuppliersByCompany :many
SELECT * FROM suppliers
WHERE company_id = $1
  AND (name ILIKE '%' || $2 || '%' OR tin ILIKE '%' || $2 || '%')
ORDER BY name LIMIT $3 OFFSET $4;

-- name: DeleteSupplier :exec
DELETE FROM suppliers WHERE id = $1 AND company_id = $2;
