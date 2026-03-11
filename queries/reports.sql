-- name: CreateReport :one
INSERT INTO reports (id, company_id, report_type, period, status, input_data, calculated_data, file_path, created_at, created_by, version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, 1)
RETURNING *;

-- name: GetReportByID :one
SELECT * FROM reports WHERE id = $1;

-- name: UpdateReport :exec
UPDATE reports SET
    status = COALESCE($2, status),
    calculated_data = COALESCE($3, calculated_data),
    file_path = COALESCE($4, file_path),
    confirmed_at = $5,
    updated_by = $6,
    updated_at = NOW(),
    version = version + 1,
    overrides = COALESCE($7, overrides),
    original_calculated_data = COALESCE($8, original_calculated_data),
    notes = COALESCE($9, notes),
    compliance_score = $10
WHERE id = $1 AND version = $11;

-- name: DeleteReport :exec
DELETE FROM reports WHERE id = $1;

-- name: ListReportsByCompany :many
SELECT * FROM reports
WHERE company_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountReportsByCompany :one
SELECT COUNT(*) FROM reports WHERE company_id = $1;

-- name: ListReportsByCompanyAndType :many
SELECT * FROM reports
WHERE company_id = $1 AND report_type = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountReportsByCompanyAndType :one
SELECT COUNT(*) FROM reports WHERE company_id = $1 AND report_type = $2;

-- name: CreateReportWithAmendment :one
INSERT INTO reports (id, company_id, report_type, period, status, input_data, calculated_data, file_path, created_at, created_by, version, amendment_number, original_report_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, 1, $10, $11)
RETURNING *;

-- name: ListAmendmentChain :many
SELECT * FROM reports
WHERE (id = $1 OR original_report_id = $1)
ORDER BY amendment_number ASC;

-- name: GetMaxAmendmentNumber :one
SELECT COALESCE(MAX(amendment_number), 0)::int AS max_amendment
FROM reports
WHERE original_report_id = $1 OR id = $1;

-- name: ArchiveDuplicateReports :exec
-- Archives all draft/rejected reports for the same company+type+period except the given report ID.
UPDATE reports SET status = 'archived', updated_at = NOW()
WHERE company_id = $1
  AND report_type = $2
  AND period = $3
  AND id != $4
  AND status IN ('draft', 'rejected');
