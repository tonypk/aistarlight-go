-- name: CreateWithholdingCertificate :one
INSERT INTO withholding_certificates (id, company_id, session_id, supplier_id, period, quarter, atc_code, income_type, income_amount, ewt_rate, tax_withheld, status, file_path, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
RETURNING *;

-- name: GetWithholdingCertificateByID :one
SELECT * FROM withholding_certificates WHERE id = $1;

-- name: UpdateWithholdingCertificate :exec
UPDATE withholding_certificates SET
    status = COALESCE($2, status), file_path = $3, updated_at = NOW()
WHERE id = $1;

-- name: ListWithholdingByCompany :many
SELECT * FROM withholding_certificates WHERE company_id = $1
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountWithholdingByCompany :one
SELECT COUNT(*) FROM withholding_certificates WHERE company_id = $1;

-- name: ListWithholdingBySupplier :many
SELECT * FROM withholding_certificates
WHERE supplier_id = $1 AND period = $2
ORDER BY created_at;
