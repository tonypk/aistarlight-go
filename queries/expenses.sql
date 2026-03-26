-- queries/expenses.sql

-- ===================== EXPENSE POLICIES =====================

-- name: CreateExpensePolicy :one
INSERT INTO expense_policies (
    id, company_id, name, category, max_amount, requires_receipt_above,
    auto_approve_below, ai_auto_approve, description, is_active
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetExpensePolicyByID :one
SELECT * FROM expense_policies WHERE id = $1 AND company_id = $2;

-- name: ListExpensePolicies :many
SELECT * FROM expense_policies
WHERE company_id = $1 AND is_active = true
ORDER BY category, name;

-- name: ListExpensePoliciesByCategory :many
SELECT * FROM expense_policies
WHERE company_id = $1 AND category = $2 AND is_active = true
ORDER BY name;

-- name: UpdateExpensePolicy :exec
UPDATE expense_policies SET
    name = $3, category = $4, max_amount = $5, requires_receipt_above = $6,
    auto_approve_below = $7, ai_auto_approve = $8, description = $9,
    updated_at = now()
WHERE id = $1 AND company_id = $2;

-- name: DeactivateExpensePolicy :exec
UPDATE expense_policies SET is_active = false, updated_at = now()
WHERE id = $1 AND company_id = $2;

-- ===================== EXPENSE REPORTS =====================

-- name: CreateExpenseReport :one
INSERT INTO expense_reports (
    id, company_id, submitter_user_id, hr_payee_id, report_number,
    title, status, total_amount, currency, notes
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetExpenseReportByID :one
SELECT * FROM expense_reports WHERE id = $1 AND company_id = $2;

-- name: GetExpenseReportForUpdate :one
SELECT * FROM expense_reports WHERE id = $1 AND company_id = $2 FOR UPDATE;

-- name: ListExpenseReportsBySubmitter :many
SELECT * FROM expense_reports
WHERE company_id = $1 AND submitter_user_id = $2
    AND ($3::text = '' OR status = $3)
ORDER BY created_at DESC
LIMIT $4 OFFSET $5;

-- name: CountExpenseReportsBySubmitter :one
SELECT COUNT(*) FROM expense_reports
WHERE company_id = $1 AND submitter_user_id = $2
    AND ($3::text = '' OR status = $3);

-- name: ListExpenseReportsByStatus :many
SELECT * FROM expense_reports
WHERE company_id = $1 AND status = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountExpenseReportsByStatus :one
SELECT COUNT(*) FROM expense_reports
WHERE company_id = $1 AND status = $2;

-- name: UpdateExpenseReportDraft :exec
UPDATE expense_reports SET
    title = $3, notes = $4, updated_at = now()
WHERE id = $1 AND company_id = $2 AND status = 'draft';

-- name: UpdateExpenseReportStatus :exec
UPDATE expense_reports SET status = $3, updated_at = now()
WHERE id = $1 AND company_id = $2;

-- name: UpdateExpenseReportSubmit :exec
UPDATE expense_reports SET
    status = 'submitted', total_amount = $3, submitted_at = now(), updated_at = now()
WHERE id = $1 AND company_id = $2 AND status = 'draft';

-- name: UpdateExpenseReportAIReview :exec
UPDATE expense_reports SET
    status = $3, ai_reviewed_at = now(), ai_risk_score = $4,
    ai_decision = $5, ai_decision_reason = $6, updated_at = now()
WHERE id = $1 AND company_id = $2;

-- name: UpdateExpenseReportApprove :exec
UPDATE expense_reports SET
    status = 'approved', approver_user_id = $3, approved_at = now(),
    accrual_journal_entry_id = $4, updated_at = now()
WHERE id = $1 AND company_id = $2;

-- name: UpdateExpenseReportReject :exec
UPDATE expense_reports SET
    status = 'rejected', approver_user_id = $3, rejection_reason = $4,
    updated_at = now()
WHERE id = $1 AND company_id = $2;

-- name: UpdateExpenseReportRejectToDraft :exec
UPDATE expense_reports SET
    status = 'draft', rejection_reason = NULL, approver_user_id = NULL,
    ai_risk_score = NULL, ai_decision = NULL, ai_decision_reason = NULL,
    ai_reviewed_at = NULL, submitted_at = NULL, updated_at = now()
WHERE id = $1 AND company_id = $2 AND status = 'rejected';

-- name: UpdateExpenseReportPaid :exec
UPDATE expense_reports SET
    status = 'paid', reviewer_user_id = $3, paid_at = now(),
    payment_reference = $4, payment_journal_entry_id = $5, updated_at = now()
WHERE id = $1 AND company_id = $2 AND status = 'approved';

-- name: DeleteExpenseReportDraft :exec
DELETE FROM expense_reports WHERE id = $1 AND company_id = $2 AND status = 'draft';

-- Report number generation (advisory lock)
-- name: GenerateExpenseReportNumber :one
SELECT COALESCE(
    MAX(CAST(SUBSTRING(report_number FROM 'EXP-\d{6}-(\d+)') AS INT)), 0
) + 1 AS next_seq
FROM expense_reports
WHERE company_id = $1
    AND report_number LIKE 'EXP-' || to_char(now(), 'YYYYMM') || '-%';

-- name: AcquireExpenseReportNumberLock :exec
SELECT pg_advisory_xact_lock(hashtext($1::text || 'expense_report_number'));

-- ===================== EXPENSE ITEMS =====================

-- name: CreateExpenseItem :one
INSERT INTO expense_items (
    id, expense_report_id, category, description, amount, currency,
    merchant_name, transaction_date, receipt_url, receipt_ocr_data,
    ai_category_confidence, gl_account_id, policy_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetExpenseItemByID :one
SELECT ei.*, er.company_id, er.status AS report_status
FROM expense_items ei
JOIN expense_reports er ON er.id = ei.expense_report_id
WHERE ei.id = $1;

-- name: ListExpenseItemsByReport :many
SELECT * FROM expense_items
WHERE expense_report_id = $1
ORDER BY transaction_date, created_at;

-- name: CountExpenseItemsByReport :one
SELECT COUNT(*) FROM expense_items WHERE expense_report_id = $1;

-- name: SumExpenseItemsByReport :one
SELECT COALESCE(SUM(amount), 0)::NUMERIC(12,2) AS total
FROM expense_items WHERE expense_report_id = $1;

-- name: UpdateExpenseItem :exec
UPDATE expense_items SET
    category = $2, description = $3, amount = $4, merchant_name = $5,
    transaction_date = $6, gl_account_id = $7, policy_id = $8, updated_at = now()
WHERE id = $1;

-- name: UpdateExpenseItemReceipt :exec
UPDATE expense_items SET
    receipt_url = $2, receipt_ocr_data = $3, ai_category_confidence = $4,
    updated_at = now()
WHERE id = $1;

-- name: UpdateExpenseItemOCRFields :exec
UPDATE expense_items SET
    merchant_name = $2, amount = $3, transaction_date = $4, category = $5,
    ai_category_confidence = $6, receipt_ocr_data = $7, updated_at = now()
WHERE id = $1;

-- name: DeleteExpenseItem :exec
DELETE FROM expense_items WHERE id = $1;

-- Duplicate detection: same amount + merchant within 7 days
-- name: FindDuplicateExpenseItems :many
SELECT ei.id, ei.amount, ei.merchant_name, ei.transaction_date, er.report_number
FROM expense_items ei
JOIN expense_reports er ON er.id = ei.expense_report_id
WHERE er.company_id = $1
    AND er.submitter_user_id = $2
    AND ei.expense_report_id != $3
    AND ei.amount = $4
    AND ei.merchant_name = $5
    AND ei.transaction_date BETWEEN $6::date - INTERVAL '7 days' AND $6::date + INTERVAL '7 days'
    AND er.status NOT IN ('draft', 'rejected');

-- ===================== EXPENSE APPROVERS =====================

-- name: CreateExpenseApprover :one
INSERT INTO expense_approvers (
    id, company_id, department_name, approver_user_id, max_amount, priority, is_active
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetExpenseApproverByID :one
SELECT ea.*, u.email AS approver_email,
    COALESCE(u.full_name, u.email) AS approver_name
FROM expense_approvers ea
JOIN users u ON u.id = ea.approver_user_id
WHERE ea.id = $1 AND ea.company_id = $2;

-- name: ListExpenseApprovers :many
SELECT ea.*, u.email AS approver_email,
    COALESCE(u.full_name, u.email) AS approver_name
FROM expense_approvers ea
JOIN users u ON u.id = ea.approver_user_id
WHERE ea.company_id = $1 AND ea.is_active = true
ORDER BY ea.department_name, ea.priority;

-- name: UpdateExpenseApprover :exec
UPDATE expense_approvers SET
    department_name = $3, max_amount = $4, priority = $5, updated_at = now()
WHERE id = $1 AND company_id = $2;

-- name: DeactivateExpenseApprover :exec
UPDATE expense_approvers SET is_active = false, updated_at = now()
WHERE id = $1 AND company_id = $2;

-- Approver resolution: find eligible approvers for a department, excluding self
-- name: FindEligibleApprovers :many
SELECT ea.approver_user_id, ea.max_amount, ea.priority,
    COALESCE(u.full_name, u.email) AS approver_name
FROM expense_approvers ea
JOIN users u ON u.id = ea.approver_user_id
WHERE ea.company_id = $1
    AND ea.department_name = $2
    AND ea.approver_user_id != $3
    AND ea.is_active = true
    AND (ea.max_amount IS NULL OR ea.max_amount >= $4)
ORDER BY ea.priority ASC;

-- Fallback: find company_admins who are not the submitter
-- name: FindAdminApprovers :many
SELECT cm.user_id AS approver_user_id,
    COALESCE(u.full_name, u.email) AS approver_name
FROM company_members cm
JOIN users u ON u.id = cm.user_id
WHERE cm.company_id = $1
    AND cm.role = 'company_admin'
    AND cm.user_id != $2;

-- Approval queue: list pending reports for departments where user is an approver
-- name: ListPendingApprovalsForUser :many
SELECT er.*
FROM expense_reports er
JOIN hr_payees hp ON hp.id = er.hr_payee_id
WHERE er.company_id = $1
    AND er.status = 'pending_approval'
    AND er.submitter_user_id != $2
    AND hp.department_name IN (
        SELECT ea.department_name FROM expense_approvers ea
        WHERE ea.company_id = $1 AND ea.approver_user_id = $2 AND ea.is_active = true
    )
ORDER BY er.created_at ASC
LIMIT $3 OFFSET $4;

-- name: CountPendingApprovalsForUser :one
SELECT COUNT(*)
FROM expense_reports er
JOIN hr_payees hp ON hp.id = er.hr_payee_id
WHERE er.company_id = $1
    AND er.status = 'pending_approval'
    AND er.submitter_user_id != $2
    AND hp.department_name IN (
        SELECT ea.department_name FROM expense_approvers ea
        WHERE ea.company_id = $1 AND ea.approver_user_id = $2 AND ea.is_active = true
    );

-- ===================== EXPENSE AUDIT LOG =====================

-- name: CreateExpenseAuditLog :one
INSERT INTO expense_audit_log (
    id, expense_report_id, action, actor_user_id, actor_type, details
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListExpenseAuditLog :many
SELECT eal.*,
    COALESCE(u.full_name, u.email, '') AS actor_name
FROM expense_audit_log eal
LEFT JOIN users u ON u.id = eal.actor_user_id
WHERE eal.expense_report_id = $1
ORDER BY eal.created_at DESC;

-- ===================== FINANCE ANALYTICS =====================

-- name: GetExpenseSpendByCategory :many
SELECT ei.category, SUM(ei.amount) AS total_amount, COUNT(*) AS item_count
FROM expense_items ei
JOIN expense_reports er ON er.id = ei.expense_report_id
WHERE er.company_id = $1
    AND er.status IN ('approved', 'paid')
    AND er.approved_at >= $2 AND er.approved_at < $3
GROUP BY ei.category
ORDER BY total_amount DESC;

-- name: GetExpenseSpendByDepartment :many
SELECT COALESCE(hp.department_name, 'Unknown') AS department,
    SUM(er.total_amount) AS total_amount, COUNT(*) AS report_count
FROM expense_reports er
LEFT JOIN hr_payees hp ON hp.id = er.hr_payee_id
WHERE er.company_id = $1
    AND er.status IN ('approved', 'paid')
    AND er.approved_at >= $2 AND er.approved_at < $3
GROUP BY department
ORDER BY total_amount DESC;

-- name: GetExpenseSpendSummary :one
SELECT
    COUNT(*) FILTER (WHERE status = 'approved') AS approved_count,
    COUNT(*) FILTER (WHERE status = 'paid') AS paid_count,
    COUNT(*) FILTER (WHERE status = 'pending_approval') AS pending_count,
    COALESCE(SUM(total_amount) FILTER (WHERE status IN ('approved', 'paid')), 0) AS total_approved,
    COALESCE(SUM(total_amount) FILTER (WHERE status = 'paid'), 0) AS total_paid,
    COALESCE(SUM(total_amount) FILTER (WHERE status = 'pending_approval'), 0) AS total_pending
FROM expense_reports
WHERE company_id = $1
    AND created_at >= $2 AND created_at < $3;

-- Merchant classification cache lookup
-- name: GetMerchantCategory :one
SELECT category, COUNT(*) AS frequency
FROM expense_items ei
JOIN expense_reports er ON er.id = ei.expense_report_id
WHERE er.company_id = $1 AND ei.merchant_name = $2
GROUP BY category
ORDER BY frequency DESC
LIMIT 1;
