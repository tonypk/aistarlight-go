-- name: ListChecklistsByFormType :many
SELECT id, form_type, check_id, check_name, severity, description, rule_ref, is_active, sort_order, created_at
FROM compliance_checklists
WHERE form_type = $1 AND is_active = true
ORDER BY sort_order;

-- name: ListAllChecklists :many
SELECT id, form_type, check_id, check_name, severity, description, rule_ref, is_active, sort_order, created_at
FROM compliance_checklists
WHERE is_active = true
ORDER BY form_type, sort_order;
