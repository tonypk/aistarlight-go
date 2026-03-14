-- name: CreateTaxCode :one
INSERT INTO tax_codes (id, company_id, code, name, tax_type, rate, is_inclusive, affects_account_id, jurisdiction, is_active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
RETURNING *;

-- name: GetTaxCodeByID :one
SELECT * FROM tax_codes WHERE id = $1;

-- name: GetTaxCodeByCode :one
SELECT * FROM tax_codes WHERE company_id = $1 AND code = $2;

-- name: UpdateTaxCode :exec
UPDATE tax_codes SET
    name = $2, tax_type = $3, rate = $4, is_inclusive = $5,
    affects_account_id = $6, is_active = $7, updated_at = NOW()
WHERE id = $1;

-- name: ListTaxCodesByCompany :many
SELECT * FROM tax_codes
WHERE company_id = $1 AND is_active = true
ORDER BY code ASC;

-- name: ListAllTaxCodesByCompany :many
SELECT * FROM tax_codes
WHERE company_id = $1
ORDER BY code ASC
LIMIT $2 OFFSET $3;

-- name: CountTaxCodesByCompany :one
SELECT COUNT(*) FROM tax_codes WHERE company_id = $1;

-- name: DeleteTaxCode :exec
DELETE FROM tax_codes WHERE id = $1 AND company_id = $2;
