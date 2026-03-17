-- name: CreateIntegrationSource :one
INSERT INTO integration_sources (
    company_id, source_system, remote_company_id,
    api_key_hash, webhook_secret, status
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetIntegrationSource :one
SELECT * FROM integration_sources
WHERE company_id = $1 AND source_system = $2;

-- name: GetIntegrationSourceByID :one
SELECT * FROM integration_sources
WHERE id = $1;

-- name: UpdateIntegrationSourceStatus :exec
UPDATE integration_sources
SET status = $3, updated_at = NOW()
WHERE id = $1 AND company_id = $2;

-- name: UpdateIntegrationSourceLastEvent :exec
UPDATE integration_sources
SET last_event_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: DeleteIntegrationSource :exec
DELETE FROM integration_sources
WHERE id = $1 AND company_id = $2;

-- name: ListIntegrationSources :many
SELECT * FROM integration_sources
WHERE company_id = $1
ORDER BY created_at DESC;

-- name: InsertEventInbox :one
INSERT INTO integration_event_inbox (
    company_id, source_system, event_id, event_type, payload
) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (source_system, event_id) DO NOTHING
RETURNING *;

-- name: ListPendingInboxEvents :many
SELECT * FROM integration_event_inbox
WHERE status IN ('received', 'failed')
ORDER BY created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkInboxProcessed :exec
UPDATE integration_event_inbox
SET status = 'processed', processed_at = NOW(), error_message = NULL
WHERE id = $1;

-- name: MarkInboxFailed :exec
UPDATE integration_event_inbox
SET status = 'failed', error_message = $2
WHERE id = $1;

-- name: ListInboxByCompany :many
SELECT * FROM integration_event_inbox
WHERE company_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CreateGLMappingRule :one
INSERT INTO gl_mapping_rules (
    company_id, jurisdiction, source_dimension, source_value,
    target_account_id, debit_credit, priority, effective_from
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetGLMappingRule :one
SELECT * FROM gl_mapping_rules
WHERE id = $1 AND company_id = $2;

-- name: UpdateGLMappingRule :one
UPDATE gl_mapping_rules
SET target_account_id = $3,
    debit_credit = $4,
    priority = $5,
    effective_to = $6,
    is_active = $7,
    updated_at = NOW()
WHERE id = $1 AND company_id = $2
RETURNING *;

-- name: DeleteGLMappingRule :exec
DELETE FROM gl_mapping_rules
WHERE id = $1 AND company_id = $2;

-- name: ListGLMappingRules :many
SELECT g.*, a.account_number, a.name as account_name
FROM gl_mapping_rules g
JOIN accounts a ON a.id = g.target_account_id
WHERE g.company_id = $1
  AND g.is_active = TRUE
  AND (g.jurisdiction = $2 OR $2 = '')
ORDER BY g.source_dimension, g.priority DESC, g.effective_from DESC;

-- name: GetActiveGLMappings :many
SELECT g.*, a.account_number, a.name as account_name
FROM gl_mapping_rules g
JOIN accounts a ON a.id = g.target_account_id
WHERE g.company_id = $1
  AND g.jurisdiction = $2
  AND g.is_active = TRUE
  AND g.effective_from <= $3::date
  AND (g.effective_to IS NULL OR g.effective_to >= $3::date)
ORDER BY g.source_dimension, g.priority DESC;

-- name: UpsertHRPayee :one
INSERT INTO hr_payees (
    company_id, hr_employee_id, hr_employee_no,
    first_name, last_name, email, tin, sss, philhealth, pagibig,
    department_name, position_title, jurisdiction, status
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (company_id, hr_employee_id)
DO UPDATE SET
    hr_employee_no = EXCLUDED.hr_employee_no,
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    email = EXCLUDED.email,
    tin = EXCLUDED.tin,
    sss = EXCLUDED.sss,
    philhealth = EXCLUDED.philhealth,
    pagibig = EXCLUDED.pagibig,
    department_name = EXCLUDED.department_name,
    position_title = EXCLUDED.position_title,
    status = EXCLUDED.status,
    updated_at = NOW()
RETURNING *;

-- name: GetHRPayeeByEmployeeID :one
SELECT * FROM hr_payees
WHERE company_id = $1 AND hr_employee_id = $2;

-- name: ListHRPayees :many
SELECT * FROM hr_payees
WHERE company_id = $1
  AND (status = $2 OR $2 = '')
ORDER BY last_name, first_name
LIMIT $3 OFFSET $4;
