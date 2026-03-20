# Agent Execution Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make AIStarlight's 7 agents capable of executing real operations (classify, create journal entries, resolve anomalies, generate reports) with user confirmation before any data-modifying action.

**Architecture:** Two-phase SSE model. Low-risk tools execute inline during agent streaming. High-risk tools create a persisted ActionPlan, close the SSE stream, and wait for user confirmation via a separate HTTP endpoint. On confirm, the tool executes and the frontend auto-sends a follow-up message to resume the agent conversation.

**Tech Stack:** Go 1.26 (Gin, sqlc, pgx/v5), Vue 3 + TypeScript (Pinia, Axios), PostgreSQL, OpenAI function calling, SSE streaming.

**Spec:** `docs/superpowers/specs/2026-03-20-agent-execution-phase1-design.md`

---

## File Structure

### New Files (Backend)

| File | Responsibility |
|------|---------------|
| `internal/agent/tool_registry.go` | ToolDef struct, ToolRegistry (register, lookup by agent), ToolContext |
| `internal/agent/tool_executor.go` | (**Modify existing**) Risk-level routing, ActionPlan creation for high-risk, summary generation |
| `internal/agent/action_plan.go` | ActionPlan CRUD, Confirm (with SELECT FOR UPDATE), Cancel, timeout cleanup |
| `internal/agent/tools/shared.go` | lookup_tax_rule, search_knowledge, get_company_stats, get_user_preferences |
| `internal/agent/tools/classification.go` | list_sessions, get_session_summary, preview_classification, classify_transactions, update_transaction |
| `internal/agent/tools/journal.go` | list_journal_entries, preview_journal_entries, create_journal_entries, get_chart_of_accounts |
| `internal/agent/tools/reconciliation.go` | list_anomalies, get_anomaly_detail, resolve_anomaly, run_reconciliation |
| `internal/agent/tools/reporting.go` | list_reports, get_report_detail, generate_report, validate_report, get_filing_calendar |
| `internal/agent/tools/compliance.go` | run_compliance_check, suggest_fixes, apply_fix |
| `internal/agent/tools/audit.go` | scan_duplicates, scan_missing_receipts, scan_classification_issues |
| `migrations/000042_action_plans.up.sql` | action_plans table |
| `migrations/000042_action_plans.down.sql` | DROP action_plans |
| `queries/action_plans.sql` | sqlc queries for action_plans |

### New Files (Frontend)

| File | Responsibility |
|------|---------------|
| `../aistarlight/frontend/src/components/ai/ActionPlanCard.vue` | Confirmation card with summary, impact, confirm/cancel buttons |

**Note:** Frontend files are in a SEPARATE repository at `/Users/anna/Documents/aistarlight/frontend/`, not within `aistarlight-go`.

### Modified Files

| File | Changes |
|------|---------|
| `internal/agent/runtime.go` | Replace `ToolExecuteFunc` with `ToolRegistry`, add high-risk branching |
| `internal/agent/registry.go` | Rename `Registry` → `AgentRegistry` |
| `internal/agent/agents/setup.go` | Update `RegisterAll` to accept `*AgentRegistry` |
| `internal/agent/agents/*.go` | Update tool definitions for all 7 agents |
| `internal/handler/agent_handler.go` | Add confirm/cancel/pending-actions endpoints |
| `internal/handler/router.go` | Register new routes |
| `cmd/api/main.go` | Update `newAgentRuntime()` to initialize ToolRegistry, ActionPlanManager, wire dependencies |
| `../aistarlight/frontend/src/stores/agent.ts` | Add pendingActions, confirmAction, cancelAction, SSE event parsing |
| `../aistarlight/frontend/src/api/agent.ts` | Add confirmAction, cancelAction, getPendingActions API functions |
| `../aistarlight/frontend/src/components/ai/AIPanel.vue` | Render ActionPlanCard in message list |

---

## Task 1: Database Migration — action_plans table

**Files:**
- Create: `migrations/000042_action_plans.up.sql`
- Create: `migrations/000042_action_plans.down.sql`

- [ ] **Step 1: Create up migration**

```sql
-- migrations/000042_action_plans.up.sql
CREATE TABLE action_plans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    thread_id UUID NOT NULL REFERENCES agent_threads(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL,
    company_id UUID NOT NULL REFERENCES companies(id),
    user_id UUID NOT NULL REFERENCES users(id),
    tool_name TEXT NOT NULL,
    tool_args JSONB NOT NULL DEFAULT '{}',
    summary TEXT NOT NULL DEFAULT '',
    impact JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'confirmed', 'cancelled', 'executed', 'failed', 'timeout')),
    result JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ,
    executed_at TIMESTAMPTZ
);

CREATE INDEX idx_action_plans_thread ON action_plans(thread_id);
CREATE INDEX idx_action_plans_pending ON action_plans(status) WHERE status = 'pending';
CREATE INDEX idx_action_plans_company ON action_plans(company_id);
```

- [ ] **Step 2: Create down migration**

```sql
-- migrations/000042_action_plans.down.sql
DROP TABLE IF EXISTS action_plans;
```

- [ ] **Step 3: Create sqlc queries**

```sql
-- queries/action_plans.sql

-- name: CreateActionPlan :one
INSERT INTO action_plans (id, thread_id, agent_id, company_id, user_id, tool_name, tool_args, summary, impact)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetActionPlan :one
SELECT * FROM action_plans WHERE id = $1;

-- name: GetActionPlanForUpdate :one
SELECT * FROM action_plans WHERE id = $1 FOR UPDATE;

-- name: UpdateActionPlanStatus :exec
UPDATE action_plans
SET status = @status, updated_at = NOW(),
    confirmed_at = CASE WHEN @status = 'confirmed' THEN NOW() ELSE confirmed_at END,
    executed_at = CASE WHEN @status = 'executed' THEN NOW() ELSE executed_at END,
    result = COALESCE(@result, result),
    error_message = COALESCE(@error_message, error_message)
WHERE id = @id;

-- name: ListPendingActionsByThread :many
SELECT * FROM action_plans
WHERE thread_id = $1 AND status = 'pending'
ORDER BY created_at DESC;

-- name: CountPendingActionsByThread :one
SELECT COUNT(*) FROM action_plans
WHERE thread_id = $1 AND status = 'pending';

-- name: TimeoutExpiredActions :execrows
UPDATE action_plans
SET status = 'timeout', updated_at = NOW()
WHERE status = 'pending' AND created_at < NOW() - INTERVAL '30 minutes';
```

- [ ] **Step 4: Run sqlc generate**

Run: `cd /Users/anna/Documents/aistarlight-go && ~/go/bin/sqlc generate`
Expected: No errors, new `action_plans.sql.go` generated in `internal/repository/sqlc/`

- [ ] **Step 5: Commit**

```bash
git add migrations/000042_action_plans.up.sql migrations/000042_action_plans.down.sql queries/action_plans.sql internal/repository/sqlc/
git commit -m "feat: add action_plans table for agent execution confirmation"
```

---

## Task 2: Tool Registry and ToolContext

**Files:**
- Create: `internal/agent/tool_registry.go`

- [ ] **Step 1: Create ToolRegistry**

```go
// internal/agent/tool_registry.go
package agent

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	oai "github.com/sashabaranov/go-openai"
)

// RiskLevel indicates whether a tool requires user confirmation.
type RiskLevel string

const (
	RiskLow  RiskLevel = "low"
	RiskHigh RiskLevel = "high"
)

// ToolContext carries per-invocation context for tool execution.
type ToolContext struct {
	Ctx          context.Context
	CompanyID    uuid.UUID
	UserID       uuid.UUID
	Jurisdiction string // "PH", "SG", "LK"
	AgentID      string
	ThreadID     uuid.UUID
}

// ToolDef defines a single executable tool.
type ToolDef struct {
	Name        string
	Description string          // Human-readable, used in LLM schema
	Parameters  json.RawMessage // JSON Schema for function calling
	RiskLevel   RiskLevel
	AgentIDs    []string // Which agents may use this tool
	SummaryTmpl string   // Template for generating human-readable summary, e.g. "Classify {count} transactions"
	Execute     func(tc ToolContext, args json.RawMessage) (json.RawMessage, error)
}

// OAITool converts a ToolDef to the OpenAI function calling format.
func (t *ToolDef) OAITool() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		},
	}
}

// ToolRegistry manages all available tools.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]*ToolDef
}

// NewToolRegistry creates an empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]*ToolDef)}
}

// Register adds a tool to the registry.
func (r *ToolRegistry) Register(t *ToolDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
}

// Get returns a tool by name.
func (r *ToolRegistry) Get(name string) (*ToolDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// ForAgent returns all tools available to a specific agent as OAI tool definitions.
func (r *ToolRegistry) ForAgent(agentID string) []oai.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []oai.Tool
	for _, t := range r.tools {
		for _, aid := range t.AgentIDs {
			if aid == agentID || aid == "*" {
				result = append(result, t.OAITool())
				break
			}
		}
	}
	return result
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/anna/Documents/aistarlight-go && go build ./internal/agent/...`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/agent/tool_registry.go
git commit -m "feat: add ToolRegistry for centralized tool management"
```

---

## Task 3: ActionPlan Manager

**Files:**
- Create: `internal/agent/action_plan.go`

- [ ] **Step 1: Create ActionPlan manager**

```go
// internal/agent/action_plan.go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// ActionPlan represents a pending high-risk tool execution.
// NOTE: JSON tag uses "plan_id" (not "id") for frontend consistency.
type ActionPlan struct {
	ID        uuid.UUID       `json:"plan_id"`
	ThreadID  uuid.UUID       `json:"thread_id"`
	AgentID   string          `json:"agent_id"`
	CompanyID uuid.UUID       `json:"company_id"`
	UserID    uuid.UUID       `json:"user_id"`
	ToolName  string          `json:"tool_name"`
	ToolArgs  json.RawMessage `json:"tool_args"`
	Summary   string          `json:"summary"`
	Impact    json.RawMessage `json:"impact"`
	Status    string          `json:"status"`
}

// ActionPlanResult is returned after confirming and executing an action plan.
type ActionPlanResult struct {
	PlanID  uuid.UUID       `json:"plan_id"`
	Status  string          `json:"status"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// ActionPlanManager handles ActionPlan CRUD and execution.
type ActionPlanManager struct {
	pool     *pgxpool.Pool
	q        *sqlc.Queries
	registry *ToolRegistry
}

// NewActionPlanManager creates a new ActionPlanManager.
func NewActionPlanManager(pool *pgxpool.Pool, q *sqlc.Queries, registry *ToolRegistry) *ActionPlanManager {
	return &ActionPlanManager{pool: pool, q: q, registry: registry}
}

// Create creates a new pending ActionPlan. Returns error if thread already has a pending action.
func (m *ActionPlanManager) Create(ctx context.Context, threadID, companyID, userID uuid.UUID, agentID, toolName string, toolArgs, impact json.RawMessage) (*ActionPlan, error) {
	// Check for existing pending action on this thread
	count, err := m.q.CountPendingActionsByThread(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("check pending: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("thread already has a pending action")
	}

	// Generate summary from tool template
	tool, ok := m.registry.Get(toolName)
	summary := fmt.Sprintf("Execute %s", toolName)
	if ok && tool.SummaryTmpl != "" {
		summary = generateSummary(tool.SummaryTmpl, toolArgs)
	}

	// NOTE: All UUID columns in action_plans are NOT NULL, so sqlc generates uuid.UUID
	// (not pgtype.UUID). No uuidToPgtype() conversion needed.
	row, err := m.q.CreateActionPlan(ctx, sqlc.CreateActionPlanParams{
		ID:        uuid.New(),
		ThreadID:  threadID,
		AgentID:   agentID,
		CompanyID: companyID,
		UserID:    userID,
		ToolName:  toolName,
		ToolArgs:  toolArgs,
		Summary:   summary,
		Impact:    impact,
	})
	if err != nil {
		return nil, fmt.Errorf("create action plan: %w", err)
	}

	return actionPlanFromRow(row), nil
}

// Confirm executes the tool and marks the plan as executed. Idempotent.
func (m *ActionPlanManager) Confirm(ctx context.Context, planID, companyID, userID uuid.UUID, jurisdiction string) (*ActionPlanResult, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := m.q.WithTx(tx)

	plan, err := qtx.GetActionPlanForUpdate(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("get plan for update: %w", err)
	}

	// Authorization: company must match (direct uuid.UUID comparison)
	// NOTE: Spec also requires user_id match OR admin/owner role check.
	// For Phase 1, company_id check is sufficient since all users in a company
	// share the same data scope. User-level + role check deferred to Phase 2.
	if plan.CompanyID != companyID {
		return nil, fmt.Errorf("unauthorized: company mismatch")
	}

	// Idempotent: if already executed, return existing result
	if plan.Status == "executed" {
		return &ActionPlanResult{
			PlanID: planID,
			Status: "executed",
			Result: plan.Result,
		}, nil
	}

	if plan.Status != "pending" {
		return nil, fmt.Errorf("action plan is %s, cannot confirm", plan.Status)
	}

	// Mark confirmed
	// NOTE: UpdateActionPlanStatus uses @param_name syntax in sqlc query,
	// generating named fields: ID, Status, Result ([]byte), ErrorMessage (*string)
	_ = qtx.UpdateActionPlanStatus(ctx, sqlc.UpdateActionPlanStatusParams{
		ID:     planID,
		Status: "confirmed",
	})

	// Execute the tool
	tool, ok := m.registry.Get(plan.ToolName)
	if !ok {
		_ = qtx.UpdateActionPlanStatus(ctx, sqlc.UpdateActionPlanStatusParams{
			ID:           planID,
			Status:       "failed",
			ErrorMessage: strPtr("tool not found: " + plan.ToolName),
		})
		_ = tx.Commit(ctx)
		return &ActionPlanResult{PlanID: planID, Status: "failed", Error: "tool not found"}, nil
	}

	tc := ToolContext{
		Ctx:          ctx,
		CompanyID:    companyID,
		UserID:       userID,
		Jurisdiction: jurisdiction,
		AgentID:      plan.AgentID,
		ThreadID:     plan.ThreadID, // uuid.UUID, no .Bytes needed
	}

	result, execErr := tool.Execute(tc, plan.ToolArgs)

	if execErr != nil {
		_ = qtx.UpdateActionPlanStatus(ctx, sqlc.UpdateActionPlanStatusParams{
			ID:           planID,
			Status:       "failed",
			ErrorMessage: strPtr(execErr.Error()),
		})
		_ = tx.Commit(ctx)
		return &ActionPlanResult{PlanID: planID, Status: "failed", Error: execErr.Error()}, nil
	}

	_ = qtx.UpdateActionPlanStatus(ctx, sqlc.UpdateActionPlanStatusParams{
		ID:     planID,
		Status: "executed",
		Result: result,
	})

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &ActionPlanResult{PlanID: planID, Status: "executed", Result: result}, nil
}

// Cancel marks a pending plan as cancelled.
func (m *ActionPlanManager) Cancel(ctx context.Context, planID, companyID uuid.UUID) error {
	plan, err := m.q.GetActionPlan(ctx, planID)
	if err != nil {
		return fmt.Errorf("get plan: %w", err)
	}
	if plan.CompanyID != companyID {
		return fmt.Errorf("unauthorized: company mismatch")
	}
	if plan.Status != "pending" {
		return fmt.Errorf("action plan is %s, cannot cancel", plan.Status)
	}
	return m.q.UpdateActionPlanStatus(ctx, sqlc.UpdateActionPlanStatusParams{
		ID:     planID,
		Status: "cancelled",
	})
}

// GetPending returns pending actions for a thread.
func (m *ActionPlanManager) GetPending(ctx context.Context, threadID uuid.UUID) ([]ActionPlan, error) {
	rows, err := m.q.ListPendingActionsByThread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	plans := make([]ActionPlan, len(rows))
	for i, r := range rows {
		plans[i] = *actionPlanFromRow(r)
	}
	return plans, nil
}

// TimeoutExpired sets all pending plans older than 30 minutes to timeout.
func (m *ActionPlanManager) TimeoutExpired(ctx context.Context) (int64, error) {
	return m.q.TimeoutExpiredActions(ctx)
}

func generateSummary(tmpl string, args json.RawMessage) string {
	var m map[string]interface{}
	if err := json.Unmarshal(args, &m); err != nil {
		return tmpl
	}
	result := tmpl
	for k, v := range m {
		result = strings.ReplaceAll(result, "{"+k+"}", fmt.Sprintf("%v", v))
	}
	return result
}

// actionPlanFromRow converts sqlc-generated ActionPlan row to domain struct.
// All UUID columns are NOT NULL → sqlc generates uuid.UUID (no .Bytes needed).
func actionPlanFromRow(row sqlc.ActionPlan) *ActionPlan {
	return &ActionPlan{
		ID:        row.ID,
		ThreadID:  row.ThreadID,
		AgentID:   row.AgentID,
		CompanyID: row.CompanyID,
		UserID:    row.UserID,
		ToolName:  row.ToolName,
		ToolArgs:  row.ToolArgs,
		Summary:   row.Summary,
		Impact:    row.Impact,
		Status:    row.Status,
	}
}

func strPtr(s string) *string { return &s }
```

- [ ] **Step 2: Add ActionPlan persistence as chat_message**

After creating an ActionPlan, persist it as a `chat_message` with `message_type = 'action_plan'` so it appears in thread history on page reload. Add this method to `ActionPlanManager`:

```go
// PersistAsChatMessage saves the ActionPlan as a chat_message record for thread history.
func (m *ActionPlanManager) PersistAsChatMessage(ctx context.Context, plan *ActionPlan) error {
	planJSON, _ := json.Marshal(plan)
	_, err := m.q.CreateAgentMessage(ctx, sqlc.CreateAgentMessageParams{
		ID:                uuid.New(),
		CompanyID:         plan.CompanyID,
		UserID:            plan.UserID,
		Role:              "assistant",
		Content:           plan.Summary,
		ToolCalls:         []byte("[]"),  // NOT NULL column — must be non-nil
		ThreadID:          uuidToPgtype(plan.ThreadID),  // chat_messages.thread_id is nullable pgtype.UUID
		AgentID:           strPtr(plan.AgentID),
		MessageType:       strPtr("action_plan"),
		CitationsJson:     nil,
		ActionResultsJson: planJSON,
	})
	return err
}
```

Note: `chat_messages.thread_id` is a NULLABLE UUID column, so it uses `pgtype.UUID` and requires `uuidToPgtype()`. This is different from `action_plans` where all UUIDs are NOT NULL.

Call this from the `Create` method after successfully creating the plan:
```go
// After m.q.CreateActionPlan succeeds:
_ = m.PersistAsChatMessage(ctx, actionPlanFromRow(row))
```

- [ ] **Step 3: Verify compilation**

Run: `cd /Users/anna/Documents/aistarlight-go && go build ./internal/agent/...`
Expected: No errors (may need adjustments to `sqlc` generated types)

- [ ] **Step 4: Commit**

```bash
git add internal/agent/action_plan.go
git commit -m "feat: add ActionPlanManager with confirm/cancel/timeout/persistence"
```

---

## Task 4: Tool Executor (Risk-Level Router)

**Files:**
- Modify: `internal/agent/tool_executor.go` (EXISTING file — currently wraps ChatService)

**IMPORTANT:** This file already exists with `type ToolExecutor struct { chatSvc *service.ChatService }` and `NewToolExecutor(chatSvc)`. We replace the implementation. The `ToolExecuteFunc` type stays in `definition.go` where it already lives — do NOT redefine it here.

- [ ] **Step 1: Replace ToolExecutor implementation**

Replace the entire contents of `internal/agent/tool_executor.go`:

```go
// internal/agent/tool_executor.go
package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// ToolExecutor routes tool calls through the registry and creates ActionPlans for high-risk tools.
// Falls back to ChatService.ExecuteTool for tools not yet migrated to ToolRegistry.
type ToolExecutor struct {
	registry *ToolRegistry
	plans    *ActionPlanManager
	chatSvc  *service.ChatService // fallback for legacy tools
}

// NewToolExecutor creates a new ToolExecutor.
// chatSvc provides backward compatibility for tools not yet in the registry.
func NewToolExecutor(registry *ToolRegistry, plans *ActionPlanManager, chatSvc *service.ChatService) *ToolExecutor {
	return &ToolExecutor{registry: registry, plans: plans, chatSvc: chatSvc}
}

// ExecuteResult holds the outcome of a tool execution attempt.
type ExecuteResult struct {
	Result     json.RawMessage // Tool result (for low-risk, direct result; for high-risk, ActionPlan JSON)
	ActionPlan *ActionPlan     // Non-nil if high-risk tool created an ActionPlan
	Error      error
}

// Execute runs a tool call, routing by risk level.
// If the tool is in the registry, uses the new path; otherwise falls back to ChatService.
func (e *ToolExecutor) Execute(tc ToolContext, toolName string, args json.RawMessage) ExecuteResult {
	tool, ok := e.registry.Get(toolName)
	if !ok {
		// Fallback to ChatService for legacy tools not yet migrated
		if e.chatSvc != nil {
			result := e.chatSvc.ExecuteTool(tc.Ctx, toolName, string(args), tc.CompanyID, tc.Jurisdiction, tc.UserID)
			return ExecuteResult{Result: json.RawMessage(result)}
		}
		errMsg, _ := json.Marshal(map[string]string{"error": "unknown tool: " + toolName})
		return ExecuteResult{Result: errMsg, Error: fmt.Errorf("unknown tool: %s", toolName)}
	}

	// Verify agent is allowed to use this tool
	allowed := false
	for _, aid := range tool.AgentIDs {
		if aid == tc.AgentID || aid == "*" {
			allowed = true
			break
		}
	}
	if !allowed {
		errMsg, _ := json.Marshal(map[string]string{"error": "agent not authorized for tool: " + toolName})
		return ExecuteResult{Result: errMsg, Error: fmt.Errorf("agent %s not authorized for tool %s", tc.AgentID, toolName)}
	}

	if tool.RiskLevel == RiskLow {
		result, err := tool.Execute(tc, args)
		if err != nil {
			errMsg, _ := json.Marshal(map[string]string{"error": err.Error()})
			return ExecuteResult{Result: errMsg, Error: err}
		}
		return ExecuteResult{Result: result}
	}

	// High-risk: create ActionPlan instead of executing
	impact := generateImpact(tool, args)
	plan, err := e.plans.Create(tc.Ctx, tc.ThreadID, tc.CompanyID, tc.UserID, tc.AgentID, toolName, args, impact)
	if err != nil {
		errMsg, _ := json.Marshal(map[string]string{"error": "failed to create action plan: " + err.Error()})
		return ExecuteResult{Result: errMsg, Error: err}
	}

	planJSON, _ := json.Marshal(map[string]interface{}{
		"status":  "awaiting_confirmation",
		"plan_id": plan.ID.String(),
		"summary": plan.Summary,
		"message": "This action requires user confirmation. The user will see a confirmation card.",
	})
	return ExecuteResult{Result: planJSON, ActionPlan: plan}
}

// MakeExecuteFunc creates a legacy ToolExecuteFunc wrapper.
// NOTE: This is used ONLY as a temporary bridge during Task 9 migration.
// It does NOT set ThreadID, so high-risk tools will fall through to ChatService
// fallback (which doesn't create ActionPlans). Once Task 9 replaces the runtime
// to use ToolExecutor.Execute directly with full ToolContext, this method becomes
// dead code and can be removed.
func (e *ToolExecutor) MakeExecuteFunc() ToolExecuteFunc {
	return func(ctx context.Context, agentID, toolName string, args json.RawMessage, companyID uuid.UUID, userID uuid.UUID, jurisdiction string) (string, error) {
		// Legacy path: only route through ChatService fallback (no ActionPlans)
		if e.chatSvc != nil {
			result := e.chatSvc.ExecuteTool(ctx, toolName, string(args), companyID, jurisdiction, userID)
			return result, nil
		}
		return `{"error":"tool not available in legacy mode"}`, fmt.Errorf("tool %s not available", toolName)
	}
}

// generateImpact creates a default impact JSON from tool args.
func generateImpact(tool *ToolDef, args json.RawMessage) json.RawMessage {
	var m map[string]interface{}
	_ = json.Unmarshal(args, &m)
	impact := map[string]interface{}{
		"affected_count": 0,
		"details":        []string{},
	}
	// Extract count-like fields if present
	for _, key := range []string{"count", "affected_count", "transaction_count"} {
		if v, ok := m[key]; ok {
			impact["affected_count"] = v
		}
	}
	result, _ := json.Marshal(impact)
	return result
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/anna/Documents/aistarlight-go && go build ./internal/agent/...`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/agent/tool_executor.go
git commit -m "feat: add ToolExecutor with risk-level routing"
```

---

## Task 5: Shared Tool Implementations

**Files:**
- Create: `internal/agent/tools/shared.go`

- [ ] **Step 1: Create shared tools**

Implement 4 shared tools that wrap existing service methods:
- `lookup_tax_rule` — calls `KnowledgeService.Search` (existing in `chat_service.go`)
- `search_knowledge` — calls `KnowledgeService.Search` with category filter
- `get_company_stats` — calls `DashboardService.GetStats`
- `get_user_preferences` — calls `ChatService.executeGetPreferences` logic

Each tool follows this pattern:
```go
package tools

import (
	"encoding/json"
	"github.com/tonypk/aistarlight-go/internal/agent"
	// ... service imports
)

func RegisterShared(r *agent.ToolRegistry, /* service dependencies */) {
	r.Register(&agent.ToolDef{
		Name:        "lookup_tax_rule",
		Description: "Search Philippine/Singapore/Sri Lanka tax regulations...",
		Parameters:  json.RawMessage(`{"type":"object","properties":{...},"required":[...]}`),
		RiskLevel:   agent.RiskLow,
		AgentIDs:    []string{"*"},
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			// Parse args, call service, return JSON result
		},
	})
	// ... register other shared tools
}
```

Reference existing tool implementations in `chat_service.go` lines 636-830 for the exact service calls and argument structures.

**ChatService Migration Strategy:** Tools registered in the `ToolRegistry` take precedence. The `ToolExecutor` falls back to `ChatService.ExecuteTool` for any tool name NOT found in the registry. This means you can migrate tools incrementally — each tool added to the registry automatically bypasses the ChatService switch statement for that tool. Once all tools are migrated, the `ChatService.ExecuteTool` method and its switch statement become dead code and can be removed in a follow-up cleanup.

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/anna/Documents/aistarlight-go && go build ./internal/agent/...`

- [ ] **Step 3: Commit**

```bash
git add internal/agent/tools/shared.go
git commit -m "feat: add shared tool implementations (lookup, knowledge, stats, prefs)"
```

---

## Task 6: Classification Tool Implementations

**Files:**
- Create: `internal/agent/tools/classification.go`

- [ ] **Step 1: Create classification tools**

Implement 5 tools:
- `list_sessions` (low) — calls `SessionService.ListSessions`
- `get_session_summary` (low) — calls `SessionService.GetSessionSummary`
- `preview_classification` (low) — calls `ClassifierService.PreviewClassification`
- `classify_transactions` (**high**) — calls `SessionService.ClassifyTransactions`
- `update_transaction` (**high**) — calls `SessionService.UpdateTransaction`

High-risk tools set `SummaryTmpl`:
```go
SummaryTmpl: "Classify transactions in session {session_id}"
```

```go
func RegisterClassification(r *agent.ToolRegistry, sessionSvc *service.SessionService, classifierSvc *service.ClassifierService) {
	// ... register each tool
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/anna/Documents/aistarlight-go && go build ./internal/agent/...`

- [ ] **Step 3: Commit**

```bash
git add internal/agent/tools/classification.go
git commit -m "feat: add classification tool implementations"
```

---

## Task 7: Remaining Tool Implementations

**Files:**
- Create: `internal/agent/tools/journal.go`
- Create: `internal/agent/tools/reconciliation.go`
- Create: `internal/agent/tools/reporting.go`
- Create: `internal/agent/tools/compliance.go`
- Create: `internal/agent/tools/audit.go`

- [ ] **Step 1: Create journal tools**

4 tools: `list_journal_entries` (low), `preview_journal_entries` (low), `create_journal_entries` (**high**), `get_chart_of_accounts` (low).
Wrap: `JournalService.List`, `SessionService.PreviewJournalEntries`, `SessionService.CreateJournalEntries`, `AccountService.ListAccounts`.
Dependencies: `svc.Account` (`*service.AccountService`), `svc.Journal` (`*service.JournalService`), `svc.Session` (`*service.SessionService`).

- [ ] **Step 2: Create reconciliation tools**

4 tools: `list_anomalies` (low), `get_anomaly_detail` (low), `resolve_anomaly` (**high**), `run_reconciliation` (**high**).
Wrap: `BankReconService` (anomaly methods), `SessionService.RunReconciliation`.
Dependencies: `svc.BankRecon` (`*service.BankReconService`), `svc.Session` (`*service.SessionService`).

- [ ] **Step 3: Create reporting tools**

5 tools: `list_reports` (low), `get_report_detail` (low), `generate_report` (**high**), `validate_report` (low), `get_filing_calendar` (low).
Wrap: `ReportService`, `ComplianceService.ValidateReport`, `DashboardService.GetCalendar`.

- [ ] **Step 4: Create compliance tools**

3 tools: `run_compliance_check` (low), `suggest_fixes` (low), `apply_fix` (**high**).
Wrap: `ComplianceService`.

- [ ] **Step 5: Create audit tools**

3 tools: `scan_duplicates` (low), `scan_missing_receipts` (low), `scan_classification_issues` (low).
Move existing implementations from `chat_service.go` into tool wrappers.
Dependencies: `svc.Audit` (`*service.AuditService`) — these tools wrap AuditService scan methods.

- [ ] **Step 6: Verify all compile**

Run: `cd /Users/anna/Documents/aistarlight-go && go build ./internal/agent/...`

- [ ] **Step 7: Commit**

```bash
git add internal/agent/tools/
git commit -m "feat: add journal, recon, reporting, compliance, audit tools"
```

---

## Task 8: Rename Registry and Update Agent Definitions

**Files:**
- Modify: `internal/agent/registry.go` — rename `Registry` → `AgentRegistry`
- Modify: `internal/agent/agents/setup.go` — update parameter type
- Modify: `internal/agent/agents/*.go` — update tool references (7 files)
- Modify: `internal/agent/runtime.go` — update field type

- [ ] **Step 1: Rename Registry to AgentRegistry**

In `internal/agent/registry.go`, rename `Registry` → `AgentRegistry` everywhere.
In `runtime.go`, update the field type from `*Registry` to `*AgentRegistry`.
In `agents/setup.go`, update `RegisterAll(r *Registry)` → `RegisterAll(r *AgentRegistry)`.

- [ ] **Step 1b: Update all callers of `Registry()` → `AgentRegistry()`**

In `internal/handler/agent_handler.go`, update three existing calls:
```go
// Line ~40: h.runtime.Registry().ListForWorkflow(...)  → h.runtime.AgentRegistry().ListForWorkflow(...)
// Line ~42: h.runtime.Registry().ListAll()              → h.runtime.AgentRegistry().ListAll()
// Line ~50: h.runtime.Registry().Get(agentID)           → h.runtime.AgentRegistry().Get(agentID)
```

In `cmd/api/main.go`, `newAgentRuntime` already uses the new name in the updated code (Task 11).

- [ ] **Step 2: Update agent definitions to remove hardcoded tools**

Each agent file (`general.go`, `classifier.go`, etc.) currently includes `Tools map[string][]oai.Tool`. This stays but becomes supplementary — the `ToolRegistry` is the primary source. Update agent system prompts to reference the new tools.

Update system prompts to include instructions about ActionPlan:
```
When you call a tool that modifies data, the system will present a confirmation card to the user.
Wait for the result — you will receive either an execution result or a cancellation notice.
```

- [ ] **Step 3: Verify compilation**

Run: `cd /Users/anna/Documents/aistarlight-go && go build ./...`

- [ ] **Step 4: Commit**

```bash
git add internal/agent/registry.go internal/agent/runtime.go internal/agent/agents/
git commit -m "refactor: rename Registry to AgentRegistry, update agent prompts"
```

---

## Task 9: Runtime Integration — Two-Phase SSE

**Files:**
- Modify: `internal/agent/runtime.go`

This is the core change. The runtime must:
1. Replace `ToolExecuteFunc` with `ToolExecutor`
2. Use `ToolRegistry.ForAgent()` instead of `agentDef.ToolsFor()` for tool list
3. When a high-risk tool is called, emit `action_plan` SSE event and close stream with `AwaitingConfirmation`

- [ ] **Step 1: Update Runtime struct and add ActionPlans accessor**

```go
type Runtime struct {
	agentRegistry *AgentRegistry  // renamed from *Registry
	toolRegistry  *ToolRegistry
	ai            *openai.Client
	q             *sqlc.Queries
	executor      *ToolExecutor
	actionPlans   *ActionPlanManager
}

func NewRuntime(agentRegistry *AgentRegistry, toolRegistry *ToolRegistry, ai *openai.Client, q *sqlc.Queries, executor *ToolExecutor, actionPlans *ActionPlanManager) *Runtime {
	return &Runtime{
		agentRegistry: agentRegistry,
		toolRegistry:  toolRegistry,
		ai:            ai,
		q:             q,
		executor:      executor,
		actionPlans:   actionPlans,
	}
}

// AgentRegistry returns the agent registry (renamed from Registry()).
func (rt *Runtime) AgentRegistry() *AgentRegistry {
	return rt.agentRegistry
}

// ActionPlans returns the action plan manager.
func (rt *Runtime) ActionPlans() *ActionPlanManager {
	return rt.actionPlans
}
```

- [ ] **Step 2: Update ProcessStream tool execution block**

Replace the tool execution loop (lines ~116-147) with:

```go
for _, tc := range choice.Message.ToolCalls {
	// Send executing event
	execEvt, _ := json.Marshal([]map[string]string{{"tool_name": tc.Function.Name, "status": "executing"}})
	ch <- StreamEvent{ToolCalls: execEvt}

	result := rt.executor.Execute(toolCtx, tc.Function.Name, json.RawMessage(tc.Function.Arguments))

	if result.ActionPlan != nil {
		// High-risk: emit action_plan event and stop
		planEvt, _ := json.Marshal(result.ActionPlan)
		ch <- StreamEvent{ActionPlan: planEvt}
		// Let LLM generate a response about the pending action
		messages = append(messages, oai.ChatCompletionMessage{
			Role:       oai.ChatMessageRoleTool,
			Content:    string(result.Result),
			ToolCallID: tc.ID,
		})
		// Continue to streaming response, but mark as awaiting confirmation
		awaitingConfirmation = true
		break
	}

	// Low-risk: proceed normally
	// ... existing logic
}
```

- [ ] **Step 3: Add StreamEvent fields**

In `internal/agent/definition.go` (NOT `runtime.go` — that's where `StreamEvent` is defined), add to `StreamEvent` struct:
```go
type StreamEvent struct {
	Token                string          `json:"token,omitempty"`
	Done                 bool            `json:"done,omitempty"`
	Content              string          `json:"content,omitempty"`
	ThreadID             string          `json:"thread_id,omitempty"`
	ToolCalls            json.RawMessage `json:"tool_calls,omitempty"`
	Actions              json.RawMessage `json:"actions,omitempty"`
	ActionPlan           json.RawMessage `json:"action_plan,omitempty"`           // NEW
	AwaitingConfirmation bool            `json:"awaiting_confirmation,omitempty"` // NEW
	PendingPlanID        string          `json:"pending_plan_id,omitempty"`       // NEW
}
```

- [ ] **Step 4: Update tool list source**

Replace `agentDef.ToolsFor(req.Jurisdiction)` with `rt.toolRegistry.ForAgent(req.AgentID)` in ProcessStream.

- [ ] **Step 5: Verify compilation**

Run: `cd /Users/anna/Documents/aistarlight-go && go build ./...`

- [ ] **Step 6: Commit**

```bash
git add internal/agent/runtime.go
git commit -m "feat: integrate ToolExecutor into runtime with two-phase SSE"
```

---

## Task 10: Agent Handler — Confirm/Cancel Endpoints

**Files:**
- Modify: `internal/handler/agent_handler.go`
- Modify: `internal/handler/router.go`

- [ ] **Step 1: Add confirm endpoint**

```go
func (h *AgentHandler) ConfirmAction(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)
	jurisdiction := middleware.GetJurisdiction(c)

	// Validate agentId exists in registry
	agentID := c.Param("agentId")
	if _, ok := h.runtime.AgentRegistry().Get(agentID); !ok {
		response.BadRequest(c, "unknown agent: "+agentID)
		return
	}

	planID, err := uuid.Parse(c.Param("planId"))
	if err != nil {
		response.BadRequest(c, "invalid plan ID")
		return
	}

	result, err := h.runtime.ActionPlans().Confirm(c.Request.Context(), planID, companyID, userID, jurisdiction)
	if err != nil {
		response.Err(c, http.StatusBadRequest, err.Error())
		return
	}
	response.OK(c, result)
}
```

- [ ] **Step 2: Add cancel endpoint**

```go
func (h *AgentHandler) CancelAction(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	planID, err := uuid.Parse(c.Param("planId"))
	if err != nil {
		response.BadRequest(c, "invalid plan ID")
		return
	}
	if err := h.runtime.ActionPlans().Cancel(c.Request.Context(), planID, companyID); err != nil {
		response.Err(c, http.StatusBadRequest, err.Error())
		return
	}
	response.OK(c, map[string]string{"status": "cancelled"})
}
```

- [ ] **Step 3: Add pending-actions endpoint**

```go
func (h *AgentHandler) PendingActions(c *gin.Context) {
	threadID, err := uuid.Parse(c.Param("threadId"))
	if err != nil {
		response.BadRequest(c, "invalid thread ID")
		return
	}
	plans, err := h.runtime.ActionPlans().GetPending(c.Request.Context(), threadID)
	if err != nil {
		response.Err(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, plans)
}
```

- [ ] **Step 4: Register routes in router.go**

Add to the agent route group:
```go
agents.POST("/:agentId/actions/:planId/confirm", agentHandler.ConfirmAction)
agents.POST("/:agentId/actions/:planId/cancel", agentHandler.CancelAction)
agents.GET("/:agentId/threads/:threadId/pending-actions", agentHandler.PendingActions)
```

- [ ] **Step 5: Update SSE streaming in handler**

In the `StreamAgent` handler, emit `action_plan` and `awaiting_confirmation` fields from StreamEvent:
```go
if evt.ActionPlan != nil {
	data, _ := json.Marshal(map[string]interface{}{
		"action_plan": json.RawMessage(evt.ActionPlan),
	})
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	c.Writer.Flush()
}
```

- [ ] **Step 6: Verify compilation**

Run: `cd /Users/anna/Documents/aistarlight-go && go build ./...`

- [ ] **Step 7: Commit**

```bash
git add internal/handler/agent_handler.go internal/handler/router.go
git commit -m "feat: add confirm/cancel/pending-actions endpoints"
```

---

## Task 11: Wiring in cmd/api/main.go

**Files:**
- Modify: `cmd/api/main.go` (specifically the `newAgentRuntime` function at ~line 331)

**IMPORTANT:** There is NO `internal/app/bootstrap.go`. Agent wiring lives in `cmd/api/main.go` in the `newAgentRuntime()` function.

- [ ] **Step 1: Update newAgentRuntime to wire new components**

Replace the existing `newAgentRuntime` function:
```go
// Before (current):
// func newAgentRuntime(ai *oai.Client, q *sqlc.Queries, chatSvc *service.ChatService) *agent.Runtime {
//     registry := agent.NewRegistry()
//     agents.RegisterAll(registry)
//     toolExec := agent.NewToolExecutor(chatSvc)
//     return agent.NewRuntime(registry, ai, q, toolExec.MakeExecuteFunc())
// }

// After:
func newAgentRuntime(ai *oai.Client, q *sqlc.Queries, pool *pgxpool.Pool, chatSvc *service.ChatService, svc services) *agent.Runtime {
	// Agent registry (renamed from Registry)
	agentRegistry := agent.NewAgentRegistry()
	agents.RegisterAll(agentRegistry)

	// Tool registry — register all tools
	// Requires import: "github.com/tonypk/aistarlight-go/internal/agent/tools"
	toolRegistry := agent.NewToolRegistry()
	tools.RegisterShared(toolRegistry, svc.Knowledge, svc.Dashboard, svc.Chat)
	tools.RegisterClassification(toolRegistry, svc.Session, svc.Classifier)
	tools.RegisterJournal(toolRegistry, svc.Account, svc.Journal, svc.Session)
	tools.RegisterReconciliation(toolRegistry, svc.BankRecon, svc.Session)
	tools.RegisterReporting(toolRegistry, svc.Report, svc.Compliance, svc.Dashboard)
	tools.RegisterCompliance(toolRegistry, svc.Compliance)
	tools.RegisterAudit(toolRegistry, svc.Audit)

	// ActionPlan manager
	actionPlanMgr := agent.NewActionPlanManager(pool, q, toolRegistry)

	// Tool executor (with ChatService fallback for unmigrated tools)
	toolExecutor := agent.NewToolExecutor(toolRegistry, actionPlanMgr, chatSvc)

	return agent.NewRuntime(agentRegistry, toolRegistry, ai, q, toolExecutor, actionPlanMgr)
}
```

Update the `newHandlers` function signature to accept `pool`:
```go
// Before: func newHandlers(svc services, cfg *config.Config, ai *oai.Client, q *sqlc.Queries) handlers {
// After:
func newHandlers(svc services, cfg *config.Config, ai *oai.Client, q *sqlc.Queries, pool *pgxpool.Pool) handlers {
    agentRuntime := newAgentRuntime(ai, q, pool, svc.Chat, svc)
    // ... rest unchanged
```

Note: The app uses `fx` dependency injection. Since `pool *pgxpool.Pool` is already in the fx container (provided by `newPool`), adding it to `newHandlers`'s parameter list is sufficient — fx will inject it automatically.

- [ ] **Step 2: Add timeout cleanup goroutine**

After creating agentRuntime, register an fx lifecycle hook for ActionPlan timeout cleanup:
```go
// In the fx.Invoke or fx.Lifecycle block:
lc.Append(fx.Hook{
	OnStart: func(_ context.Context) error {
		ctx, cancel := context.WithCancel(context.Background())
		// Store cancel for OnStop
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if n, err := agentRuntime.ActionPlans().TimeoutExpired(context.Background()); err != nil {
						slog.Error("action plan timeout cleanup failed", "error", err)
					} else if n > 0 {
						slog.Info("timed out expired action plans", "count", n)
					}
				case <-ctx.Done():
					return
				}
			}
		}()
		return nil
	},
})
```

Alternatively, if the existing app structure doesn't use lifecycle hooks directly, a simpler approach is acceptable — just add a `go func()` with a comment noting it runs until process exit.

- [ ] **Step 3: Verify full build**

Run: `cd /Users/anna/Documents/aistarlight-go && go build ./cmd/api/...`

- [ ] **Step 4: Commit**

```bash
git add cmd/api/main.go
git commit -m "feat: wire ToolRegistry, ActionPlanManager, ToolExecutor, timeout cron"
```

---

## Task 12: Frontend — ActionPlanCard Component

**Files:**
- Create: `/Users/anna/Documents/aistarlight/frontend/src/components/ai/ActionPlanCard.vue`

**IMPORTANT:** Frontend is in a SEPARATE repository at `/Users/anna/Documents/aistarlight/frontend/`. All frontend file paths in Tasks 12-14 refer to that location.

- [ ] **Step 1: Create ActionPlanCard component**

```vue
<script setup lang="ts">
import { ref } from 'vue'
import { useAgentStore } from '../../stores/agent'

interface ActionPlanProps {
  planId: string
  toolName: string
  summary: string
  impact: { affected_count: number; details: string[] }
  status: 'pending' | 'confirming' | 'executed' | 'failed' | 'cancelled' | 'timeout'
  result?: Record<string, unknown>
  error?: string
}

const props = defineProps<ActionPlanProps>()
const agent = useAgentStore()
const confirming = ref(false)

async function confirm() {
  confirming.value = true
  await agent.confirmAction(props.planId)
  confirming.value = false
}

async function cancel() {
  await agent.cancelAction(props.planId)
}
</script>

<template>
  <div class="action-plan-card" :class="status">
    <div class="plan-header">
      <span class="plan-icon">{{ status === 'executed' ? '✅' : status === 'failed' ? '❌' : status === 'cancelled' ? '🚫' : '⚡' }}</span>
      <span class="plan-summary">{{ summary }}</span>
    </div>
    <div v-if="impact?.details?.length" class="plan-impact">
      <div v-for="(detail, i) in impact.details" :key="i" class="impact-item">{{ detail }}</div>
    </div>
    <div v-if="status === 'pending'" class="plan-actions">
      <button class="btn-confirm" :disabled="confirming" @click="confirm">
        {{ confirming ? 'Executing...' : 'Confirm' }}
      </button>
      <button class="btn-cancel" :disabled="confirming" @click="cancel">Cancel</button>
    </div>
    <div v-else-if="status === 'executed'" class="plan-result">Action completed successfully</div>
    <div v-else-if="status === 'failed'" class="plan-result error">Failed: {{ error }}</div>
    <div v-else-if="status === 'cancelled'" class="plan-result">Action cancelled</div>
  </div>
</template>
```

Add scoped styles following existing CSS variable patterns (`--bg-surface`, `--border-default`, `--brand-primary`, etc.).

- [ ] **Step 2: Commit**

```bash
cd /Users/anna/Documents/aistarlight
git add frontend/src/components/ai/ActionPlanCard.vue
git commit -m "feat: add ActionPlanCard component for agent action confirmation"
```

---

## Task 13: Frontend — Agent Store and API Updates

**Files:**
- Modify: `/Users/anna/Documents/aistarlight/frontend/src/stores/agent.ts`
- Modify: `/Users/anna/Documents/aistarlight/frontend/src/api/agent.ts`

- [ ] **Step 1: Add API functions**

In `/Users/anna/Documents/aistarlight/frontend/src/api/agent.ts`, add:
```typescript
export interface ActionPlan {
  plan_id: string
  tool_name: string
  summary: string
  impact: { affected_count: number; details: string[] }
  status: string
  result?: Record<string, unknown>
  error?: string
}

export async function confirmAction(agentId: string, planId: string): Promise<ActionPlan> {
  const res = await client.post(`/api/v1/agents/${agentId}/actions/${planId}/confirm`)
  return res.data.data
}

export async function cancelAction(agentId: string, planId: string): Promise<void> {
  await client.post(`/api/v1/agents/${agentId}/actions/${planId}/cancel`)
}

export async function getPendingActions(agentId: string, threadId: string): Promise<ActionPlan[]> {
  const res = await client.get(`/api/v1/agents/${agentId}/threads/${threadId}/pending-actions`)
  return res.data.data || []
}
```

- [ ] **Step 2: Update agent store**

In `/Users/anna/Documents/aistarlight/frontend/src/stores/agent.ts`, add:
- State: `awaitingConfirmation: false`
- Action `confirmAction(planId)`:
  1. Call `confirmAction` API
  2. Update the ActionPlanCard message status to 'executed'
  3. Auto-send follow-up message: `[System] Action executed: {tool_name} completed.`
- Action `cancelAction(planId)`:
  1. Call `cancelAction` API
  2. Update ActionPlanCard status to 'cancelled'
  3. Auto-send: `[System] Action cancelled by user: {tool_name}`

- [ ] **Step 3: Update SSE parsing in sendMessage**

In the stream parsing loop, handle new event fields:
```typescript
if (parsed.action_plan) {
  // Insert ActionPlanCard as a special message
  messages.value.push({
    role: 'assistant',
    content: '',
    isActionPlan: true,
    actionPlan: parsed.action_plan,
  })
}
if (parsed.awaiting_confirmation) {
  awaitingConfirmation.value = true
}
```

- [ ] **Step 4: Commit**

```bash
cd /Users/anna/Documents/aistarlight
git add frontend/src/stores/agent.ts frontend/src/api/agent.ts
git commit -m "feat: add confirmAction/cancelAction to agent store and API"
```

---

## Task 14: Frontend — AIPanel Integration

**Files:**
- Modify: `/Users/anna/Documents/aistarlight/frontend/src/components/ai/AIPanel.vue`

- [ ] **Step 1: Import and render ActionPlanCard**

In the message list template, add conditional rendering for action plan messages:
```vue
<template v-if="msg.isActionPlan && msg.actionPlan">
  <ActionPlanCard
    :plan-id="msg.actionPlan.plan_id"
    :tool-name="msg.actionPlan.tool_name"
    :summary="msg.actionPlan.summary"
    :impact="msg.actionPlan.impact"
    :status="msg.actionPlan.status || 'pending'"
    :result="msg.actionPlan.result"
    :error="msg.actionPlan.error"
  />
</template>
```

- [ ] **Step 2: Commit**

```bash
cd /Users/anna/Documents/aistarlight
git add frontend/src/components/ai/AIPanel.vue
git commit -m "feat: integrate ActionPlanCard into AIPanel"
```

---

## Task 15: Run Migration on Server and Deploy

**Files:**
- No new files

- [ ] **Step 1: Run migration on server**

```bash
ssh aistarlight-gce "sudo docker exec aistarlight-go-postgres-1 psql -U aistarlight -d aistarlight -f -" < migrations/000042_action_plans.up.sql
```

Or via the migration tool if configured.

- [ ] **Step 2: Build Go binaries**

```bash
cd /Users/anna/Documents/aistarlight-go && make build-linux
```

- [ ] **Step 3: Build and deploy frontend**

```bash
# SCP updated files to server
# On server:
sudo -u anna bash -c 'cd /home/anna/aistarlight-go && sudo docker compose -f docker-compose.prod.yml up -d --build api worker frontend'
sudo -u anna bash -c 'cd /home/anna/aistarlight-go && sudo docker compose -f docker-compose.prod.yml restart nginx'
```

- [ ] **Step 4: Verify deployment**

Test via browser:
1. Login to tax.clawpapa.win
2. Open AI panel
3. Ask the classifier agent to classify a session
4. Verify ActionPlanCard appears
5. Confirm or cancel
6. Verify agent responds appropriately

- [ ] **Step 5: Commit any fixes**

---

## Task 16: End-to-End Verification

- [ ] **Step 1: Test low-risk tool flow**

Ask an agent: "Show me my filing calendar"
Expected: Agent calls `get_filing_calendar` → executes directly → shows deadlines

- [ ] **Step 2: Test high-risk tool with confirm**

Ask classifier agent: "Classify the transactions in my latest session"
Expected: Agent calls `classify_transactions` → ActionPlanCard appears → click Confirm → executes → agent summarizes result

- [ ] **Step 3: Test high-risk tool with cancel**

Ask journal agent: "Create journal entries from session X"
Expected: ActionPlanCard appears → click Cancel → agent acknowledges cancellation

- [ ] **Step 4: Test page reload with pending action**

Create a pending action → reload page → verify ActionPlanCard still shows with buttons

- [ ] **Step 5: Commit final state**

```bash
git add -A
git commit -m "feat: complete Phase 1 agent execution with ActionPlan confirmation"
```
