# AIStarlight Agent Execution Phase 1: Tool Expansion + Action Confirmation

**Date:** 2026-03-20
**Status:** Draft
**Scope:** Phase 1 of 3 (Phase 2: Multi-step Action Plans, Phase 3: Workflow Templates)

## Problem

AIStarlight's 8 specialized agents (general, classifier, journal, compliance, filing, recon, audit, setup) are chat-only. They can answer questions and look up tax rules, but cannot execute operations like classifying transactions, creating journal entries, or generating reports. Users must manually switch to the relevant page and perform actions themselves, breaking the conversational flow.

## Goal

Make agents execution-capable: they can perform real operations (classify, create, resolve, generate) with user confirmation before any data-modifying action. Low-risk operations (queries, previews) execute automatically; high-risk operations (writes, deletes) require explicit user approval via an ActionPlan confirmation card in the chat UI.

## Non-Goals

- Multi-step workflow orchestration (Phase 2)
- Workflow templates / one-click automation (Phase 3)
- LLM provider abstraction (separate effort)
- New agent creation UI

---

## Design

### 1. Tool Registry

A centralized registry replaces the current hardcoded tool definitions.

**Location:** `internal/agent/tool_registry.go`

```go
type RiskLevel string
const (
    RiskLow  RiskLevel = "low"
    RiskHigh RiskLevel = "high"
)

type ToolDef struct {
    Name        string
    Description string              // For LLM function-calling schema
    Parameters  json.RawMessage     // JSON Schema for parameters
    RiskLevel   RiskLevel
    AgentIDs    []string            // Which agents may invoke this tool
    Execute     func(ctx context.Context, companyID uuid.UUID, userID uuid.UUID, args json.RawMessage) (json.RawMessage, error)
}

type ToolRegistry struct {
    tools map[string]*ToolDef
}

func (r *ToolRegistry) Register(t *ToolDef)
func (r *ToolRegistry) ForAgent(agentID string) []*ToolDef
func (r *ToolRegistry) Get(name string) *ToolDef
```

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

#### Shared Tools (all agents)
| Tool | Risk | Description |
|------|------|-------------|
| `lookup_tax_rule` | low | Search tax regulations via RAG (existing) |
| `search_knowledge` | low | Search knowledge base by query |
| `get_company_stats` | low | Get company financial summary (revenue, expenses, etc.) |
| `get_user_preferences` | low | Get user preferences (existing) |

**Totals:** 23 tools (15 low-risk, 8 high-risk)

### 3. Tool Implementation Files

```
internal/agent/tools/
├── classification.go   // list_sessions, get_session_summary, preview_classification, classify_transactions, update_transaction
├── journal.go          // list_journal_entries, preview_journal_entries, create_journal_entries, get_chart_of_accounts
├── reconciliation.go   // list_anomalies, get_anomaly_detail, resolve_anomaly, run_reconciliation
├── reporting.go        // list_reports, get_report_detail, generate_report, validate_report, get_filing_calendar
├── compliance.go       // run_compliance_check, suggest_fixes, apply_fix
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
    impact JSONB,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'confirmed', 'cancelled', 'executed', 'failed')),
    result JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ,
    executed_at TIMESTAMPTZ
);

CREATE INDEX idx_action_plans_thread ON action_plans(thread_id);
CREATE INDEX idx_action_plans_status ON action_plans(status) WHERE status = 'pending';
```

#### Execution Flow

```
Agent calls tool via LLM function-calling
         │
    ToolExecutor checks RiskLevel
         │
    ┌────┴────┐
    │         │
  Low       High
    │         │
 Execute    Create ActionPlan
 directly   (status: pending)
    │         │
 Return     Push SSE event
 result     "action_plan"
 to Agent     │
              │
         User sees
         ActionPlanCard
              │
    ┌────────┴────────┐
    │                 │
 Confirm           Cancel
    │                 │
 Execute tool    Set status
    │            "cancelled"
 Set status         │
 "executed"    Return cancel
    │          signal to Agent
 Push SSE
 "action_result"
    │
 Agent continues
 with result
```

#### ActionPlan Manager

**Location:** `internal/agent/action_plan.go`

```go
type ActionPlanManager struct {
    queries *sqlc.Queries
    registry *ToolRegistry
}

func (m *ActionPlanManager) Create(ctx, threadID, agentID, companyID, userID, toolName string, toolArgs json.RawMessage) (*ActionPlan, error)
func (m *ActionPlanManager) Confirm(ctx, planID, userID uuid.UUID) (*ActionPlanResult, error)
func (m *ActionPlanManager) Cancel(ctx, planID, userID uuid.UUID) error
func (m *ActionPlanManager) GetPending(ctx, threadID uuid.UUID) ([]ActionPlan, error)
```

The `summary` field is generated by the LLM as part of the tool call. When the Agent calls a high-risk tool, it must include a `confirmation_summary` parameter describing the action in human-readable terms. The runtime extracts this to populate the ActionPlan summary.

#### API Endpoints

```
POST /api/v1/agents/:agentId/actions/:planId/confirm
POST /api/v1/agents/:agentId/actions/:planId/cancel
GET  /api/v1/agents/:agentId/threads/:threadId/pending-actions
```

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
   a. Look up tool in registry
   b. If low-risk → execute directly → add result → continue
   c. If high-risk → create ActionPlan → push `action_plan` SSE event → **pause**
   d. When user confirms → execute tool → push `action_result` SSE event → resume LLM with tool result
   e. When user cancels → push cancel → resume LLM with "User cancelled this action"

The "pause" mechanism: the SSE stream stays open. The runtime waits on a channel for the confirmation/cancellation signal. A goroutine listens for the HTTP confirm/cancel request and signals the channel. Timeout after 5 minutes → auto-cancel.

### 6. SSE Event Extensions

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

// Action execution completed
{
  "event": "action_result",
  "data": {
    "plan_id": "uuid",
    "status": "executed",
    "result": { "classified_count": 26, "success": true }
  }
}
```

### 7. Frontend Changes

#### New Component: `ActionPlanCard.vue`

**Location:** `frontend/src/components/ai/ActionPlanCard.vue`

Renders inside the Agent chat message list as a special card:
- Header: tool name icon + summary text
- Body: impact details as a bullet list
- Footer: [Confirm] [Cancel] buttons
- States: pending → confirming (loading) → executed (success) / failed (error) / cancelled

#### Agent Store Extensions

**File:** `frontend/src/stores/agent.ts`

New state:
```typescript
pendingActions: Map<string, ActionPlan>  // planId → ActionPlan
```

New methods:
```typescript
confirmAction(planId: string): Promise<void>
cancelAction(planId: string): Promise<void>
```

SSE parsing additions in `sendMessage`:
- Parse `action_plan` event → add to `pendingActions`, render ActionPlanCard
- Parse `action_result` event → update action status, show result

#### API Module

**File:** `frontend/src/api/agent.ts`

New functions:
```typescript
confirmAction(agentId: string, planId: string): Promise<void>
cancelAction(agentId: string, planId: string): Promise<void>
```

### 8. Agent Definition Updates

Each agent's system prompt is updated to describe available tools and instruct the agent to:
1. Prefer querying/previewing before suggesting modifications
2. Always explain what a high-risk tool will do before calling it
3. Include a clear `confirmation_summary` parameter for high-risk tools
4. Handle cancellation gracefully (acknowledge and suggest alternatives)

Example addition to classifier agent system prompt:
```
You have tools to classify transactions. Always preview first using preview_classification,
then explain the results to the user. If they want to proceed, call classify_transactions
with a clear confirmation_summary explaining what will change.
```

---

## File Changes Summary

### New Files
| File | Purpose |
|------|---------|
| `internal/agent/tool_registry.go` | Centralized tool registration |
| `internal/agent/tool_executor.go` | Tool execution with risk checking |
| `internal/agent/action_plan.go` | ActionPlan CRUD and confirmation flow |
| `internal/agent/tools/classification.go` | Classification tool implementations |
| `internal/agent/tools/journal.go` | Journal entry tool implementations |
| `internal/agent/tools/reconciliation.go` | Reconciliation tool implementations |
| `internal/agent/tools/reporting.go` | Reporting tool implementations |
| `internal/agent/tools/compliance.go` | Compliance tool implementations |
| `internal/agent/tools/shared.go` | Shared tool implementations |
| `db/migrations/NNNNNN_action_plans.up.sql` | ActionPlan table migration |
| `db/query/action_plans.sql` | sqlc queries for action_plans |
| `frontend/src/components/ai/ActionPlanCard.vue` | Confirmation card component |

### Modified Files
| File | Changes |
|------|---------|
| `internal/agent/runtime.go` | Integrate ToolRegistry, ActionPlan pause/resume flow |
| `internal/agent/agents/*.go` | Update tool lists and system prompts for all 8 agents |
| `internal/handler/agent_handler.go` | Add confirm/cancel endpoints |
| `internal/handler/router.go` | Register new agent action routes |
| `internal/app/bootstrap.go` | Initialize ToolRegistry, ActionPlanManager |
| `frontend/src/stores/agent.ts` | Add pendingActions state, confirm/cancel methods, SSE parsing |
| `frontend/src/api/agent.ts` | Add confirmAction, cancelAction API calls |
| `frontend/src/components/ai/AIPanel.vue` | Render ActionPlanCard in message list |

---

## Testing Strategy

### Unit Tests
- Tool registry: registration, lookup, agent filtering
- Tool executor: risk level routing (low → execute, high → ActionPlan)
- ActionPlan manager: create, confirm, cancel, timeout
- Each tool implementation: correct service method called with correct args

### Integration Tests
- Full SSE flow: message → tool call → action_plan event → confirm → action_result event
- Cancel flow: action_plan → cancel → agent receives cancellation
- Timeout flow: action_plan → 5 min timeout → auto-cancel

### E2E Tests
- User asks agent to classify → sees ActionPlanCard → confirms → sees result
- User asks agent to classify → sees ActionPlanCard → cancels → agent acknowledges

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| SSE connection drops during ActionPlan wait | Frontend reconnects and checks `pending-actions` endpoint |
| User closes browser with pending action | 5-minute timeout auto-cancels |
| Tool execution fails after confirmation | ActionPlan status set to `failed`, error shown in card |
| LLM generates bad tool arguments | Validate args against JSON Schema before execution |
| Agent calls wrong tool for context | System prompt instructions + tool parameter validation |

---

## Future Phases

**Phase 2 — Multi-step Action Plans:**
Agent generates a plan with multiple sequential tool calls. User confirms the entire plan at once. Runtime executes tools in sequence, reporting progress via SSE.

**Phase 3 — Workflow Templates:**
Pre-defined workflow templates (e.g., "Monthly Tax Filing") that chain multiple agent actions. One-click trigger from sidebar or dashboard. Pause at human-confirmation checkpoints.
