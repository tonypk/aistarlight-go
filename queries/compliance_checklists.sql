-- name: ListChecklistsByFormType :many
SELECT * FROM compliance_checklists
WHERE form_type = $1 AND is_active = true
ORDER BY sort_order;

-- name: ListAllChecklists :many
SELECT * FROM compliance_checklists
WHERE is_active = true AND jurisdiction = $1
ORDER BY form_type, sort_order;
