-- ---- Company Approval Settings ----

-- name: GetApprovalSettings :one
SELECT * FROM company_approval_settings WHERE company_id = $1;

-- name: UpsertApprovalSettings :one
INSERT INTO company_approval_settings (
    company_id, is_enabled, amount_threshold, new_vendor_receipts,
    risk_flags_require_approval, approver_user_id
) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (company_id) DO UPDATE SET
    is_enabled = EXCLUDED.is_enabled,
    amount_threshold = EXCLUDED.amount_threshold,
    new_vendor_receipts = EXCLUDED.new_vendor_receipts,
    risk_flags_require_approval = EXCLUDED.risk_flags_require_approval,
    approver_user_id = EXCLUDED.approver_user_id,
    updated_at = NOW()
RETURNING *;

-- ---- Receipt Approvals ----

-- name: CreateReceiptApproval :one
INSERT INTO receipt_approvals (
    batch_id, company_id, status, trigger_reason,
    requested_by, notes
) VALUES ($1, $2, 'pending', $3, $4, $5)
RETURNING *;

-- name: GetReceiptApproval :one
SELECT * FROM receipt_approvals WHERE id = $1;

-- name: GetReceiptApprovalByBatch :one
SELECT * FROM receipt_approvals WHERE batch_id = $1 ORDER BY created_at DESC LIMIT 1;

-- name: UpdateReceiptApprovalStatus :one
UPDATE receipt_approvals SET
    status = $2,
    approved_by = $3,
    notes = COALESCE($4, notes),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListPendingApprovals :many
SELECT ra.*, rb.results, rb.total_images, rb.period, rb.created_at AS batch_created_at
FROM receipt_approvals ra
JOIN receipt_batches rb ON ra.batch_id = rb.id
WHERE ra.company_id = $1 AND ra.status = 'pending'
ORDER BY ra.created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountPendingApprovals :one
SELECT COUNT(*) FROM receipt_approvals
WHERE company_id = $1 AND status = 'pending';

-- name: ListApprovalsByCompany :many
SELECT ra.*, rb.results, rb.total_images, rb.period
FROM receipt_approvals ra
JOIN receipt_batches rb ON ra.batch_id = rb.id
WHERE ra.company_id = $1
ORDER BY ra.created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountApprovalsByCompany :one
SELECT COUNT(*) FROM receipt_approvals WHERE company_id = $1;

-- name: UpdateBatchApprovalStatus :exec
UPDATE receipt_batches SET
    approval_status = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: CountVendorReceiptBatches :one
SELECT COUNT(*) FROM receipt_batches rb
JOIN transactions t ON t.source_file_id = rb.id::text AND t.source_type = 'receipt'
WHERE rb.company_id = $1
  AND t.description ILIKE '%' || $2 || '%'
  AND rb.status NOT IN ('cancelled', 'failed');
