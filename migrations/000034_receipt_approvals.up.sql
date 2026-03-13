-- Receipt-level approval workflow.

-- Per-company approval settings.
CREATE TABLE IF NOT EXISTS company_approval_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    is_enabled BOOLEAN NOT NULL DEFAULT false,
    amount_threshold NUMERIC(15,2) DEFAULT 10000,          -- require approval above this amount
    new_vendor_receipts INT DEFAULT 3,                     -- first N receipts from new vendor need approval
    risk_flags_require_approval BOOLEAN NOT NULL DEFAULT true,
    approver_user_id UUID REFERENCES users(id),            -- default approver
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id)
);

-- Individual receipt approval records.
CREATE TABLE IF NOT EXISTS receipt_approvals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id UUID NOT NULL REFERENCES receipt_batches(id) ON DELETE CASCADE,
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',   -- pending, approved, rejected
    trigger_reason TEXT NOT NULL,                     -- amount_threshold, new_vendor, risk_flag, manual
    requested_by UUID REFERENCES users(id),
    approved_by UUID REFERENCES users(id),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_receipt_approvals_batch ON receipt_approvals(batch_id);
CREATE INDEX idx_receipt_approvals_company_status ON receipt_approvals(company_id, status);

-- Track approval status on receipt batches.
ALTER TABLE receipt_batches ADD COLUMN IF NOT EXISTS approval_status VARCHAR(20) DEFAULT 'none';
