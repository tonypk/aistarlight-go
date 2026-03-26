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

Extends existing RBAC (company_admin / accountant / member / viewer):

| Role | Can Submit | Can Approve | Can Mark Paid | Can Manage Policies |
|------|-----------|-------------|---------------|-------------------|
| company_admin | Yes | Yes | Yes | Yes |
| accountant | Yes | No | Yes | No |
| member | Yes | No | No | No |
| viewer | No | No | No | No |

Approval is role-independent — configured per department via `expense_approvers` table. A `member` who is listed as a department approver can approve expenses for that department.

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
    status TEXT NOT NULL DEFAULT 'draft',          -- draft/submitted/ai_reviewed/pending_approval/approved/rejected/paid
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
    reviewer_user_id UUID REFERENCES users(id),    -- finance reviewer
    paid_at TIMESTAMPTZ,
    payment_reference TEXT,                         -- check number, transfer ref, etc.
    journal_entry_id UUID,                          -- FK to journal_entries after GL posting
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_expense_reports_company_status ON expense_reports(company_id, status);
CREATE INDEX idx_expense_reports_submitter ON expense_reports(submitter_user_id, created_at DESC);
```

### expense_items

Individual line items within a report.

```sql
CREATE TABLE expense_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    expense_report_id UUID NOT NULL REFERENCES expense_reports(id) ON DELETE CASCADE,
    category TEXT NOT NULL,                 -- meals/transport/office/travel/entertainment/other
    description TEXT NOT NULL,
    amount NUMERIC(12,2) NOT NULL,
    currency TEXT NOT NULL DEFAULT 'PHP',
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_expense_approvers_dept ON expense_approvers(company_id, department_name) WHERE is_active;
```

### expense_audit_log

Immutable audit trail for all expense actions.

```sql
CREATE TABLE expense_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    expense_report_id UUID NOT NULL REFERENCES expense_reports(id) ON DELETE CASCADE,
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
PUT    /reports/:id          Update draft report (title, notes)
DELETE /reports/:id          Delete draft report
POST   /reports/:id/items    Add expense item to report
PUT    /items/:id            Update expense item
DELETE /items/:id            Delete expense item
POST   /items/:id/receipt    Upload receipt image (triggers OCR)
POST   /reports/:id/submit   Submit report for approval
```

### Approval Endpoints

```
GET    /approvals            List reports pending my approval
POST   /reports/:id/approve  Approve report
POST   /reports/:id/reject   Reject report (with reason)
```

### Finance Endpoints

```
GET    /finance/queue        List approved reports pending payment
POST   /reports/:id/mark-paid  Mark as paid (with payment_reference)
GET    /finance/summary      Spending summary (by period, category, department)
```

### Admin Endpoints

```
CRUD   /policies             Manage expense policies
CRUD   /approvers            Manage department approvers
GET    /analytics            Spending analytics dashboard data
```

### Request/Response Patterns

Follow AIStarlight-Go existing patterns:
- Responses wrapped in `{"success": true, "data": {...}}` envelope
- Pagination: `?page=1&page_size=20`, response includes `meta.total`
- Errors: `{"success": false, "error": {"code": "...", "message": "..."}}`

## AI Features

### Receipt OCR

**Trigger**: `POST /items/:id/receipt` with image upload.

**Flow**:
1. Save image to disk/S3 → set `receipt_url`
2. Send image to Claude Vision API with prompt:
   ```
   Extract from this receipt: merchant_name, total_amount, currency, date, items/description.
   Return JSON with confidence scores.
   ```
3. Parse response → auto-fill `merchant_name`, `amount`, `transaction_date`, `category`
4. Store raw OCR in `receipt_ocr_data` JSONB
5. Set `ai_category_confidence` based on Claude's confidence

**Error handling**: If OCR fails or confidence < 0.5, leave fields empty for manual entry. Never block submission on OCR failure.

### AI Policy Agent

**Trigger**: `POST /reports/:id/submit` — runs after status changes to `submitted`.

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

### Intelligent Classification

When creating expense items, suggest category based on merchant name:
- Maintain a `merchant_category_cache` (in-memory or Redis) built from company history
- If unknown merchant, use Claude to classify based on merchant name
- Suggest GL account mapping via existing `gl_mapping_rules`

## Approval Flow

```
Employee creates draft
    ↓ (adds items, uploads receipts)
Employee submits
    ↓
AI Policy Agent evaluates
    ├─ auto_approved (risk < 30, all under threshold)
    │   ↓ status = approved, audit_log: actor_type=ai
    │   ↓ auto-generate GL journal entry
    │   ↓ add to finance payment queue
    │
    ├─ needs_review (risk 30-70)
    │   ↓ status = pending_approval
    │   ↓ notify department approver
    │   ↓ approver approves/rejects
    │       ├─ approved → generate GL → finance queue
    │       └─ rejected → notify employee (with reason)
    │
    └─ high_risk (risk > 70)
        ↓ status = pending_approval (flagged)
        ↓ notify approver + finance
        ↓ approver approves/rejects
            ├─ approved → generate GL → finance queue
            └─ rejected → notify employee

Finance marks paid
    ↓ status = paid, payment_reference recorded
```

**Approver resolution**:
1. Look up `expense_approvers` by submitter's `department_name` (from `hr_payees`)
2. Filter by `max_amount >= report.total_amount`
3. Pick by `priority` (lowest first)
4. Fallback: any `company_admin`

## GL Journal Entry Generation

When a report reaches `approved` status:

1. Group items by `gl_account_id` (mapped from category via `gl_mapping_rules`)
2. Create journal entry:
   ```
   DR  Expense accounts (per category)     amount per group
     CR  Employee Reimbursement Payable     total_amount
   ```
3. Use existing `CreateJournalEntry` service method
4. Link: `expense_reports.journal_entry_id = journal_entry.id`
5. Journal memo: `"Expense Report {report_number}: {title}"`

When marked `paid`:
```
DR  Employee Reimbursement Payable    total_amount
  CR  Cash / Bank account             total_amount
```

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
- Accept: JPEG, PNG, PDF (max 10MB)
- Storage: `{UPLOAD_DIR}/receipts/{company_id}/{YYYY-MM}/{item_id}.{ext}`
- Serve: `GET /api/v1/expenses/receipts/:item_id` (auth-protected, streams file)
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

- **Unit tests**: Policy evaluation logic, risk score calculation, GL entry generation, approver resolution
- **Integration tests**: Full submit → AI review → approve → GL → paid flow
- **AI tests**: Mock Claude API responses, test OCR parsing and category classification
- **Target**: 80%+ coverage on business logic

## Migration Path

Single migration file adding all 5 tables. Follows AIStarlight-Go's golang-migrate format.

## Future Sub-Projects (out of scope)

After Expense Management is live:
1. **Bill Pay / Accounts Payable** — vendor invoice management, approval chains, payment tracking
2. **Budgets** — department/project budgets, real-time spend vs. plan tracking
3. **Procurement** — purchase requests, PO approval, vendor management, three-way matching
4. **Travel Management** — trip requests, policy-controlled booking, per diem rules
5. **AI Intelligence** — cross-module spend optimization, anomaly detection, forecasting
