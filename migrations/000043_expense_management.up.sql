-- Expense policies (company-configurable rules)
CREATE TABLE expense_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    max_amount NUMERIC(12,2),
    requires_receipt_above NUMERIC(12,2) DEFAULT 0,
    auto_approve_below NUMERIC(12,2),
    ai_auto_approve BOOLEAN NOT NULL DEFAULT false,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_expense_policies_company ON expense_policies(company_id) WHERE is_active;

-- Expense reports (one per submission)
CREATE TABLE expense_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    submitter_user_id UUID NOT NULL REFERENCES users(id),
    hr_payee_id UUID REFERENCES hr_payees(id),
    report_number TEXT NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    total_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'PHP',
    submitted_at TIMESTAMPTZ,
    ai_reviewed_at TIMESTAMPTZ,
    ai_risk_score INTEGER,
    ai_decision TEXT,
    ai_decision_reason TEXT,
    approver_user_id UUID REFERENCES users(id),
    approved_at TIMESTAMPTZ,
    rejection_reason TEXT,
    reviewer_user_id UUID REFERENCES users(id),
    paid_at TIMESTAMPTZ,
    payment_reference TEXT,
    accrual_journal_entry_id UUID REFERENCES journal_entries(id),
    payment_journal_entry_id UUID REFERENCES journal_entries(id),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_expense_report_number UNIQUE (company_id, report_number),
    CHECK (currency = 'PHP')
);
CREATE INDEX idx_expense_reports_company_status ON expense_reports(company_id, status);
CREATE INDEX idx_expense_reports_submitter ON expense_reports(submitter_user_id, created_at DESC);

-- Expense items (line items within a report)
CREATE TABLE expense_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    expense_report_id UUID NOT NULL REFERENCES expense_reports(id) ON DELETE CASCADE,
    category TEXT NOT NULL,
    description TEXT NOT NULL,
    amount NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL DEFAULT 'PHP' CHECK (currency = 'PHP'),
    merchant_name TEXT,
    transaction_date DATE NOT NULL,
    receipt_url TEXT,
    receipt_ocr_data JSONB,
    ai_category_confidence NUMERIC(3,2),
    gl_account_id UUID REFERENCES accounts(id),
    policy_id UUID REFERENCES expense_policies(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_expense_items_report ON expense_items(expense_report_id);

-- Expense approvers (per-department config)
CREATE TABLE expense_approvers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    department_name TEXT NOT NULL,
    approver_user_id UUID NOT NULL REFERENCES users(id),
    max_amount NUMERIC(12,2),
    priority INTEGER NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_expense_approver UNIQUE (company_id, department_name, approver_user_id)
);
CREATE INDEX idx_expense_approvers_dept ON expense_approvers(company_id, department_name) WHERE is_active;

-- Expense audit log (immutable trail)
CREATE TABLE expense_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    expense_report_id UUID REFERENCES expense_reports(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    actor_user_id UUID REFERENCES users(id),
    actor_type TEXT NOT NULL DEFAULT 'user',
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_expense_audit_report ON expense_audit_log(expense_report_id, created_at);
