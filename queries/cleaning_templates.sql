-- name: GetCleaningTemplateByHash :one
SELECT id, company_id, signature_hash, header_signature, ai_result, hit_count, created_at, updated_at
FROM cleaning_templates
WHERE company_id = $1 AND signature_hash = $2;

-- name: UpsertCleaningTemplate :one
INSERT INTO cleaning_templates (company_id, signature_hash, header_signature, ai_result)
VALUES ($1, $2, $3, $4)
ON CONFLICT (company_id, signature_hash)
DO UPDATE SET
    ai_result = EXCLUDED.ai_result,
    hit_count = cleaning_templates.hit_count + 1,
    updated_at = NOW()
RETURNING id, company_id, signature_hash, header_signature, ai_result, hit_count, created_at, updated_at;

-- name: IncrementCleaningTemplateHitCount :exec
UPDATE cleaning_templates
SET hit_count = hit_count + 1, updated_at = NOW()
WHERE id = $1;

-- name: ListCleaningTemplatesByCompany :many
SELECT id, company_id, signature_hash, header_signature, ai_result, hit_count, created_at, updated_at
FROM cleaning_templates
WHERE company_id = $1
ORDER BY hit_count DESC, updated_at DESC
LIMIT $2;
