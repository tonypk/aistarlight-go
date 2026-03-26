# Expense Management + Reimbursement — Design Spec

## Context

finance.halaos.com (AIStarlight-Go) is a full accounting and tax compliance system for the Philippine market. It currently handles chart of accounts, journal entries, and BIR tax form auto-fill. It receives payroll data from AIGoNHR (hr.halaos.com) via signed webhooks and has bidirectional SSO.

This spec adds Ramp-style Expense Management: employee expense submission with receipt OCR, AI policy enforcement, two-level approval, GL journal entry generation, and a finance queue for payment tracking.

## Goals

1. Employees submit expense reports with receipt photos from finance.halaos.com
2. AI automatically extracts receipt data (OCR), classifies expenses, and evaluates policy compliance
3. Low-risk expenses are AI-auto-approved; others go through manager → finance approval
4. Approved expenses generate GL journal entries automatically
5. Finance team tracks payment status through a dedicated queue

## Non-Goals

- Actual payment execution (bank transfers, ACH) — only record/mark as paid
- Credit card or virtual card issuance
- Multi-currency support (PHP only for v1)
- Travel booking engine
- Procurement P2P flow (separate sub-project)

## Architecture

### Codebase & Infrastructure

- **Codebase**: AIStarlight-Go (`/Users/anna/Documents/aistarlight-go`)
- **Server**: 34.124.185.43 (GCE, domain: finance.halaos.com)
- **Stack**: Go 1.26, Gin, sqlc, PostgreSQL, Redis, Vue3+TS frontend
- **AI**: Claude API (already integrated in AIStarlight for column mapping and RAG)
- **File storage**: Local filesystem (configurable path), S3-compatible optional

### User Access

All employees access finance.halaos.com:
- Via SSO from hr.halaos.com (existing `POST /api/v1/auth/sso`)
- Via direct login (if account exists)
- SSO auto-creates user and links to `hr_payees` record

### Permission Model

Extends existing RBAC (company_admin / accountant / member / viewer).

**Pre-requisite fix**: The existing `roleLevel()` function in `internal/handler/middleware/rbac.go` maps `member` to level 0 (below `viewer` at 1). This must be corrected before implementing expense routes:

```
company_admin = 4
accountant    = 3
member        = 2
viewer        = 1
```

**Expense permissions**:

| Role | Can Submit | Can Approve | Can Mark Paid | Can Manage Policies |
|------|-----------|-------------|---------------|-------------------|
| company_admin | Yes | Via expense_approvers | Yes | Yes |
| accountant | Yes | Via expense_approvers | Yes | No |
| member | Yes | Via expense_approvers | No | No |
| viewer | No | No | No | No |

Approval is configured per department via `expense_approvers` table. Any user (member, accountant, or admin) listed in `expense_approvers` can approve expenses for that department, subject to the self-approval prevention rule (see Approval Flow).

### Segregation of Duties

- **Self-approval prevention**: A user MUST NOT approve their own expense report. Approver resolution skips any `expense_approvers` row where `approver_user_id = submitter_user_id`. If no eligible approver remains, escalate to `company_admin` (who is also not the submitter). If no eligible admin exists, hold report in `pending_approval` and notify all `company_admin` users.
- **Admin safeguard**: When a `company_admin` submits an expense, a different `company_admin` or designated approver must approve it. Single-admin companies must designate at least one other approver.

## Data Model

### expense_policies

Company-configurable expense rules.

```sql
CREATE TABLE expense_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    name TEXT NOT NULL,                    -- e.g. "Meals", "Transportation"
    category TEXT NOT NULL,                -- enum-like: meals, transport, office, travel, entertainment, other
    max_amount NUMERIC(12,2),              -- per-item max (NULL = no limit)
    requires_receipt_above NUMERIC(12,2) DEFAULT 0, -- require receipt if amount > this
    auto_approve_below NUMERIC(12,2),      -- AI auto-approve if amount < this AND low risk
    ai_auto_approve BOOLEAN NOT NULL DEFAULT false, -- enable AI auto-approval for this policy
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_expense_policies_company ON expense_policies(company_id) WHERE is_active;
```

### expense_reports

One report per submission (may contain multiple items).

```sql
CREATE TABLE expense_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    submitter_user_id UUID NOT NULL REFERENCES users(id),
    hr_payee_id UUID REFERENCES hr_payees(id),  -- links to HR employee record
    report_number TEXT NOT NULL,                  -- auto-generated: EXP-YYYYMM-XXXX
    title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',          -- draft/submitted/pending_approval/approved/rejected/paid
    total_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'PHP',
    submitted_at TIMESTAMPTZ,
    ai_reviewed_at TIMESTAMPTZ,
    ai_risk_score INTEGER,                         -- 0-100
    ai_decision TEXT,                              -- auto_approved/needs_review/high_risk
    ai_decision_reason TEXT,
    approver_user_id UUID REFERENCES users(id),    -- who approved/rejected
    approved_at TIMESTAMPTZ,
    rejection_reason TEXT,
    reviewer_user_id UUID REFERENCES users(id),    -- finance reviewer who marked paid
    paid_at TIMESTAMPTZ,
    payment_reference TEXT,                         -- check number, transfer ref, etc.
    accrual_journal_entry_id UUID REFERENCES journal_entries(id),  -- DR Expense / CR Payable
    payment_journal_entry_id UUID REFERENCES journal_entries(id),  -- DR Payable / CR Cash
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_expense_report_number UNIQUE (company_id, report_number),
    CHECK (currency = 'PHP')
);
CREATE INDEX idx_expense_reports_company_status ON expense_reports(company_id, status);
CREATE INDEX idx_expense_reports_submitter ON expense_reports(submitter_user_id, created_at DESC);
```

**Report number generation**: Use a per-company sequence via advisory lock:
```sql
SELECT pg_advisory_xact_lock(company_id_bigint);
SELECT COALESCE(MAX(CAST(SUBSTRING(report_number FROM 'EXP-\d{6}-(\d+)') AS INT)), 0) + 1
FROM expense_reports WHERE company_id = $1 AND report_number LIKE 'EXP-' || to_char(now(), 'YYYYMM') || '-%';
```
Format: `EXP-202603-0001`

**Status transitions** (only these are valid):
- `draft` → `submitted` (via submit endpoint)
- `submitted` → `pending_approval` (AI says needs_review or high_risk)
- `submitted` → `approved` (AI auto-approves)
- `pending_approval` → `approved` (human approves)
- `pending_approval` → `rejected` (human rejects)
- `rejected` → `draft` (employee revises and resubmits)
- `approved` → `paid` (finance marks paid)

### expense_items

Individual line items within a report.

```sql
CREATE TABLE expense_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    expense_report_id UUID NOT NULL REFERENCES expense_reports(id) ON DELETE CASCADE,
    category TEXT NOT NULL,                 -- meals/transport/office/travel/entertainment/other
    description TEXT NOT NULL,
    amount NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL DEFAULT 'PHP' CHECK (currency = 'PHP'),
    merchant_name TEXT,
    transaction_date DATE NOT NULL,
    receipt_url TEXT,                       -- file path or S3 URL
    receipt_ocr_data JSONB,                -- raw OCR extraction result
    ai_category_confidence NUMERIC(3,2),   -- 0.00-1.00
    gl_account_id UUID REFERENCES accounts(id), -- mapped GL account
    policy_id UUID REFERENCES expense_policies(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_expense_items_report ON expense_items(expense_report_id);
```

**Item state gating**: `PUT /items/:id` and `DELETE /items/:id` MUST return 409 Conflict if the parent report status is not `draft`. Items are frozen once submitted.

**Company ownership**: All item endpoints MUST verify company ownership by joining through `expense_reports.company_id`. There is no direct `company_id` on items to avoid denormalization.

### expense_approvers

Per-department approval configuration.

```sql
CREATE TABLE expense_approvers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    department_name TEXT NOT NULL,          -- matches hr_payees.department_name
    approver_user_id UUID NOT NULL REFERENCES users(id),
    max_amount NUMERIC(12,2),              -- can approve up to this amount (NULL = unlimited)
    priority INTEGER NOT NULL DEFAULT 0,   -- lower = first choice
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_expense_approver UNIQUE (company_id, department_name, approver_user_id)
);
CREATE INDEX idx_expense_approvers_dept ON expense_approvers(company_id, department_name) WHERE is_active;
```

### expense_audit_log

Immutable audit trail for all expense actions. Uses `ON DELETE SET NULL` to preserve audit entries if a report is deleted.

```sql
CREATE TABLE expense_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    expense_report_id UUID REFERENCES expense_reports(id) ON DELETE SET NULL,
    action TEXT NOT NULL,                   -- created/submitted/ai_reviewed/approved/rejected/paid/edited
    actor_user_id UUID REFERENCES users(id),
    actor_type TEXT NOT NULL DEFAULT 'user', -- user/ai/system
    details JSONB,                          -- action-specific metadata
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_expense_audit_report ON expense_audit_log(expense_report_id, created_at);
```

## API Design

All endpoints under `/api/v1/expenses/`. Requires JWT authentication.

### Employee Endpoints

```
POST   /reports              Create new expense report (draft)
GET    /reports              List my reports (paginated, filterable by status/date)
GET    /reports/:id          Get report detail with items + audit log
PUT    /reports/:id          Update draft report (title, notes). Returns 409 if not draft.
DELETE /reports/:id          Soft-delete draft report. Returns 409 if not draft.
POST   /reports/:id/items    Add expense item to draft report. Returns 409 if not draft.
PUT    /items/:id            Update expense item. Returns 409 if parent report not draft.
DELETE /items/:id            Delete expense item. Returns 409 if parent report not draft.
POST   /items/:id/receipt    Upload receipt image (triggers OCR). Max 10MB, JPEG/PNG/PDF.
GET    /receipts/:item_id    Serve receipt file (auth-protected, streams file)
POST   /reports/:id/submit   Submit report for approval. Returns 422 if 0 items.
```

### Approval Endpoints

```
GET    /approvals            List reports pending my approval
POST   /reports/:id/approve  Approve report. Returns 403 if self-approval.
POST   /reports/:id/reject   Reject report (with reason). Returns 403 if self-approval.
```

### Finance Endpoints

```
GET    /finance/queue        List approved reports pending payment
POST   /reports/:id/mark-paid  Mark as paid (with payment_reference)
GET    /finance/summary      Spending summary (by period, category, department)
```

### Admin Endpoints

```
POST   /policies             Create expense policy
GET    /policies             List expense policies
GET    /policies/:id         Get policy detail
PUT    /policies/:id         Update expense policy
DELETE /policies/:id         Deactivate expense policy (soft delete)
POST   /approvers            Add department approver
GET    /approvers            List department approvers
PUT    /approvers/:id        Update approver config
DELETE /approvers/:id        Remove department approver
GET    /analytics            Spending analytics dashboard data
```

### Validation Rules

- **Submit**: Reject with 422 if report has 0 items
- **Submit**: Recompute `total_amount` from SUM of items within a `SELECT ... FOR UPDATE` transaction
- **Item create/edit/delete**: Reject with 409 if parent report status is not `draft`
- **Report delete**: Only allowed when status is `draft`
- **Approve/reject**: Reject with 403 if `approver_user_id = submitter_user_id`
- **Receipt upload**: Validate file magic bytes (not just extension). Max 10MB. Accept only JPEG/PNG/PDF.
- **Limits**: Max 50 items per report, 1 receipt per item, 100MB total per report

### Request/Response Patterns

Follow AIStarlight-Go existing patterns:
- Responses wrapped in `{"success": true, "data": {...}}` envelope
- Pagination: `?page=1&page_size=20`, response includes `meta.total`
- Errors: `{"success": false, "error": "message string"}`

## AI Features

### Receipt OCR

**Trigger**: `POST /items/:id/receipt` with image upload.

**Flow**:
1. Validate file: check magic bytes, enforce 10MB limit
2. Save image to disk → set `receipt_url`
3. Send image to Claude Vision API with prompt:
   ```
   Extract from this receipt: merchant_name, total_amount, currency, date, items/description.
   Return JSON with confidence scores.
   ```
4. Parse response → auto-fill `merchant_name`, `amount`, `transaction_date`, `category`
5. Store raw OCR in `receipt_ocr_data` JSONB
6. Set `ai_category_confidence` based on Claude's confidence

**Error handling**: If OCR fails or confidence < 0.5, leave fields empty for manual entry. Never block submission on OCR failure. Log OCR errors for monitoring.

### AI Policy Agent

**Trigger**: `POST /reports/:id/submit` — runs synchronously after validation passes.

**Flow**:
1. Load company's `expense_policies`
2. For each item, check:
   - Amount within policy `max_amount`
   - Receipt present if required (`requires_receipt_above`)
   - Category matches merchant (heuristic + Claude check)
   - Duplicate detection: same amount + merchant within 7 days
   - Weekend/holiday flag
3. Compute aggregate `risk_score` (0-100):
   - Base: 10 (low risk)
   - +20 if any item missing receipt above threshold
   - +15 if any item exceeds policy max
   - +10 per duplicate candidate
   - +10 if weekend/holiday transactions
   - +15 if category-merchant mismatch
4. Decision:
   - `risk_score < 30` AND all items under `auto_approve_below` AND `ai_auto_approve=true` → `auto_approved`
   - `risk_score 30-70` → `needs_review` (route to department approver)
   - `risk_score > 70` → `high_risk` (route to approver + flag for finance)
5. Write `expense_audit_log` entry with actor_type=`ai`
6. Update report: `ai_risk_score`, `ai_decision`, `ai_decision_reason`, `ai_reviewed_at`
7. Set status: `approved` (if auto_approved) or `pending_approval` (if needs_review/high_risk)

### Intelligent Classification

When creating expense items, suggest category based on merchant name:
- Use Redis hash `expense:merchants:{company_id}` mapping merchant → category (built from company history, TTL 7 days)
- If unknown merchant, use Claude to classify based on merchant name
- Suggest GL account mapping via existing `gl_mapping_rules` (source_dimension=`expense`, source_value=category)
- Cache rebuilt on miss: query recent expense_items for same merchant pattern

## Approval Flow

```
Employee creates draft
    ↓ (adds items, uploads receipts)
Employee submits (recomputes total in transaction)
    ↓
AI Policy Agent evaluates
    ├─ auto_approved (risk < 30, all under threshold)
    │   ↓ status = approved, audit_log: actor_type=ai
    │   ↓ auto-generate accrual GL journal entry
    │   ↓ add to finance payment queue
    │
    ├─ needs_review (risk 30-70)
    │   ↓ status = pending_approval
    │   ↓ notify department approver
    │   ↓ approver approves/rejects (self-approval blocked)
    │       ├─ approved → generate accrual GL → finance queue
    │       └─ rejected → notify employee (with reason) → status back to draft
    │
    └─ high_risk (risk > 70)
        ↓ status = pending_approval (flagged)
        ↓ notify approver + finance
        ↓ approver approves/rejects (self-approval blocked)
            ├─ approved → generate accrual GL → finance queue
            └─ rejected → notify employee → status back to draft

Finance marks paid
    ↓ generate payment GL journal entry
    ↓ status = paid, payment_reference recorded
```

**Approver resolution**:
1. Look up `expense_approvers` by submitter's `department_name` (from `hr_payees`)
2. Exclude rows where `approver_user_id = submitter_user_id` (self-approval prevention)
3. Filter by `max_amount IS NULL OR max_amount >= report.total_amount`
4. Pick by `priority` (lowest first)
5. Fallback: any `company_admin` who is not the submitter
6. If no eligible approver exists, hold in `pending_approval` and notify all `company_admin` users

## GL Journal Entry Generation

### On Approval (Accrual Entry)

When a report reaches `approved` status:

1. Verify all items have `gl_account_id` set. If any item has NULL `gl_account_id`:
   - Attempt auto-mapping via `gl_mapping_rules` (source_dimension=`expense`, source_value=category)
   - If no mapping found, use a configurable default suspense account (`EXPENSE_SUSPENSE_ACCOUNT_ID` env var)
   - If no suspense account configured, block GL posting — hold report in `approved` with `accrual_journal_entry_id = NULL`, surface in finance queue with "GL mapping required" flag
2. Group items by `gl_account_id`
3. Create journal entry:
   ```
   DR  Expense accounts (per category)     amount per group
     CR  Employee Reimbursement Payable     total_amount
   ```
4. Use existing `CreateJournalEntry` service method with `source_type = 'expense_reimbursement'`, `source_id = expense_report.id`
5. Link: `expense_reports.accrual_journal_entry_id = journal_entry.id`
6. Journal memo: `"Expense Report {report_number}: {title}"`

### On Payment (Payment Entry)

When marked `paid`:
```
DR  Employee Reimbursement Payable    total_amount
  CR  Cash / Bank account             total_amount
```
- Use `source_type = 'expense_payment'`, `source_id = expense_report.id`
- Link: `expense_reports.payment_journal_entry_id = journal_entry.id`
- Journal memo: `"Payment: Expense Report {report_number}"`

## Frontend Pages

### Page List (Vue3 + NaiveUI)

| Route | Component | Role | Description |
|-------|-----------|------|-------------|
| `/expenses` | ExpenseListView | all | My expense reports with status tabs |
| `/expenses/new` | ExpenseFormView | member+ | Create/edit expense report with item list |
| `/expenses/:id` | ExpenseDetailView | all | Report detail, items, receipts, audit log |
| `/expenses/approvals` | ApprovalQueueView | approvers | Pending approval list with approve/reject |
| `/expenses/finance` | FinanceQueueView | accountant+ | Payment queue, mark paid, summary |
| `/expenses/policies` | PolicyManageView | admin | CRUD expense policies |
| `/expenses/approvers` | ApproverManageView | admin | Configure department approvers |
| `/expenses/analytics` | ExpenseAnalyticsView | admin/accountant | Charts: spend by category, department, trend |

### Key UI Components

- **ReceiptUploader**: Drag-drop + camera capture, shows OCR extraction preview
- **ExpenseItemForm**: Auto-filled from OCR, category dropdown, amount, date, merchant
- **RiskBadge**: Color-coded risk score display (green/yellow/red)
- **ApprovalCard**: Report summary with one-tap approve/reject buttons
- **SpendChart**: ECharts-based spend analytics (by category, department, time)

### Navigation

Add "Expenses" to the main sidebar in `AppLayout.vue`, between existing menu items. Sub-items:
- My Expenses (all roles)
- Approvals (visible when user is an approver)
- Finance Queue (accountant+)
- Policies (admin)
- Analytics (admin/accountant)

## File Storage

### Receipt Upload

- Upload endpoint: `POST /api/v1/expenses/items/:id/receipt`
- Validation: Magic byte check (JPEG: `FF D8 FF`, PNG: `89 50 4E 47`, PDF: `25 50 44 46`), max 10MB per file
- Limits: 1 receipt per item, max 50 items per report, 100MB total per report
- Storage: `{UPLOAD_DIR}/receipts/{company_id}/{YYYY-MM}/{item_id}.{ext}`
- Serve: `GET /api/v1/expenses/receipts/:item_id` (auth-protected, verify company ownership, stream file)
- Config: `EXPENSE_UPLOAD_DIR` env var, default `/data/receipts`

## Notifications

v1: In-app only (audit log serves as activity feed on report detail page).

Future: Email, Telegram bot integration.

Notification triggers:
- Report submitted → approver
- AI flagged high risk → approver + finance
- Approved → submitter
- Rejected → submitter (with reason)
- Marked paid → submitter

## Testing Strategy

- **Unit tests**: Policy evaluation logic, risk score calculation, GL entry generation, approver resolution (including self-approval prevention), report number generation
- **Integration tests**: Full submit → AI review → approve → GL → paid flow; self-approval rejection; empty report rejection; item freeze after submit
- **AI tests**: Mock Claude API responses, test OCR parsing and category classification
- **Target**: 80%+ coverage on business logic

## Migration Path

Single migration file adding all 5 tables + the RBAC level fix. Follows AIStarlight-Go's golang-migrate format.

## Future Sub-Projects (out of scope)

After Expense Management is live:
1. **Bill Pay / Accounts Payable** — vendor invoice management, approval chains, payment tracking
2. **Budgets** — department/project budgets, real-time spend vs. plan tracking
3. **Procurement** — purchase requests, PO approval, vendor management, three-way matching
4. **Travel Management** — trip requests, policy-controlled booking, per diem rules
5. **AI Intelligence** — cross-module spend optimization, anomaly detection, forecasting
