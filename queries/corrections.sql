-- name: CreateCorrection :one
INSERT INTO corrections (id, company_id, user_id, entity_type, entity_id, field_name, old_value, new_value, reason, context_data, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
RETURNING *;

-- name: ListCorrectionsByEntity :many
SELECT * FROM corrections WHERE entity_type = $1 AND entity_id = $2
ORDER BY created_at DESC;

-- name: ListCorrectionsByCompany :many
SELECT * FROM corrections WHERE company_id = $1
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountCorrectionsByCompany :one
SELECT COUNT(*) FROM corrections WHERE company_id = $1;

-- name: CreateCorrectionRule :one
INSERT INTO correction_rules (id, company_id, rule_type, match_criteria, correction_field, correction_value, confidence, source_correction_count, is_active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
RETURNING *;

-- name: UpdateCorrectionRule :exec
UPDATE correction_rules SET
    match_criteria = COALESCE($2, match_criteria),
    correction_value = COALESCE($3, correction_value),
    confidence = COALESCE($4, confidence),
    source_correction_count = COALESCE($5, source_correction_count),
    is_active = COALESCE($6, is_active),
    updated_at = NOW()
WHERE id = $1;

-- name: GetCorrectionRuleByID :one
SELECT * FROM correction_rules WHERE id = $1;

-- name: ListActiveCorrectionRulesByCompany :many
SELECT * FROM correction_rules
WHERE company_id = $1 AND is_active = true
ORDER BY confidence DESC;

-- name: FindMatchingCorrectionRules :many
SELECT * FROM correction_rules
WHERE company_id = $1 AND rule_type = $2 AND correction_field = $3 AND is_active = true
ORDER BY confidence DESC;

-- name: CreateValidationResult :one
INSERT INTO validation_results (id, report_id, company_id, overall_score, check_results, rag_findings, validated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
RETURNING *;

-- name: GetLatestValidationByReport :one
SELECT * FROM validation_results WHERE report_id = $1
ORDER BY validated_at DESC LIMIT 1;

-- name: ListValidationsByReport :many
SELECT * FROM validation_results WHERE report_id = $1
ORDER BY validated_at DESC;
