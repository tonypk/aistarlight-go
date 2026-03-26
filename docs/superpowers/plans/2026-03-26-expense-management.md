# Expense Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Ramp-style expense management to finance.halaos.com — employee expense submission with receipt OCR, AI policy enforcement, two-level approval, GL journal entry generation, and finance payment queue.

**Architecture:** New `expenses` domain within the existing AIStarlight-Go monolith. Five new database tables, six new service files, one handler file, eight frontend views. AI features use existing Claude API client. GL entries use existing JournalService.Create(). Receipt files stored on local filesystem. All endpoints under `/api/v1/expenses/`.

**Tech Stack:** Go 1.26 (Gin, sqlc/pgx), PostgreSQL, Redis, Claude API (Vision for OCR), Vue3+TS+NaiveUI, ECharts

**Spec:** `docs/superpowers/specs/2026-03-26-expense-management-design.md`

---

## File Map

### Backend — Create

| File | Responsibility |
|------|---------------|
| `migrations/000043_expense_management.up.sql` | 5 tables + indexes |
| `migrations/000043_expense_management.down.sql` | Drop tables |
| `queries/expenses.sql` | All sqlc queries (~30) |
| `internal/domain/expense.go` | Domain structs + status constants |
| `internal/service/expense_policy_service.go` | Policy CRUD |
| `internal/service/expense_approver_service.go` | Approver CRUD + resolution |
| `internal/service/expense_report_service.go` | Report CRUD + submit + status |
| `internal/service/expense_receipt_service.go` | File upload + OCR trigger |
| `internal/service/expense_ai_service.go` | Policy agent + risk scoring + classification |
| `internal/service/expense_gl_service.go` | Accrual + payment GL entries |
| `internal/handler/expense_handler.go` | All ~25 HTTP endpoints |
| `internal/service/expense_policy_service_test.go` | Policy tests |
| `internal/service/expense_report_service_test.go` | Report + approval tests |
| `internal/service/expense_ai_service_test.go` | AI risk scoring tests |
| `internal/service/expense_gl_service_test.go` | GL generation tests |

### Backend — Modify

| File | Change |
|------|--------|
| `internal/handler/middleware/rbac.go:82-93` | Fix roleLevel — add member=2, bump admin=4, accountant=3 |
| `internal/domain/journal.go:19-27` | Add SourceExpenseReimbursement + SourceExpensePayment constants |
| `internal/handler/router.go:13-72` | Add Expense field to Router struct |
| `internal/handler/router.go:82-679` | Add expense route group in Setup() |
| `cmd/api/main.go:125-174` | Add Expense services to services struct |
| `cmd/api/main.go:176-277` | Wire expense services in newServices() |
| `cmd/api/main.go:279-330` | Add Expense handler to handlers struct |
| `cmd/api/main.go:356-423` | Wire expense handler in newHandlers() |
| `cmd/api/main.go:425-513` | Add Expense to Router wiring in newGinEngine() |

### Frontend — Create (in `../aistarlight/frontend/`)

| File | Responsibility |
|------|---------------|
| `src/api/expenses.ts` | API client methods |
| `src/types/expense.ts` | TypeScript interfaces |
| `src/views/expenses/ExpenseListView.vue` | My expense reports list |
| `src/views/expenses/ExpenseFormView.vue` | Create/edit report + items |
| `src/views/expenses/ExpenseDetailView.vue` | Report detail + audit log |
| `src/views/expenses/ApprovalQueueView.vue` | Pending approvals |
| `src/views/expenses/FinanceQueueView.vue` | Payment queue |
| `src/views/expenses/PolicyManageView.vue` | Admin policy CRUD |
| `src/views/expenses/ApproverManageView.vue` | Admin approver config |
| `src/views/expenses/ExpenseAnalyticsView.vue` | Spend analytics |
| `src/components/expenses/ReceiptUploader.vue` | Drag-drop receipt upload |
| `src/components/expenses/ExpenseItemForm.vue` | Line item form |
| `src/components/expenses/RiskBadge.vue` | Risk score badge |
| `src/components/expenses/ApprovalCard.vue` | Approve/reject card |
| `src/components/expenses/SpendChart.vue` | ECharts spend chart |

### Frontend — Modify (in `../aistarlight/frontend/`)

| File | Change |
|------|--------|
| `src/router/index.ts` | Add 8 expense routes |
| `src/components/layout/DashboardLayout.vue` | Add Expenses nav group |

---

## Tasks

### Task 1: Database Migration

**Files:**
- Create: `migrations/000043_expense_management.up.sql`
- Create: `migrations/000043_expense_management.down.sql`

- [ ] **Step 1: Write up migration**

```sql
-- migrations/000043_expense_management.up.sql

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
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_expense_audit_report ON expense_audit_log(expense_report_id, created_at);
```

- [ ] **Step 2: Write down migration**

```sql
-- migrations/000043_expense_management.down.sql
DROP TABLE IF EXISTS expense_audit_log;
DROP TABLE IF EXISTS expense_approvers;
DROP TABLE IF EXISTS expense_items;
DROP TABLE IF EXISTS expense_reports;
DROP TABLE IF EXISTS expense_policies;
```

- [ ] **Step 3: Commit**

```bash
git add migrations/000043_expense_management.up.sql migrations/000043_expense_management.down.sql
git commit -m "feat(expense): add migration for 5 expense tables"
```

---

### Task 2: RBAC Fix + Domain Models + Source Types

**Files:**
- Modify: `internal/handler/middleware/rbac.go:82-93`
- Modify: `internal/domain/journal.go:19-27`
- Create: `internal/domain/expense.go`

- [ ] **Step 1: Fix roleLevel in rbac.go**

Replace lines 82-93 in `internal/handler/middleware/rbac.go`:

```go
func roleLevel(r domain.CompanyRole) int {
	switch r {
	case domain.CompanyRoleAdmin:
		return 4
	case domain.CompanyRoleAccountant:
		return 3
	case domain.CompanyRoleMember:
		return 2
	case domain.CompanyRoleViewer:
		return 1
	default:
		return 0
	}
}
```

- [ ] **Step 2: Add journal source types**

Add to `internal/domain/journal.go` after line 27 (after `SourceYearEndClose`):

```go
	SourceExpenseReimbursement JournalSourceType = "expense_reimbursement"
	SourceExpensePayment       JournalSourceType = "expense_payment"
```

- [ ] **Step 3: Create domain/expense.go**

```go
// internal/domain/expense.go
package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// -- Expense report statuses --

type ExpenseReportStatus string

const (
	ExpenseStatusDraft           ExpenseReportStatus = "draft"
	ExpenseStatusSubmitted       ExpenseReportStatus = "submitted"
	ExpenseStatusPendingApproval ExpenseReportStatus = "pending_approval"
	ExpenseStatusApproved        ExpenseReportStatus = "approved"
	ExpenseStatusRejected        ExpenseReportStatus = "rejected"
	ExpenseStatusPaid            ExpenseReportStatus = "paid"
)

// -- AI decision types --

type ExpenseAIDecision string

const (
	AIDecisionAutoApproved ExpenseAIDecision = "auto_approved"
	AIDecisionNeedsReview  ExpenseAIDecision = "needs_review"
	AIDecisionHighRisk     ExpenseAIDecision = "high_risk"
)

// -- Expense categories --

const (
	CategoryMeals         = "meals"
	CategoryTransport     = "transport"
	CategoryOffice        = "office"
	CategoryTravel        = "travel"
	CategoryEntertainment = "entertainment"
	CategoryOther         = "other"
)

var ValidCategories = map[string]bool{
	CategoryMeals: true, CategoryTransport: true, CategoryOffice: true,
	CategoryTravel: true, CategoryEntertainment: true, CategoryOther: true,
}

// -- Audit log actions --

const (
	AuditActionCreated   = "created"
	AuditActionSubmitted = "submitted"
	AuditActionAIReview  = "ai_reviewed"
	AuditActionApproved  = "approved"
	AuditActionRejected  = "rejected"
	AuditActionPaid      = "paid"
	AuditActionEdited    = "edited"
)

// -- Domain structs --

type ExpensePolicy struct {
	ID                  uuid.UUID        `json:"id"`
	CompanyID           uuid.UUID        `json:"company_id"`
	Name                string           `json:"name"`
	Category            string           `json:"category"`
	MaxAmount           *decimal.Decimal `json:"max_amount,omitempty"`
	RequiresReceiptAbove *decimal.Decimal `json:"requires_receipt_above,omitempty"`
	AutoApproveBelow    *decimal.Decimal `json:"auto_approve_below,omitempty"`
	AIAutoApprove       bool             `json:"ai_auto_approve"`
	Description         *string          `json:"description,omitempty"`
	IsActive            bool             `json:"is_active"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

type ExpenseReport struct {
	ID                    uuid.UUID           `json:"id"`
	CompanyID             uuid.UUID           `json:"company_id"`
	SubmitterUserID       uuid.UUID           `json:"submitter_user_id"`
	HRPayeeID             *uuid.UUID          `json:"hr_payee_id,omitempty"`
	ReportNumber          string              `json:"report_number"`
	Title                 string              `json:"title"`
	Status                ExpenseReportStatus `json:"status"`
	TotalAmount           decimal.Decimal     `json:"total_amount"`
	Currency              string              `json:"currency"`
	SubmittedAt           *time.Time          `json:"submitted_at,omitempty"`
	AIReviewedAt          *time.Time          `json:"ai_reviewed_at,omitempty"`
	AIRiskScore           *int                `json:"ai_risk_score,omitempty"`
	AIDecision            *string             `json:"ai_decision,omitempty"`
	AIDecisionReason      *string             `json:"ai_decision_reason,omitempty"`
	ApproverUserID        *uuid.UUID          `json:"approver_user_id,omitempty"`
	ApprovedAt            *time.Time          `json:"approved_at,omitempty"`
	RejectionReason       *string             `json:"rejection_reason,omitempty"`
	ReviewerUserID        *uuid.UUID          `json:"reviewer_user_id,omitempty"`
	PaidAt                *time.Time          `json:"paid_at,omitempty"`
	PaymentReference      *string             `json:"payment_reference,omitempty"`
	AccrualJournalEntryID *uuid.UUID          `json:"accrual_journal_entry_id,omitempty"`
	PaymentJournalEntryID *uuid.UUID          `json:"payment_journal_entry_id,omitempty"`
	Notes                 *string             `json:"notes,omitempty"`
	CreatedAt             time.Time           `json:"created_at"`
	UpdatedAt             time.Time           `json:"updated_at"`
	// Joined fields
	Items    []ExpenseItem    `json:"items,omitempty"`
	AuditLog []ExpenseAudit   `json:"audit_log,omitempty"`
}

type ExpenseItem struct {
	ID                   uuid.UUID        `json:"id"`
	ExpenseReportID      uuid.UUID        `json:"expense_report_id"`
	Category             string           `json:"category"`
	Description          string           `json:"description"`
	Amount               decimal.Decimal  `json:"amount"`
	Currency             string           `json:"currency"`
	MerchantName         *string          `json:"merchant_name,omitempty"`
	TransactionDate      time.Time        `json:"transaction_date"`
	ReceiptURL           *string          `json:"receipt_url,omitempty"`
	ReceiptOCRData       []byte           `json:"receipt_ocr_data,omitempty"`
	AICategoryConfidence *decimal.Decimal `json:"ai_category_confidence,omitempty"`
	GLAccountID          *uuid.UUID       `json:"gl_account_id,omitempty"`
	PolicyID             *uuid.UUID       `json:"policy_id,omitempty"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

type ExpenseApprover struct {
	ID             uuid.UUID        `json:"id"`
	CompanyID      uuid.UUID        `json:"company_id"`
	DepartmentName string           `json:"department_name"`
	ApproverUserID uuid.UUID        `json:"approver_user_id"`
	MaxAmount      *decimal.Decimal `json:"max_amount,omitempty"`
	Priority       int              `json:"priority"`
	IsActive       bool             `json:"is_active"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	// Joined
	ApproverName  string `json:"approver_name,omitempty"`
	ApproverEmail string `json:"approver_email,omitempty"`
}

type ExpenseAudit struct {
	ID              uuid.UUID  `json:"id"`
	ExpenseReportID *uuid.UUID `json:"expense_report_id,omitempty"`
	Action          string     `json:"action"`
	ActorUserID     *uuid.UUID `json:"actor_user_id,omitempty"`
	ActorType       string     `json:"actor_type"`
	Details         []byte     `json:"details,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	// Joined
	ActorName string `json:"actor_name,omitempty"`
}

// -- Helpers --

// ValidStatusTransitions defines allowed state transitions.
var ValidStatusTransitions = map[ExpenseReportStatus][]ExpenseReportStatus{
	ExpenseStatusDraft:           {ExpenseStatusSubmitted},
	ExpenseStatusSubmitted:       {ExpenseStatusPendingApproval, ExpenseStatusApproved},
	ExpenseStatusPendingApproval: {ExpenseStatusApproved, ExpenseStatusRejected},
	ExpenseStatusRejected:        {ExpenseStatusDraft},
	ExpenseStatusApproved:        {ExpenseStatusPaid},
}

func CanTransition(from, to ExpenseReportStatus) bool {
	allowed, ok := ValidStatusTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd /Users/anna/Documents/aistarlight-go && go build ./internal/domain/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/handler/middleware/rbac.go internal/domain/journal.go internal/domain/expense.go
git commit -m "feat(expense): fix RBAC roleLevel, add domain models and journal source types"
```

---

### Task 3: sqlc Queries

**Files:**
- Create: `queries/expenses.sql`

**Context:** sqlc config at `sqlc.yaml` reads from `queries/` dir, outputs to `internal/repository/sqlc`. Use `:one`, `:many`, `:exec` annotations. After writing, run `~/go/bin/sqlc generate`.

- [ ] **Step 1: Write all expense queries**

```sql
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
```

- [ ] **Step 2: Run sqlc generate**

Run: `cd /Users/anna/Documents/aistarlight-go && ~/go/bin/sqlc generate`
Expected: No errors, new files in `internal/repository/sqlc/`

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/repository/sqlc/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add queries/expenses.sql internal/repository/sqlc/
git commit -m "feat(expense): add sqlc queries for expense management"
```

---

### Task 4: Expense Policy Service + Approver Service

**Files:**
- Create: `internal/service/expense_policy_service.go`
- Create: `internal/service/expense_approver_service.go`
- Create: `internal/service/expense_policy_service_test.go`

**Context:** Follow pattern from `internal/service/journal_service.go` — struct with `*sqlc.Queries`, input DTOs, domain conversion helpers.

- [ ] **Step 1: Write ExpensePolicyService**

`internal/service/expense_policy_service.go`:
- `NewExpensePolicyService(q *sqlc.Queries) *ExpensePolicyService`
- `Create(ctx, input CreatePolicyInput) (*domain.ExpensePolicy, error)` — validate category ∈ ValidCategories, create via sqlc
- `GetByID(ctx, id, companyID uuid.UUID) (*domain.ExpensePolicy, error)`
- `List(ctx, companyID uuid.UUID) ([]domain.ExpensePolicy, error)`
- `Update(ctx, id, companyID uuid.UUID, input UpdatePolicyInput) error`
- `Deactivate(ctx, id, companyID uuid.UUID) error`
- `toPolicy(db sqlc.ExpensePolicy) *domain.ExpensePolicy` — convert sqlc → domain

Input structs:

```go
type CreatePolicyInput struct {
    CompanyID           uuid.UUID
    Name                string
    Category            string
    MaxAmount           *decimal.Decimal
    RequiresReceiptAbove *decimal.Decimal
    AutoApproveBelow    *decimal.Decimal
    AIAutoApprove       bool
    Description         *string
}

type UpdatePolicyInput struct {
    Name                string
    Category            string
    MaxAmount           *decimal.Decimal
    RequiresReceiptAbove *decimal.Decimal
    AutoApproveBelow    *decimal.Decimal
    AIAutoApprove       bool
    Description         *string
}
```

- [ ] **Step 2: Write ExpenseApproverService**

`internal/service/expense_approver_service.go`:
- `NewExpenseApproverService(q *sqlc.Queries) *ExpenseApproverService`
- `Create(ctx, input CreateApproverInput) (*domain.ExpenseApprover, error)`
- `GetByID(ctx, id, companyID uuid.UUID) (*domain.ExpenseApprover, error)`
- `List(ctx, companyID uuid.UUID) ([]domain.ExpenseApprover, error)`
- `Update(ctx, id, companyID uuid.UUID, input UpdateApproverInput) error`
- `Deactivate(ctx, id, companyID uuid.UUID) error`
- `ResolveApprover(ctx, companyID, submitterUserID uuid.UUID, department string, amount decimal.Decimal) (*uuid.UUID, error)` — implements self-approval prevention + fallback logic from spec

ResolveApprover logic:
1. Call `FindEligibleApprovers(companyID, department, submitterUserID, amount)` — excludes self
2. If results, return first by priority
3. Else call `FindAdminApprovers(companyID, submitterUserID)` — excludes self
4. If results, return first admin
5. Else return nil (no eligible approver — hold in pending_approval)

- [ ] **Step 3: Write policy service tests**

`internal/service/expense_policy_service_test.go`:
- Test Create with valid/invalid category
- Test List returns active only
- Test Deactivate

Use mock sqlc.Queries pattern from existing test files.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ -run TestExpensePolicy -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/expense_policy_service.go internal/service/expense_approver_service.go internal/service/expense_policy_service_test.go
git commit -m "feat(expense): add policy and approver services with self-approval prevention"
```

---

### Task 5: Expense Report Service

**Files:**
- Create: `internal/service/expense_report_service.go`
- Create: `internal/service/expense_report_service_test.go`

**Context:** This is the core service. Needs `*sqlc.Queries`, `*pgxpool.Pool` (for transactions), `*event.Publisher`. Report number generation uses advisory lock within a transaction.

- [ ] **Step 1: Write ExpenseReportService**

`internal/service/expense_report_service.go`:

Key methods:
- `NewExpenseReportService(q, pool, publisher, approverSvc, aiSvc, glSvc)` — takes dependencies
- `Create(ctx, input) (*domain.ExpenseReport, error)` — generate report number in tx (advisory lock + sequence), create report, write audit log
- `GetByID(ctx, id, companyID) (*domain.ExpenseReport, error)` — fetch report + items + audit log
- `ListMy(ctx, companyID, userID, status, page, limit) ([]domain.ExpenseReport, int64, error)`
- `UpdateDraft(ctx, id, companyID, input) error` — only if status=draft
- `DeleteDraft(ctx, id, companyID) error` — only if status=draft
- `AddItem(ctx, reportID, companyID, input) (*domain.ExpenseItem, error)` — verify report is draft, count < 50
- `UpdateItem(ctx, itemID, input) error` — verify parent report is draft (join check via GetExpenseItemByID)
- `DeleteItem(ctx, itemID) error` — verify parent report is draft
- `Submit(ctx, id, companyID, userID) error` — verify ≥1 items, recompute total in tx (SUM with FOR UPDATE), call AI policy agent, set status

Report number generation pattern (inside Create, within transaction):
```go
// Advisory lock
qtx.AcquireExpenseReportNumberLock(ctx, companyID.String())
// Get next sequence
row, _ := qtx.GenerateExpenseReportNumber(ctx, companyID)
reportNumber := fmt.Sprintf("EXP-%s-%04d", time.Now().Format("200601"), row.NextSeq)
```

Submit flow:
```go
func (s *ExpenseReportService) Submit(ctx, id, companyID, userID) error {
    tx, _ := s.pool.Begin(ctx)
    defer tx.Rollback(ctx)
    qtx := s.q.WithTx(tx)

    // Lock report
    report, _ := qtx.GetExpenseReportForUpdate(ctx, ...)

    // Verify draft status
    if report.Status != "draft" { return ErrReportNotDraft }

    // Count items
    count, _ := qtx.CountExpenseItemsByReport(ctx, id)
    if count == 0 { return ErrReportNoItems }

    // Recompute total
    total, _ := qtx.SumExpenseItemsByReport(ctx, id)

    // Update to submitted
    qtx.UpdateExpenseReportSubmit(ctx, id, companyID, total)

    // Audit log
    qtx.CreateExpenseAuditLog(ctx, ...)

    tx.Commit(ctx)

    // AI policy agent (async-ish — runs after commit, updates report)
    go s.aiSvc.EvaluateReport(context.Background(), id, companyID)

    return nil
}
```

- [ ] **Step 2: Write report service tests**

`internal/service/expense_report_service_test.go`:
- TestCreate_GeneratesReportNumber
- TestSubmit_RecomputesTotal
- TestSubmit_RejectsEmptyReport (0 items → 422)
- TestAddItem_RejectIfNotDraft (→ 409)
- TestDeleteDraft_RejectIfSubmitted
- TestUpdateDraft_Success

- [ ] **Step 3: Run tests**

Run: `go test ./internal/service/ -run TestExpenseReport -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/service/expense_report_service.go internal/service/expense_report_service_test.go
git commit -m "feat(expense): add report service with submit flow and report number generation"
```

---

### Task 6: Receipt Upload + File Storage

**Files:**
- Create: `internal/service/expense_receipt_service.go`

**Context:** Follow pattern from existing `internal/handler/receipt_handler.go` for multipart upload. Store files at `{EXPENSE_UPLOAD_DIR}/receipts/{company_id}/{YYYY-MM}/{item_id}.{ext}`. Validate magic bytes for JPEG/PNG/PDF.

- [ ] **Step 1: Write ExpenseReceiptService**

`internal/service/expense_receipt_service.go`:

```go
type ExpenseReceiptService struct {
    q         *sqlc.Queries
    uploadDir string
}

func NewExpenseReceiptService(q *sqlc.Queries, uploadDir string) *ExpenseReceiptService

// ValidateReceiptFile checks magic bytes and size.
// Returns detected extension or error.
func (s *ExpenseReceiptService) ValidateReceiptFile(data []byte) (string, error)
    // JPEG: FF D8 FF
    // PNG: 89 50 4E 47
    // PDF: 25 50 44 46
    // Max 10MB

// SaveReceipt saves receipt file to disk and updates item.
// Enforces: 10MB per file, 100MB total per report (sum of all receipt files).
func (s *ExpenseReceiptService) SaveReceipt(ctx, itemID, companyID uuid.UUID, data []byte, ext string) (string, error)
    // 1. Look up parent report via item → calculate current total receipt size
    // 2. Check: currentTotal + len(data) <= 100MB, reject if exceeded
    // 3. Path: {uploadDir}/receipts/{companyID}/{YYYY-MM}/{itemID}.{ext}
    // 4. Create dirs, write file, update expense_items.receipt_url

// ServeReceipt reads receipt file from disk (for GET endpoint).
func (s *ExpenseReceiptService) GetReceiptPath(ctx, itemID uuid.UUID) (string, error)
```

Magic byte validation:
```go
var magicBytes = map[string][]byte{
    ".jpg": {0xFF, 0xD8, 0xFF},
    ".png": {0x89, 0x50, 0x4E, 0x47},
    ".pdf": {0x25, 0x50, 0x44, 0x46},
}

func (s *ExpenseReceiptService) ValidateReceiptFile(data []byte) (string, error) {
    if len(data) > 10*1024*1024 {
        return "", errors.New("file exceeds 10MB limit")
    }
    for ext, magic := range magicBytes {
        if len(data) >= len(magic) && bytes.Equal(data[:len(magic)], magic) {
            return ext, nil
        }
    }
    return "", errors.New("unsupported file type: only JPEG, PNG, PDF allowed")
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/service/expense_receipt_service.go
git commit -m "feat(expense): add receipt upload service with magic byte validation"
```

---

### Task 7: AI Services (OCR + Policy Agent + Classification)

**Files:**
- Create: `internal/service/expense_ai_service.go`
- Create: `internal/service/expense_ai_service_test.go`

**Context:** Uses existing Claude API client (`internal/platform/openai` — despite the name, it wraps Anthropic Claude). Redis for merchant classification cache.

- [ ] **Step 1: Write ExpenseAIService**

`internal/service/expense_ai_service.go`:

```go
type ExpenseAIService struct {
    q     *sqlc.Queries
    pool  *pgxpool.Pool
    ai    *oai.Client  // Claude API client
    redis *redis.Client
}

func NewExpenseAIService(q, pool, ai, redis)
```

Key methods:

**OCR (Receipt extraction)**:
```go
func (s *ExpenseAIService) ExtractReceipt(ctx, imageData []byte) (*OCRResult, error)
```
- Send image to Claude Vision API with extraction prompt
- Parse response into `OCRResult{MerchantName, Amount, Currency, Date, Description, Confidence}`
- Return nil error + empty fields if OCR fails (never block on OCR failure)

**Policy Agent (Risk scoring)**:
```go
func (s *ExpenseAIService) EvaluateReport(ctx, reportID, companyID uuid.UUID) error
```
- Load company policies
- Load report items
- For each item: check max_amount, receipt requirement, duplicate detection, weekend flag
- Compute aggregate risk_score (0-100) per spec formula:
  - Base: 10
  - +20 missing receipt above threshold
  - +15 exceeds policy max
  - +10 per duplicate candidate
  - +10 weekend/holiday
  - +15 category-merchant mismatch (use Claude)
- Determine decision: auto_approved / needs_review / high_risk
- Update report via `UpdateExpenseReportAIReview`
- If auto_approved: set status=approved, generate accrual GL entry via `glSvc.CreateAccrualEntry()`, link journal entry ID
- If needs_review/high_risk: set status=pending_approval
- Write audit log with actor_type='ai' (include risk_score and decision in details JSONB)

**Classification (Merchant → category)**:
```go
func (s *ExpenseAIService) ClassifyMerchant(ctx, companyID uuid.UUID, merchantName string) (string, float64, error)
```
- Check Redis cache `expense:merchants:{companyID}`
- If miss, check DB via `GetMerchantCategory`
- If miss, call Claude to classify
- Cache result in Redis (TTL 7 days)

- [ ] **Step 2: Write AI service tests**

`internal/service/expense_ai_service_test.go`:
- TestRiskScore_LowRisk (base 10, all clean → auto_approved)
- TestRiskScore_MissingReceipt (+20)
- TestRiskScore_ExceedsMax (+15)
- TestRiskScore_DuplicateFound (+10)
- TestRiskScore_Weekend (+10)
- TestRiskScore_HighRisk (score > 70)
- TestRiskScore_NoPolicies (all items pass with base score)

Test risk scoring with mock data (no actual Claude calls in unit tests).

- [ ] **Step 3: Run tests**

Run: `go test ./internal/service/ -run TestRiskScore -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/service/expense_ai_service.go internal/service/expense_ai_service_test.go
git commit -m "feat(expense): add AI service with OCR, policy agent risk scoring, and merchant classification"
```

---

### Task 8: GL Journal Entry Generation

**Files:**
- Create: `internal/service/expense_gl_service.go`
- Create: `internal/service/expense_gl_service_test.go`

**Context:** Uses existing `JournalService.Create()` with `CreateJournalEntryInput`. Two entry types: accrual on approval, payment on mark-paid.

- [ ] **Step 1: Write ExpenseGLService**

`internal/service/expense_gl_service.go`:

```go
type ExpenseGLService struct {
    q          *sqlc.Queries
    journalSvc *JournalService
}

func NewExpenseGLService(q *sqlc.Queries, journalSvc *JournalService) *ExpenseGLService
```

**CreateAccrualEntry** (on approval):
```go
func (s *ExpenseGLService) CreateAccrualEntry(ctx, report *domain.ExpenseReport, items []domain.ExpenseItem) (*uuid.UUID, error)
```
1. Verify all items have `gl_account_id`. If nil → attempt mapping via `gl_mapping_rules` (source_dimension='expense', source_value=category). If no mapping → use `EXPENSE_SUSPENSE_ACCOUNT_ID` env. If no env → return nil (hold without GL posting).
2. Group items by `gl_account_id`, sum amounts per group.
3. Build `CreateJournalEntryInput`:
   - DR lines: one per expense account group
   - CR line: Employee Reimbursement Payable account (configurable via `EXPENSE_PAYABLE_ACCOUNT_ID` env)
   - SourceType: `expense_reimbursement`
   - SourceID: report.ID
   - Memo: `"Expense Report {report_number}: {title}"`
4. Call `journalSvc.Create(ctx, input)` → auto-posts
5. Return journal entry ID

**CreatePaymentEntry** (on mark-paid):
```go
func (s *ExpenseGLService) CreatePaymentEntry(ctx, report *domain.ExpenseReport) (*uuid.UUID, error)
```
1. Build `CreateJournalEntryInput`:
   - DR: Employee Reimbursement Payable (total_amount)
   - CR: Cash/Bank account (configurable via `EXPENSE_CASH_ACCOUNT_ID` env)
   - SourceType: `expense_payment`
   - SourceID: report.ID
   - Memo: `"Payment: Expense Report {report_number}"`
2. Call `journalSvc.Create(ctx, input)`
3. Return journal entry ID

- [ ] **Step 2: Write GL service tests**

`internal/service/expense_gl_service_test.go`:
- TestCreateAccrualEntry_GroupsByAccount
- TestCreateAccrualEntry_MissingGLAccount_UsesSuspense
- TestCreateAccrualEntry_BalancedDebitsCredits
- TestCreatePaymentEntry_CorrectAmounts

- [ ] **Step 3: Run tests**

Run: `go test ./internal/service/ -run TestExpenseGL -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/service/expense_gl_service.go internal/service/expense_gl_service_test.go
git commit -m "feat(expense): add GL journal entry generation for accrual and payment"
```

---

### Task 9: Expense Handler (All Endpoints)

**Files:**
- Create: `internal/handler/expense_handler.go`

**Context:** Follow pattern from `internal/handler/journal_handler.go`. Single handler struct with all methods. Uses `middleware.GetCompanyID(c)`, `middleware.GetUserID(c)`, `response.OK/Created/Paginated/BadRequest/Forbidden/Conflict/NotFound/UnprocessableEntity`.

- [ ] **Step 1: Write ExpenseHandler**

`internal/handler/expense_handler.go`:

```go
type ExpenseHandler struct {
    reportSvc  *service.ExpenseReportService
    policySvc  *service.ExpensePolicyService
    approverSvc *service.ExpenseApproverService
    receiptSvc *service.ExpenseReceiptService
    aiSvc      *service.ExpenseAIService
    glSvc      *service.ExpenseGLService
    q          *sqlc.Queries
}

func NewExpenseHandler(reportSvc, policySvc, approverSvc, receiptSvc, aiSvc, glSvc, q)
```

**Employee endpoints:**
- `CreateReport(c)` — bind JSON {title, notes}, call reportSvc.Create → response.Created
- `ListMyReports(c)` — query params: status, page, page_size → reportSvc.ListMy → response.Paginated
- `GetReport(c)` — `:id` param → reportSvc.GetByID → response.OK
- `UpdateReport(c)` — `:id`, bind JSON {title, notes} → reportSvc.UpdateDraft → response.OK. Return 409 if not draft.
- `DeleteReport(c)` — `:id` → reportSvc.DeleteDraft → response.OK. Return 409 if not draft.
- `AddItem(c)` — `:id/items`, bind JSON → reportSvc.AddItem → response.Created. Return 409 if not draft.
- `UpdateItem(c)` — `/items/:id`, bind JSON → reportSvc.UpdateItem → response.OK. Return 409 if not draft.
- `DeleteItem(c)` — `/items/:id` → reportSvc.DeleteItem → response.OK. Return 409 if not draft.
- `UploadReceipt(c)` — `/items/:id/receipt`, multipart file → receiptSvc.SaveReceipt + aiSvc.ExtractReceipt → response.OK
- `ServeReceipt(c)` — `/receipts/:itemId` → verify company ownership → stream file
- `SubmitReport(c)` — `:id/submit` → reportSvc.Submit → response.OK. Return 422 if 0 items.

**Approval endpoints:**
- `ListPendingApprovals(c)` — list reports with status=pending_approval for approver's departments → response.Paginated
- `ApproveReport(c)` — `:id/approve` → check self-approval (403), approve, generate GL → response.OK
- `RejectReport(c)` — `:id/reject`, bind JSON {reason} → check self-approval (403) → response.OK

**Finance endpoints:**
- `ListFinanceQueue(c)` — list reports with status=approved → response.Paginated
- `MarkPaid(c)` — `:id/mark-paid`, bind JSON {payment_reference} → generate payment GL → response.OK
- `GetFinanceSummary(c)` — query params: from, to → analytics queries → response.OK

**Admin endpoints:**
- `CreatePolicy(c)`, `ListPolicies(c)`, `GetPolicy(c)`, `UpdatePolicy(c)`, `DeletePolicy(c)`
- `CreateApprover(c)`, `ListApprovers(c)`, `UpdateApprover(c)`, `DeleteApprover(c)`
- `GetAnalytics(c)` — spend by category + department + summary

Error mapping:
- `ErrReportNotDraft` / `ErrItemFrozen` → `response.Conflict(c, msg)` (409)
- `ErrReportNoItems` → `response.UnprocessableEntity(c, msg)` (422)
- `ErrSelfApproval` → `response.Forbidden(c, msg)` (403)
- `ErrNotFound` → `response.NotFound(c, msg)` (404)

- [ ] **Step 2: Commit**

```bash
git add internal/handler/expense_handler.go
git commit -m "feat(expense): add handler with all 25+ HTTP endpoints"
```

---

### Task 10: Router Wiring + main.go Integration

**Files:**
- Modify: `internal/handler/router.go`
- Modify: `cmd/api/main.go`

**Context:** Add `Expense *ExpenseHandler` to Router struct. Add expense route group in Setup(). Wire all expense services and handler in main.go using fx pattern.

- [ ] **Step 1: Add Expense to Router struct**

In `internal/handler/router.go`, add after line 64 (after `Integration *IntegrationHandler`):

```go
	// Expense management
	Expense *ExpenseHandler
```

- [ ] **Step 2: Add expense routes in Setup()**

In `internal/handler/router.go` Setup(), add before the closing `}` of the function (before line 679):

```go
	// ---- Expense Management Routes ----
	if rt.Expense != nil {
		expenses := api.Group("/expenses")
		expenses.Use(authMw)
		{
			// Employee endpoints (member+)
			expenses.POST("/reports", rt.Expense.CreateReport)
			expenses.GET("/reports", rt.Expense.ListMyReports)
			expenses.GET("/reports/:id", rt.Expense.GetReport)
			expenses.PUT("/reports/:id", rt.Expense.UpdateReport)
			expenses.DELETE("/reports/:id", rt.Expense.DeleteReport)
			expenses.POST("/reports/:id/items", rt.Expense.AddItem)
			expenses.PUT("/items/:id", rt.Expense.UpdateItem)
			expenses.DELETE("/items/:id", rt.Expense.DeleteItem)
			expenses.POST("/items/:id/receipt", rt.Expense.UploadReceipt)
			expenses.GET("/receipts/:itemId", rt.Expense.ServeReceipt)
			expenses.POST("/reports/:id/submit", rt.Expense.SubmitReport)

			// Approval endpoints
			expenses.GET("/approvals", rt.Expense.ListPendingApprovals)
			expenses.POST("/reports/:id/approve", rt.Expense.ApproveReport)
			expenses.POST("/reports/:id/reject", rt.Expense.RejectReport)

			// Finance endpoints (accountant+)
			expenses.GET("/finance/queue", middleware.RequireCompanyRole(rt.CompanySvc, domain.CompanyRoleAccountant), rt.Expense.ListFinanceQueue)
			expenses.POST("/reports/:id/mark-paid", middleware.RequireCompanyRole(rt.CompanySvc, domain.CompanyRoleAccountant), rt.Expense.MarkPaid)
			expenses.GET("/finance/summary", middleware.RequireCompanyRole(rt.CompanySvc, domain.CompanyRoleAccountant), rt.Expense.GetFinanceSummary)

			// Admin endpoints (company_admin)
			expenses.POST("/policies", middleware.RequireCompanyRole(rt.CompanySvc, domain.CompanyRoleAdmin), rt.Expense.CreatePolicy)
			expenses.GET("/policies", rt.Expense.ListPolicies)
			expenses.GET("/policies/:id", rt.Expense.GetPolicy)
			expenses.PUT("/policies/:id", middleware.RequireCompanyRole(rt.CompanySvc, domain.CompanyRoleAdmin), rt.Expense.UpdatePolicy)
			expenses.DELETE("/policies/:id", middleware.RequireCompanyRole(rt.CompanySvc, domain.CompanyRoleAdmin), rt.Expense.DeletePolicy)
			expenses.POST("/approvers", middleware.RequireCompanyRole(rt.CompanySvc, domain.CompanyRoleAdmin), rt.Expense.CreateApprover)
			expenses.GET("/approvers", rt.Expense.ListApprovers)
			expenses.PUT("/approvers/:id", middleware.RequireCompanyRole(rt.CompanySvc, domain.CompanyRoleAdmin), rt.Expense.UpdateApprover)
			expenses.DELETE("/approvers/:id", middleware.RequireCompanyRole(rt.CompanySvc, domain.CompanyRoleAdmin), rt.Expense.DeleteApprover)
			expenses.GET("/analytics", middleware.RequireCompanyRole(rt.CompanySvc, domain.CompanyRoleAccountant), rt.Expense.GetAnalytics)
		}
	}
```

Note: `RequireCompanyRole` takes `AccessChecker` interface. `CompanyService` implements it (confirmed: `internal/service/company_service.go:246`). Import `domain` in router.go for the role constants.

- [ ] **Step 3: Wire services in main.go**

Add to `services` struct (after line 173):
```go
	// Expense management
	ExpensePolicy   *service.ExpensePolicyService
	ExpenseApprover *service.ExpenseApproverService
	ExpenseReport   *service.ExpenseReportService
	ExpenseReceipt  *service.ExpenseReceiptService
	ExpenseAI       *service.ExpenseAIService
	ExpenseGL       *service.ExpenseGLService
```

Add in `newServices()` (before the return statement, after line 275):
```go
	// Expense management services
	expensePolicySvc := service.NewExpensePolicyService(q)
	expenseApproverSvc := service.NewExpenseApproverService(q)
	expenseGLSvc := service.NewExpenseGLService(q, journalSvc)
	expenseAISvc := service.NewExpenseAIService(q, pool, ai, /* redis from config */)
	expenseReceiptSvc := service.NewExpenseReceiptService(q, cfg.Expense.UploadDir)
	expenseReportSvc := service.NewExpenseReportService(q, pool, publisher, expenseApproverSvc, expenseAISvc, expenseGLSvc)
```

Add to return (after `HRInbox: hrInbox,`):
```go
		ExpensePolicy:   expensePolicySvc,
		ExpenseApprover: expenseApproverSvc,
		ExpenseReport:   expenseReportSvc,
		ExpenseReceipt:  expenseReceiptSvc,
		ExpenseAI:       expenseAISvc,
		ExpenseGL:       expenseGLSvc,
```

- [ ] **Step 4: Wire handler in main.go**

Add to `handlers` struct (after line 329):
```go
	// Expense management
	Expense *handler.ExpenseHandler
```

Add in `newHandlers()` return (after line 421):
```go
		Expense: handler.NewExpenseHandler(
			svc.ExpenseReport, svc.ExpensePolicy, svc.ExpenseApprover,
			svc.ExpenseReceipt, svc.ExpenseAI, svc.ExpenseGL, q,
		),
```

Add to Router wiring in `newGinEngine()` (after line 490):
```go
		Expense:     h.Expense,
```

- [ ] **Step 5: Add config for expense upload dir**

Add `Expense` section to config struct if not exists:
```go
type ExpenseConfig struct {
    UploadDir string `env:"EXPENSE_UPLOAD_DIR" envDefault:"/data/receipts"`
}
```

- [ ] **Step 6: Verify compilation**

Run: `cd /Users/anna/Documents/aistarlight-go && go build ./cmd/api/`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/handler/router.go cmd/api/main.go internal/config/
git commit -m "feat(expense): wire expense services and handler into router and main.go"
```

---

### Task 11: Frontend API Client + Types

**Files:**
- Create: `../aistarlight/frontend/src/types/expense.ts`
- Create: `../aistarlight/frontend/src/api/expenses.ts`

**Context:** Follow pattern from `frontend/src/api/accounting.ts`. Uses axios client from `./client`. Types use interfaces.

- [ ] **Step 1: Write TypeScript types**

`frontend/src/types/expense.ts`:

```typescript
export interface ExpensePolicy {
  id: string
  company_id: string
  name: string
  category: string
  max_amount: number | null
  requires_receipt_above: number | null
  auto_approve_below: number | null
  ai_auto_approve: boolean
  description: string | null
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface ExpenseReport {
  id: string
  company_id: string
  submitter_user_id: string
  hr_payee_id: string | null
  report_number: string
  title: string
  status: 'draft' | 'submitted' | 'pending_approval' | 'approved' | 'rejected' | 'paid'
  total_amount: number
  currency: string
  submitted_at: string | null
  ai_reviewed_at: string | null
  ai_risk_score: number | null
  ai_decision: string | null
  ai_decision_reason: string | null
  approver_user_id: string | null
  approved_at: string | null
  rejection_reason: string | null
  reviewer_user_id: string | null
  paid_at: string | null
  payment_reference: string | null
  notes: string | null
  created_at: string
  updated_at: string
  items?: ExpenseItem[]
  audit_log?: ExpenseAudit[]
}

export interface ExpenseItem {
  id: string
  expense_report_id: string
  category: string
  description: string
  amount: number
  currency: string
  merchant_name: string | null
  transaction_date: string
  receipt_url: string | null
  receipt_ocr_data: any | null
  ai_category_confidence: number | null
  gl_account_id: string | null
  policy_id: string | null
  created_at: string
  updated_at: string
}

export interface ExpenseApprover {
  id: string
  company_id: string
  department_name: string
  approver_user_id: string
  max_amount: number | null
  priority: number
  is_active: boolean
  approver_name: string
  approver_email: string
  created_at: string
  updated_at: string
}

export interface ExpenseAudit {
  id: string
  expense_report_id: string | null
  action: string
  actor_user_id: string | null
  actor_type: string
  details: any | null
  created_at: string
  actor_name: string
}

export interface ExpenseSpendSummary {
  approved_count: number
  paid_count: number
  pending_count: number
  total_approved: number
  total_paid: number
  total_pending: number
}

export interface SpendByCategory {
  category: string
  total_amount: number
  item_count: number
}

export interface SpendByDepartment {
  department: string
  total_amount: number
  report_count: number
}
```

- [ ] **Step 2: Write API client**

`frontend/src/api/expenses.ts`:

```typescript
import client from './client'

export const expenseReportApi = {
  create: (data: { title: string; notes?: string }) =>
    client.post('/expenses/reports', data),
  list: (params?: { status?: string; page?: number; page_size?: number }) =>
    client.get('/expenses/reports', { params }),
  get: (id: string) =>
    client.get(`/expenses/reports/${id}`),
  update: (id: string, data: { title?: string; notes?: string }) =>
    client.put(`/expenses/reports/${id}`, data),
  delete: (id: string) =>
    client.delete(`/expenses/reports/${id}`),
  submit: (id: string) =>
    client.post(`/expenses/reports/${id}/submit`),
  approve: (id: string) =>
    client.post(`/expenses/reports/${id}/approve`),
  reject: (id: string, data: { reason: string }) =>
    client.post(`/expenses/reports/${id}/reject`, data),
  markPaid: (id: string, data: { payment_reference: string }) =>
    client.post(`/expenses/reports/${id}/mark-paid`, data),
}

export const expenseItemApi = {
  add: (reportId: string, data: any) =>
    client.post(`/expenses/reports/${reportId}/items`, data),
  update: (id: string, data: any) =>
    client.put(`/expenses/items/${id}`, data),
  delete: (id: string) =>
    client.delete(`/expenses/items/${id}`),
  uploadReceipt: (id: string, file: File) => {
    const form = new FormData()
    form.append('file', file)
    return client.post(`/expenses/items/${id}/receipt`, form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },
}

export const expenseApprovalApi = {
  listPending: (params?: { page?: number; page_size?: number }) =>
    client.get('/expenses/approvals', { params }),
}

export const expenseFinanceApi = {
  queue: (params?: { page?: number; page_size?: number }) =>
    client.get('/expenses/finance/queue', { params }),
  summary: (params?: { from?: string; to?: string }) =>
    client.get('/expenses/finance/summary', { params }),
}

export const expensePolicyApi = {
  create: (data: any) => client.post('/expenses/policies', data),
  list: () => client.get('/expenses/policies'),
  get: (id: string) => client.get(`/expenses/policies/${id}`),
  update: (id: string, data: any) => client.put(`/expenses/policies/${id}`, data),
  delete: (id: string) => client.delete(`/expenses/policies/${id}`),
}

export const expenseApproverApi = {
  create: (data: any) => client.post('/expenses/approvers', data),
  list: () => client.get('/expenses/approvers'),
  update: (id: string, data: any) => client.put(`/expenses/approvers/${id}`, data),
  delete: (id: string) => client.delete(`/expenses/approvers/${id}`),
}

export const expenseAnalyticsApi = {
  get: (params?: { from?: string; to?: string }) =>
    client.get('/expenses/analytics', { params }),
}
```

- [ ] **Step 3: Commit**

```bash
cd /Users/anna/Documents/aistarlight/frontend
git add src/types/expense.ts src/api/expenses.ts
git commit -m "feat(expense): add TypeScript types and API client for expense management"
```

---

### Task 12: Frontend Routes + Navigation

**Files:**
- Modify: `../aistarlight/frontend/src/router/index.ts`
- Modify: `../aistarlight/frontend/src/components/layout/DashboardLayout.vue`

**Context:** Add 8 expense routes as children of the dashboard layout. Add "Expenses" group to sidebar with role-based visibility. Follow existing patterns (lazy-loaded components, `mi()` helper for menu items).

- [ ] **Step 1: Add routes to router/index.ts**

Add expense routes in the children array of the root route (after existing routes, before the catch-all):

```typescript
// Expense Management
{
  path: "expenses",
  name: "expenses",
  component: () => import("../views/expenses/ExpenseListView.vue"),
  meta: { title: "My Expenses" },
},
{
  path: "expenses/new",
  name: "expense-new",
  component: () => import("../views/expenses/ExpenseFormView.vue"),
  meta: { title: "New Expense" },
},
{
  path: "expenses/:id",
  name: "expense-detail",
  component: () => import("../views/expenses/ExpenseDetailView.vue"),
  meta: { title: "Expense Detail" },
},
{
  path: "expenses/:id/edit",
  name: "expense-edit",
  component: () => import("../views/expenses/ExpenseFormView.vue"),
  meta: { title: "Edit Expense" },
},
{
  path: "expenses/approvals",
  name: "expense-approvals",
  component: () => import("../views/expenses/ApprovalQueueView.vue"),
  meta: { title: "Expense Approvals" },
},
{
  path: "expenses/finance",
  name: "expense-finance",
  component: () => import("../views/expenses/FinanceQueueView.vue"),
  meta: { title: "Finance Queue" },
},
{
  path: "expenses/policies",
  name: "expense-policies",
  component: () => import("../views/expenses/PolicyManageView.vue"),
  meta: { title: "Expense Policies" },
},
{
  path: "expenses/approvers",
  name: "expense-approvers",
  component: () => import("../views/expenses/ApproverManageView.vue"),
  meta: { title: "Expense Approvers" },
},
{
  path: "expenses/analytics",
  name: "expense-analytics",
  component: () => import("../views/expenses/ExpenseAnalyticsView.vue"),
  meta: { title: "Expense Analytics" },
},
```

- [ ] **Step 2: Add Expenses group to sidebar navigation**

In `DashboardLayout.vue`, add an "Expenses" section to the menu groups (between Accounting and Tax & Filing). Import icons:

```typescript
import { WalletOutline, CheckmarkCircleOutline, CashOutline, ShieldCheckmarkOutline, StatsChartOutline } from '@vicons/ionicons5'
```

Add menu group:

```typescript
// Expenses section
const expensesGroup = {
  label: t('nav.expenses'),
  key: 'expenses-group',
  icon: renderIcon(WalletOutline),
  children: [
    mi(t('nav.myExpenses'), 'expenses', WalletOutline, 'member'),
    mi(t('nav.expenseApprovals'), 'expense-approvals', CheckmarkCircleOutline, 'member'),
    mi(t('nav.financeQueue'), 'expense-finance', CashOutline, 'accountant'),
    mi(t('nav.expensePolicies'), 'expense-policies', ShieldCheckmarkOutline, 'company_admin'),
    mi(t('nav.expenseAnalytics'), 'expense-analytics', StatsChartOutline, 'accountant'),
  ].filter(Boolean),
}
```

Add `expensesGroup` to the menu options array (only show if it has visible children).

- [ ] **Step 3: Add i18n keys**

Add to the relevant locale files (`en.ts`, `zh.ts`):

```typescript
// English
nav: {
  // ... existing
  expenses: 'Expenses',
  myExpenses: 'My Expenses',
  expenseApprovals: 'Approvals',
  financeQueue: 'Finance Queue',
  expensePolicies: 'Policies',
  expenseApprovers: 'Approvers',
  expenseAnalytics: 'Analytics',
}
```

- [ ] **Step 4: Commit**

```bash
cd /Users/anna/Documents/aistarlight/frontend
git add src/router/index.ts src/components/layout/DashboardLayout.vue src/locales/
git commit -m "feat(expense): add frontend routes and navigation"
```

---

### Task 13: Frontend Views — Employee (List + Form + Detail)

**Files:**
- Create: `frontend/src/views/expenses/ExpenseListView.vue`
- Create: `frontend/src/views/expenses/ExpenseFormView.vue`
- Create: `frontend/src/views/expenses/ExpenseDetailView.vue`
- Create: `frontend/src/components/expenses/ReceiptUploader.vue`
- Create: `frontend/src/components/expenses/ExpenseItemForm.vue`
- Create: `frontend/src/components/expenses/RiskBadge.vue`

**Context:** Use NaiveUI components (NDataTable, NButton, NForm, NModal, NTag, NUpload). Follow existing AIStarlight view patterns.

- [ ] **Step 1: Write ExpenseListView.vue**

Status tabs (draft/submitted/pending/approved/rejected/paid), NDataTable with columns (report_number, title, total_amount, status, created_at, actions). "New Expense" button. Click row → detail view.

- [ ] **Step 2: Write ExpenseFormView.vue**

NForm with title + notes fields. Below: expense items list with add/edit/delete. Each item: category dropdown, description, amount, merchant, date, receipt upload. Auto-fills from OCR. Submit button sends report for AI review.

- [ ] **Step 3: Write ExpenseDetailView.vue**

Report header (number, title, status badge, total). Items table. Receipt thumbnails (click to enlarge). AI risk score badge. Approval status. Audit log timeline.

- [ ] **Step 4: Write ReceiptUploader.vue**

NUpload with drag-drop zone + camera capture. Shows OCR extraction preview after upload. Accept JPEG/PNG/PDF, max 10MB.

- [ ] **Step 5: Write ExpenseItemForm.vue**

NForm inside NModal. Fields: category (NSelect), description (NInput), amount (NInputNumber), merchant_name (NInput), transaction_date (NDatePicker), gl_account_id (NSelect from chart of accounts). Auto-populates from OCR data.

- [ ] **Step 6: Write RiskBadge.vue**

NTag component. Color: green (0-29), yellow (30-70), red (71-100). Shows score number.

- [ ] **Step 7: Verify frontend build**

Run: `cd /Users/anna/Documents/aistarlight/frontend && npm run build`
Expected: No errors

- [ ] **Step 8: Commit**

```bash
git add src/views/expenses/ src/components/expenses/
git commit -m "feat(expense): add employee expense views (list, form, detail) with receipt upload"
```

---

### Task 14: Frontend Views — Approval + Finance + Admin

**Files:**
- Create: `frontend/src/views/expenses/ApprovalQueueView.vue`
- Create: `frontend/src/views/expenses/FinanceQueueView.vue`
- Create: `frontend/src/views/expenses/PolicyManageView.vue`
- Create: `frontend/src/views/expenses/ApproverManageView.vue`
- Create: `frontend/src/views/expenses/ExpenseAnalyticsView.vue`
- Create: `frontend/src/components/expenses/ApprovalCard.vue`
- Create: `frontend/src/components/expenses/SpendChart.vue`

- [ ] **Step 1: Write ApprovalQueueView.vue**

List of pending_approval reports. Each shows report summary with risk badge. Approve/Reject buttons. Reject requires reason modal.

- [ ] **Step 2: Write ApprovalCard.vue**

Card component showing: submitter name, report title, amount, risk badge, item count. Approve (green) + Reject (red) buttons.

- [ ] **Step 3: Write FinanceQueueView.vue**

List of approved reports pending payment. Mark Paid button → modal for payment_reference. Summary stats at top (total pending, total paid this month).

- [ ] **Step 4: Write PolicyManageView.vue**

CRUD table for expense policies. NDataTable with create/edit modal (NForm). Fields match ExpensePolicy type. Deactivate instead of hard delete.

- [ ] **Step 5: Write ApproverManageView.vue**

CRUD table for department approvers. NDataTable with create/edit modal. Department dropdown (from hr_payees departments), user select, max_amount, priority.

- [ ] **Step 6: Write ExpenseAnalyticsView.vue**

Summary cards (total approved, paid, pending). Two charts: spend by category (pie/donut), spend by department (bar). Date range picker for filtering. Uses ECharts via vue-echarts.

- [ ] **Step 7: Write SpendChart.vue**

Reusable ECharts wrapper. Props: type ('pie'|'bar'), data, title.

- [ ] **Step 8: Verify frontend build**

Run: `cd /Users/anna/Documents/aistarlight/frontend && npm run build`
Expected: No errors

- [ ] **Step 9: Commit**

```bash
git add src/views/expenses/ src/components/expenses/
git commit -m "feat(expense): add approval, finance, admin, and analytics views"
```

---

### Task 15: Integration Verification

**Files:** None (verification only)

- [ ] **Step 1: Run go vet**

Run: `cd /Users/anna/Documents/aistarlight-go && go vet ./...`
Expected: No issues

- [ ] **Step 2: Run all Go tests**

Run: `go test ./... -count=1`
Expected: All pass

- [ ] **Step 3: Build Go binary**

Run: `make build-linux`
Expected: Binary produced at repo root

- [ ] **Step 4: Build frontend**

Run: `cd /Users/anna/Documents/aistarlight/frontend && npm run build`
Expected: No errors

- [ ] **Step 5: Verify sqlc is in sync**

Run: `cd /Users/anna/Documents/aistarlight-go && ~/go/bin/sqlc generate && git diff --stat`
Expected: No changes (sqlc output matches committed code)

---

## Task Dependencies

```
Task 1 (migration) → Task 2 (RBAC + domain) → Task 3 (sqlc queries)
Task 3 → Task 4 (policy+approver svc) ─┐
Task 3 → Task 5 (report svc) ───────────┤
Task 3 → Task 6 (receipt svc) ──────────┤── all parallel after Task 3
Task 3 → Task 7 (AI svc) ──────────────┤
Task 3 → Task 8 (GL svc) ──────────────┘
Tasks 4-8 → Task 9 (handler) → Task 10 (router wiring)
Task 10 → Task 11 (frontend API) → Task 12 (routes+nav)
Task 12 → Task 13 (employee views) ─┐── parallel
Task 12 → Task 14 (admin views) ────┘
Tasks 13-14 → Task 15 (verification)
```
