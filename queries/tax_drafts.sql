-- name: UpsertTaxDraft :one
INSERT INTO tax_drafts (company_id, form_type, period_start, period_end, result, triggered_by)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (company_id, form_type, period_start, period_end)
DO UPDATE SET result = $5, triggered_by = $6, created_at = now()
RETURNING *;

-- name: GetLatestTaxDraft :one
SELECT * FROM tax_drafts
WHERE company_id = $1 AND form_type = $2
ORDER BY period_end DESC
LIMIT 1;

-- name: GetTaxDraftByPeriod :one
SELECT * FROM tax_drafts
WHERE company_id = $1 AND form_type = $2 AND period_start = $3 AND period_end = $4;
