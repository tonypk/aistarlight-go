-- name: GetActiveRule :one
SELECT * FROM tax_rules
WHERE rule_type = $1
  AND rule_key = $2
  AND effective_from <= $3
  AND (effective_to IS NULL OR effective_to > $3)
  AND jurisdiction = $4
ORDER BY effective_from DESC
LIMIT 1;

-- name: ListRulesByType :many
SELECT * FROM tax_rules
WHERE rule_type = $1 AND jurisdiction = $2
ORDER BY rule_key, effective_from DESC;

-- name: ListActiveRulesByType :many
SELECT * FROM tax_rules
WHERE rule_type = $1
  AND effective_from <= $2
  AND (effective_to IS NULL OR effective_to > $2)
  AND jurisdiction = $3
ORDER BY rule_key;

-- name: GetFormSchemaByType :one
SELECT * FROM form_schemas
WHERE form_type = $1 AND is_active = true
ORDER BY version DESC
LIMIT 1;

-- name: ListActiveFormSchemas :many
SELECT * FROM form_schemas
WHERE is_active = true AND jurisdiction = $1
ORDER BY form_type;
