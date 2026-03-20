# AIStarlight Agent Execution Phase 1: Tool Expansion + Action Confirmation

**Date:** 2026-03-20
**Status:** Draft
**Scope:** Phase 1 of 3 (Phase 2: Multi-step Action Plans, Phase 3: Workflow Templates)

## Problem

AIStarlight's specialized agents (general, classifier, journal, compliance, filing, recon, audit) are chat-only. They can answer questions and look up tax rules, but cannot execute operations like classifying transactions, creating journal entries, or generating reports. Users must manually switch to the relevant page and perform actions themselves, breaking the conversational flow.

## Goal

Make agents execution-capable: they can perform real operations (classify, create, resolve, generate) with user confirmation before any data-modifying action. Low-risk operations (queries, previews) execute automatically; high-risk operations (writes, deletes) require explicit user approval via an ActionPlan confirmation card in the chat UI.

## Non-Goals

- Multi-step workflow orchestration (Phase 2)
- Workflow templates / one-click automation (Phase 3)
- LLM provider abstraction (separate effort)
- New agent creation UI
- Changes to the existing `/api/v1/chat/stream` endpoint (backward compatible, no ActionPlan there)

---

## Design

### 1. Tool Registry

A centralized registry replaces the current hardcoded tool definitions. This replaces the existing `ToolExecuteFunc` in `runtime.go` and the `ChatService.ExecuteTool` switch statement. The 13 existing tools in `ChatService` are migrated into the new registry as individual `ToolDef` implementations.

The existing `Registry` in `internal/agent/registry.go` (for agent definitions) is renamed to `AgentRegistry` for clarity.

**Location:** `internal/agent/tool_registry.go`

```go
type RiskLevel string
const (
    RiskLow  RiskLevel = "low"
    RiskHigh RiskLevel = "high"
)

type ToolContext struct {
    Ctx          context.Context
    CompanyID    uuid.UUID
    UserID       uuid.UUID
    Jurisdiction string  // "PH", "SG", "LK"
    AgentID      string
}

type ToolDef struct {
    Name        string
    Description string              // For LLM function-calling schema
    Parameters  json.RawMessage     // JSON Schema for parameters
    RiskLevel   RiskLevel
    AgentIDs    []string            // Which agents may invoke this tool
    SummaryTmpl string              // Template for generating human-readable summary
    Execute     func(tc ToolContext, args json.RawMessage) (json.RawMessage, error)
}

type ToolRegistry struct {
    tools map[string]*ToolDef
}

func (r *ToolRegistry) Register(t *ToolDef)
func (r *ToolRegistry) ForAgent(agentID string) []*ToolDef
func (r *ToolRegistry) Get(name string) *ToolDef
```

### Migration from ChatService.ExecuteTool

The existing `ChatService.executeTool` switch statement (handling `generate_report`, `update_transaction`, `delete_transaction`, `lookup_tax_rule`, etc.) is decomposed: each case becomes a `ToolDef` registered in the `ToolRegistry`. The `ChatService` no longer owns tool execution — the `ToolExecutor` does. The existing chat endpoint (`/api/v1/chat/stream`) is updated to use the `ToolRegistry` but **without** the ActionPlan confirmation flow (all tools execute directly, preserving backward compatibility).

### 2. Tool Inventory

#### Classifier Agent
| Tool | Risk | Description |
|------|------|-------------|
| `list_sessions` | low | List reconciliation/classification sessions with status |
| `get_session_summary` | low | Get transaction counts, classification stats for a session |
| `preview_classification` | low | Preview AI classification results without writing |
| `classify_transactions` | **high** | Execute batch classification on a session |
| `update_transaction` | **high** | Update category/VAT type on a single transaction |

#### Journal Agent
| Tool | Risk | Description |
|------|------|-------------|
| `list_journal_entries` | low | List journal entries with filters |
| `preview_journal_entries` | low | Preview journal entries that would be generated from a session |
| `create_journal_entries` | **high** | Create journal entries from classified transactions |
| `get_chart_of_accounts` | low | List chart of accounts |

#### Recon Agent
| Tool | Risk | Description |
|------|------|-------------|
| `list_anomalies` | low | List detected anomalies for a session |
| `get_anomaly_detail` | low | Get full anomaly details |
| `resolve_anomaly` | **high** | Mark anomaly as resolved/dismissed with note |
| `run_reconciliation` | **high** | Execute reconciliation matching on a session |

#### Filing Agent
| Tool | Risk | Description |
|------|------|-------------|
| `list_reports` | low | List generated reports |
| `get_report_detail` | low | Get report data and validation status |
| `generate_report` | **high** | Generate a tax report (BIR 2550M, etc.) |
| `validate_report` | low | Run compliance validation on a report |
| `get_filing_calendar` | low | Get upcoming filing deadlines |

#### Compliance Agent
| Tool | Risk | Description |
|------|------|-------------|
| `run_compliance_check` | low | Execute compliance check (read-only) |
| `suggest_fixes` | low | Get AI-generated fix suggestions |
| `apply_fix` | **high** | Apply an auto-fix to a compliance violation |

#### Audit Agent
| Tool | Risk | Description |
|------|------|-------------|
| `scan_duplicates` | low | Scan for duplicate transactions (existing) |
| `scan_missing_receipts` | low | Scan for missing receipt records (existing) |
| `scan_classification_issues` | low | Scan for classification anomalies (existing) |

#### General Agent
| Tool | Risk | Description |
|------|------|-------------|
| `generate_report` | **high** | Generate a tax report (shared with Filing Agent) |
| `lookup_tax_rule` | low | Search tax regulations via RAG (existing) |
| `get_user_preferences` | low | Get user preferences (existing) |
| `search_knowledge` | low | Search knowledge base by query |
| `get_company_stats` | low | Get company financial summary |

#### Shared Tools (all agents)
| Tool | Risk | Description |
|------|------|-------------|
| `lookup_tax_rule` | low | Search tax regulations via RAG (existing) |
| `search_knowledge` | low | Search knowledge base by query |
| `get_company_stats` | low | Get company financial summary (revenue, expenses, etc.) |
| `get_user_preferences` | low | Get user preferences (existing) |

**Totals:** 25 unique tools (16 low-risk, 9 high-risk)

**Note:** `generate_report` is high-risk for both General and Filing agents. The existing behavior where General agent could execute it without confirmation changes — it now requires ActionPlan confirmation via the agent endpoints.

### 3. Tool Implementation Files

```
internal/agent/tools/
├── classification.go   // list_sessions, get_session_summary, preview_classification, classify_transactions, update_transaction
├── journal.go          // list_journal_entries, preview_journal_entries, create_journal_entries, get_chart_of_accounts
├── reconciliation.go   // list_anomalies, get_anomaly_detail, resolve_anomaly, run_reconciliation
├── reporting.go        // list_reports, get_report_detail, generate_report, validate_report, get_filing_calendar
├── compliance.go       // run_compliance_check, suggest_fixes, apply_fix
├── audit.go            // scan_duplicates, scan_missing_receipts, scan_classification_issues
└── shared.go           // lookup_tax_rule, search_knowledge, get_company_stats, get_user_preferences
```

Each tool function calls existing service methods. Tools are thin wrappers around existing `internal/service/` functionality.

### 4. ActionPlan Confirmation System

#### Data Model

```sql
CREATE TABLE action_plans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    thread_id UUID NOT NULL REFERENCES agent_threads(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL,
    company_id UUID NOT NULL REFERENCES companies(id),
    user_id UUID NOT NULL REFERENCES users(id),
    tool_name TEXT NOT NULL,
    tool_args JSONB NOT NULL,
    summary TEXT NOT NULL,
    impact JSONB DEFAULT '{}',
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
CREATE INDEX idx_action_plans_status ON action_plans(status) WHERE status = 'pending';
```

**Down migration:**
```sql
DROP TABLE IF EXISTS action_plans;
```

**Impact field contract:**
```json
{
  "affected_count": 26,
  "details": ["15 transactions → Input VAT (Sales)", "8 transactions → Services (Exempt)"]
}
```

All tool implementations must populate `affected_count` (integer) and `details` (string array) to ensure consistent frontend rendering.

#### Two-Phase Execution Flow (No Long-Lived SSE)

When a high-risk tool is called, the SSE stream **closes** instead of pausing. This avoids holding goroutines/connections open for minutes and survives server restarts.

```
Phase 1: Agent Stream
─────────────────────
User sends message → SSE stream opens
  → Agent reasons → calls high-risk tool
  → Runtime creates ActionPlan (status: pending)
  → SSE emits "action_plan" event
  → Agent emits final message: "I've prepared the action. Please confirm."
  → SSE emits "message_end" with awaiting_confirmation: true
  → SSE stream closes

Phase 2: Confirm + Resume
─────────────────────────
User clicks [Confirm] → POST /actions/:planId/confirm
  → Server executes tool (within DB transaction)
  → Returns ActionPlanResult (status: executed, result: {...})
  → Frontend displays result in ActionPlanCard
  → Frontend auto-sends follow-up message to agent:
    "[System] Action executed: classify_transactions completed. 26 transactions classified."
  → New SSE stream opens → Agent responds to the result

Cancel flow:
User clicks [Cancel] → POST /actions/:planId/cancel
  → Server sets status to "cancelled"
  → Frontend auto-sends: "[System] Action cancelled by user: classify_transactions"
  → New SSE stream opens → Agent acknowledges cancellation
```

This eliminates long-lived connections and survives nginx restarts / server deployments.

#### Summary Generation

The `summary` field is generated **server-side** using the tool's `SummaryTmpl` template combined with the tool arguments. This is more reliable than depending on the LLM to include a `confirmation_summary` parameter.

Example templates:
- `classify_transactions`: "Classify {count} transactions in session {session_id}"
- `create_journal_entries`: "Create journal entries from session {session_id}"
- `resolve_anomaly`: "Resolve anomaly {anomaly_id} as {status}"

The tool executor calls a `GenerateSummary(toolDef, args)` function that fills the template from args.

#### ActionPlan Manager

**Location:** `internal/agent/action_plan.go`

```go
type ActionPlanManager struct {
    queries  *sqlc.Queries
    pool     *pgxpool.Pool
    registry *ToolRegistry
}

func (m *ActionPlanManager) Create(ctx, threadID, agentID, companyID, userID uuid.UUID, toolName string, toolArgs json.RawMessage, impact json.RawMessage) (*ActionPlan, error)
func (m *ActionPlanManager) Confirm(ctx, planID, companyID, userID uuid.UUID) (*ActionPlanResult, error)
func (m *ActionPlanManager) Cancel(ctx, planID, companyID, userID uuid.UUID) error
func (m *ActionPlanManager) GetPending(ctx, threadID uuid.UUID) ([]ActionPlan, error)
func (m *ActionPlanManager) TimeoutExpired(ctx context.Context) (int, error)  // cleanup cron
```

**Confirm authorization rules:**
1. `company_id` must match the authenticated user's company
2. `user_id` must match the plan creator OR the confirming user must have `admin`/`owner` role
3. Plan `status` must be `pending` — uses `SELECT ... FOR UPDATE` to prevent double-confirm race conditions
4. If `status` is already `executed`, return the existing result (idempotent)
5. Maximum 1 pending action per thread (reject new high-risk tool calls if one is pending)

#### API Endpoints

```
POST /api/v1/agents/:agentId/actions/:planId/confirm
POST /api/v1/agents/:agentId/actions/:planId/cancel
GET  /api/v1/agents/:agentId/threads/:threadId/pending-actions
```

All endpoints require JWT auth and enforce company_id scoping.

### 5. Runtime Changes

**File:** `internal/agent/runtime.go`

Current flow:
1. Receive user message
2. Build messages array (system prompt + history + user message)
3. Call LLM with tools
4. If tool call → execute → add result → call LLM again
5. Stream response tokens via SSE

New flow (changes at step 4):
1-3. Same as current
4. If tool call:
   a. Look up tool in ToolRegistry
   b. If low-risk → execute directly → add result → continue LLM loop
   c. If high-risk:
      - Check if thread already has a pending action → reject with message
      - Create ActionPlan → emit `action_plan` SSE event
      - Let LLM finish its response (it should acknowledge the pending action)
      - Close SSE stream with `awaiting_confirmation: true`
5. When confirm endpoint is called → execute tool → return result
6. Frontend sends follow-up message with result → new SSE stream → Agent continues

### 6. ActionPlan Persistence in Chat History

ActionPlan events are persisted as special messages in `chat_messages` using `message_type = 'action_plan'` (column exists from migration 000024). When loading thread history, the frontend reconstructs ActionPlanCards from these records.

Saved fields in `chat_messages`:
- `message_type`: `'action_plan'`
- `content`: the summary text
- `action_results_json`: `{ "plan_id": "...", "status": "executed|cancelled|...", "impact": {...}, "result": {...} }`

This ensures ActionPlan cards appear correctly when users reload a conversation.

### 7. SSE Event Extensions

Current events: `message_start`, `content_delta`, `tool_call`, `message_end`

New events:
```json
// Agent requests user confirmation
{
  "event": "action_plan",
  "data": {
    "plan_id": "uuid",
    "tool_name": "classify_transactions",
    "summary": "Classify 26 transactions in session abc123",
    "impact": {
      "affected_count": 26,
      "details": [
        "15 transactions → Input VAT (Sales)",
        "8 transactions → Services (Exempt)",
        "3 transactions → Capital Goods"
      ]
    }
  }
}
```

The `message_end` event is extended with an optional field:
```json
{
  "event": "message_end",
  "data": {
    "done": true,
    "awaiting_confirmation": true,
    "pending_plan_id": "uuid"
  }
}
```

Note: `action_result` is not an SSE event — the confirm endpoint returns the result directly via HTTP response, since the SSE stream is already closed.

### 8. Frontend Changes

#### New Component: `ActionPlanCard.vue`

**Location:** `frontend/src/components/ai/ActionPlanCard.vue`

Renders inside the Agent chat message list as a special card:
- Header: tool name icon + summary text
- Body: impact details as a bullet list (`affected_count` + `details` array)
- Footer: [Confirm] [Cancel] buttons (only when status is `pending`)
- States: pending → confirming (loading) → executed (success) / failed (error) / cancelled / timeout
- Timeout state: shown when the plan has `status: timeout` (cleaned up by server cron)

#### Agent Store Extensions

**File:** `frontend/src/stores/agent.ts`

New state:
```typescript
pendingActions: Map<string, ActionPlan>  // planId → ActionPlan
awaitingConfirmation: boolean            // true when SSE closed with awaiting_confirmation
```

New methods:
```typescript
async confirmAction(planId: string): Promise<void>
  // 1. Call confirmAction API
  // 2. Update ActionPlanCard to show result
  // 3. Auto-send follow-up message: "[System] Action executed: {tool_name} completed."
  // 4. This triggers a new SSE stream where Agent responds to the result

async cancelAction(planId: string): Promise<void>
  // 1. Call cancelAction API
  // 2. Update ActionPlanCard to show cancelled
  // 3. Auto-send follow-up message: "[System] Action cancelled by user: {tool_name}"
  // 4. This triggers a new SSE stream where Agent acknowledges
```

SSE parsing additions in `sendMessage`:
- Parse `action_plan` event → add to messages as ActionPlanCard, store in `pendingActions`
- Parse `message_end` with `awaiting_confirmation` → set `awaitingConfirmation = true`

On page reload:
- When loading thread messages, reconstruct ActionPlanCards from `message_type = 'action_plan'` messages
- Check `pending-actions` endpoint for any still-pending actions

#### API Module

**File:** `frontend/src/api/agent.ts`

New functions:
```typescript
confirmAction(agentId: string, planId: string): Promise<ActionPlanResult>
cancelAction(agentId: string, planId: string): Promise<void>
getPendingActions(agentId: string, threadId: string): Promise<ActionPlan[]>
```

### 9. Agent Definition Updates

Each agent's system prompt is updated to describe available tools and instruct the agent to:
1. Prefer querying/previewing before suggesting modifications
2. Always explain what a high-risk tool will do before calling it
3. Handle the response after a high-risk tool call gracefully (the system will request user confirmation)
4. When receiving a `[System] Action executed:` message, summarize the result and suggest next steps
5. When receiving a `[System] Action cancelled:` message, acknowledge and suggest alternatives

Example addition to classifier agent system prompt:
```
You have tools to classify transactions. Always preview first using preview_classification,
then explain the results to the user. If they want to proceed, call classify_transactions.
The system will ask the user for confirmation before executing. After execution, you will
receive the result — summarize it and suggest next steps.
```

---

## File Changes Summary

### New Files
| File | Purpose |
|------|---------|
| `internal/agent/tool_registry.go` | Centralized tool registration and lookup |
| `internal/agent/tool_executor.go` | Tool execution with risk checking and ActionPlan creation |
| `internal/agent/action_plan.go` | ActionPlan CRUD, confirmation with authorization |
| `internal/agent/tools/classification.go` | Classification tool implementations |
| `internal/agent/tools/journal.go` | Journal entry tool implementations |
| `internal/agent/tools/reconciliation.go` | Reconciliation tool implementations |
| `internal/agent/tools/reporting.go` | Reporting tool implementations |
| `internal/agent/tools/compliance.go` | Compliance tool implementations |
| `internal/agent/tools/audit.go` | Audit scanning tool implementations |
| `internal/agent/tools/shared.go` | Shared tool implementations (knowledge, stats) |
| `migrations/NNNNNN_action_plans.up.sql` | ActionPlan table migration |
| `migrations/NNNNNN_action_plans.down.sql` | ActionPlan table down migration |
| `queries/action_plans.sql` | sqlc queries for action_plans |
| `frontend/src/components/ai/ActionPlanCard.vue` | Confirmation card component |

### Modified Files
| File | Changes |
|------|---------|
| `internal/agent/runtime.go` | Integrate ToolRegistry, two-phase ActionPlan flow |
| `internal/agent/registry.go` | Rename `Registry` → `AgentRegistry` for clarity |
| `internal/agent/agents/*.go` | Update tool lists and system prompts for all 7 agents |
| `internal/handler/agent_handler.go` | Add confirm/cancel/pending-actions endpoints |
| `internal/handler/router.go` | Register new agent action routes |
| `internal/app/bootstrap.go` | Initialize ToolRegistry, ActionPlanManager |
| `internal/service/chat_service.go` | Remove ExecuteTool switch, delegate to ToolRegistry |
| `frontend/src/stores/agent.ts` | Add pendingActions state, confirm/cancel methods, SSE parsing |
| `frontend/src/api/agent.ts` | Add confirmAction, cancelAction, getPendingActions API calls |
| `frontend/src/components/ai/AIPanel.vue` | Render ActionPlanCard in message list, handle awaiting state |

---

## Testing Strategy

### Unit Tests
- Tool registry: registration, lookup, agent filtering, summary template generation
- Tool executor: risk level routing (low → execute, high → ActionPlan)
- ActionPlan manager: create, confirm (with SELECT FOR UPDATE), cancel, timeout, idempotent confirm
- Authorization: company_id mismatch rejected, user_id mismatch without admin role rejected
- Each tool implementation: correct service method called with correct args

### Integration Tests
- Full flow: message → tool call → action_plan SSE event → stream closes → confirm HTTP → result
- Cancel flow: action_plan → cancel HTTP → follow-up message → agent acknowledges
- Idempotent confirm: confirm twice → second returns same result, no double execution
- Thread reload: action plan persisted in chat_messages, reconstructed on load

### E2E Tests
- User asks agent to classify → sees ActionPlanCard → confirms → sees result → agent summarizes
- User asks agent to classify → sees ActionPlanCard → cancels → agent acknowledges
- User reloads page with pending action → ActionPlanCard shows with buttons

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Server restarts with pending action | Two-phase design: ActionPlan persisted in DB, confirm endpoint is stateless |
| User closes browser with pending action | Background cron sets `status = 'timeout'` after 30 minutes |
| Tool execution fails after confirmation | ActionPlan status set to `failed`, error shown in card, user can retry |
| LLM generates bad tool arguments | Validate args against JSON Schema before ActionPlan creation |
| Agent calls wrong tool for context | System prompt instructions + tool parameter validation |
| Double-click confirm button | SELECT FOR UPDATE + idempotent confirm (return existing result if executed) |
| Concurrent actions on same thread | Max 1 pending action per thread enforced at creation time |

---

## Future Phases

**Phase 2 — Multi-step Action Plans:**
Agent generates a plan with multiple sequential tool calls. User confirms the entire plan at once. Runtime executes tools in sequence, reporting progress. Built on top of the ActionPlan infrastructure from Phase 1.

**Phase 3 — Workflow Templates:**
Pre-defined workflow templates (e.g., "Monthly Tax Filing") that chain multiple agent actions. One-click trigger from sidebar or dashboard. Pause at human-confirmation checkpoints.
