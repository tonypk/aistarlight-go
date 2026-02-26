-- name: GetActiveRule :one
SELECT id, rule_type, rule_key, value, effective_from, effective_to, source_ref, description, created_at
FROM tax_rules
WHERE rule_type = $1
  AND rule_key = $2
  AND effective_from <= $3
  AND (effective_to IS NULL OR effective_to > $3)
ORDER BY effective_from DESC
LIMIT 1;

-- name: ListRulesByType :many
SELECT id, rule_type, rule_key, value, effective_from, effective_to, source_ref, description, created_at
FROM tax_rules
WHERE rule_type = $1
ORDER BY rule_key, effective_from DESC;

-- name: ListActiveRulesByType :many
SELECT id, rule_type, rule_key, value, effective_from, effective_to, source_ref, description, created_at
FROM tax_rules
WHERE rule_type = $1
  AND effective_from <= $2
  AND (effective_to IS NULL OR effective_to > $2)
ORDER BY rule_key;

-- name: GetFormSchemaByType :one
SELECT id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, created_at, updated_at
FROM form_schemas
WHERE form_type = $1 AND is_active = true
ORDER BY version DESC
LIMIT 1;

-- name: ListActiveFormSchemas :many
SELECT id, form_type, version, name, frequency, is_active, schema_def, calculation_rules, created_at, updated_at
FROM form_schemas
WHERE is_active = true
ORDER BY form_type;
