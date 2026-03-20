// internal/agent/action_plan.go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// ActionPlan represents a pending high-risk tool execution.
// JSON tag uses "plan_id" (not "id") for frontend consistency.
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

// ActionPlanResult holds the outcome of an action plan execution.
type ActionPlanResult struct {
	PlanID uuid.UUID       `json:"plan_id"`
	Status string          `json:"status"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// ActionPlanManager coordinates creation, confirmation, cancellation, and
// timeout of high-risk tool action plans.
type ActionPlanManager struct {
	pool     *pgxpool.Pool
	q        *sqlc.Queries
	registry *ToolRegistry
}

// NewActionPlanManager creates an ActionPlanManager.
func NewActionPlanManager(pool *pgxpool.Pool, q *sqlc.Queries, registry *ToolRegistry) *ActionPlanManager {
	return &ActionPlanManager{pool: pool, q: q, registry: registry}
}

// Create creates a new action plan for a high-risk tool call.
// It enforces a maximum of 1 pending action plan per thread.
func (m *ActionPlanManager) Create(
	ctx context.Context,
	threadID uuid.UUID,
	agentID string,
	companyID uuid.UUID,
	userID uuid.UUID,
	toolName string,
	toolArgs json.RawMessage,
	impact json.RawMessage,
) (*ActionPlan, error) {
	// Enforce max 1 pending plan per thread
	count, err := m.q.CountPendingActionsByThread(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("count pending actions: %w", err)
	}
	if count >= 1 {
		return nil, fmt.Errorf("thread already has a pending action plan awaiting confirmation")
	}

	// Look up tool to get SummaryTmpl
	tool, ok := m.registry.Get(toolName)
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}

	summary := m.generateSummary(tool.SummaryTmpl, toolArgs)

	if impact == nil {
		impact = json.RawMessage(`{}`)
	}

	planID := uuid.New()
	row, err := m.q.CreateActionPlan(ctx, sqlc.CreateActionPlanParams{
		ID:        planID,
		ThreadID:  threadID,
		AgentID:   agentID,
		CompanyID: companyID,
		UserID:    userID,
		ToolName:  toolName,
		ToolArgs:  []byte(toolArgs),
		Summary:   summary,
		Impact:    []byte(impact),
	})
	if err != nil {
		return nil, fmt.Errorf("create action plan: %w", err)
	}

	plan := m.actionPlanFromRow(row)

	// Persist as chat message so the UI can render it
	if err := m.PersistAsChatMessage(ctx, plan); err != nil {
		slog.Warn("failed to persist action plan as chat message", "plan_id", planID, "error", err)
	}

	return plan, nil
}

// Confirm executes the action plan after user confirmation.
// It uses a database transaction with SELECT FOR UPDATE to prevent double-execution.
func (m *ActionPlanManager) Confirm(ctx context.Context, planID uuid.UUID, companyID uuid.UUID) (*ActionPlanResult, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	qtx := m.q.WithTx(tx)

	row, err := qtx.GetActionPlanForUpdate(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("get action plan for update: %w", err)
	}

	// Verify ownership
	if row.CompanyID != companyID {
		return nil, fmt.Errorf("action plan does not belong to this company")
	}

	// Idempotent: already executed
	if row.Status == "executed" {
		if err = tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit: %w", err)
		}
		var result json.RawMessage
		if len(row.Result) > 0 {
			result = json.RawMessage(row.Result)
		}
		return &ActionPlanResult{
			PlanID: planID,
			Status: "executed",
			Result: result,
		}, nil
	}

	// Validate status
	if row.Status != "pending" {
		return nil, fmt.Errorf("action plan is not pending (status: %s)", row.Status)
	}

	// Mark as confirmed
	if err = qtx.UpdateActionPlanStatus(ctx, sqlc.UpdateActionPlanStatusParams{
		Status:       "confirmed",
		Result:       nil,
		ErrorMessage: nil,
		ID:           planID,
	}); err != nil {
		return nil, fmt.Errorf("mark confirmed: %w", err)
	}

	// Execute the tool
	tool, ok := m.registry.Get(row.ToolName)
	if !ok {
		errMsg := fmt.Sprintf("tool not found: %s", row.ToolName)
		_ = qtx.UpdateActionPlanStatus(ctx, sqlc.UpdateActionPlanStatusParams{
			Status:       "failed",
			Result:       nil,
			ErrorMessage: strPtr(errMsg),
			ID:           planID,
		})
		_ = tx.Commit(ctx)
		return &ActionPlanResult{
			PlanID: planID,
			Status: "failed",
			Error:  errMsg,
		}, nil
	}

	tc := ToolContext{
		Ctx:       ctx,
		CompanyID: row.CompanyID,
		UserID:    row.UserID,
		AgentID:   row.AgentID,
		ThreadID:  row.ThreadID,
	}

	toolResult, execErr := tool.Execute(tc, json.RawMessage(row.ToolArgs))

	if execErr != nil {
		errMsg := execErr.Error()
		_ = qtx.UpdateActionPlanStatus(ctx, sqlc.UpdateActionPlanStatusParams{
			Status:       "failed",
			Result:       nil,
			ErrorMessage: strPtr(errMsg),
			ID:           planID,
		})
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("commit after failure: %w", commitErr)
		}
		return &ActionPlanResult{
			PlanID: planID,
			Status: "failed",
			Error:  errMsg,
		}, nil
	}

	resultBytes := []byte(toolResult)
	if err = qtx.UpdateActionPlanStatus(ctx, sqlc.UpdateActionPlanStatusParams{
		Status:       "executed",
		Result:       resultBytes,
		ErrorMessage: nil,
		ID:           planID,
	}); err != nil {
		return nil, fmt.Errorf("mark executed: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &ActionPlanResult{
		PlanID: planID,
		Status: "executed",
		Result: json.RawMessage(resultBytes),
	}, nil
}

// Cancel cancels a pending action plan.
func (m *ActionPlanManager) Cancel(ctx context.Context, planID uuid.UUID, companyID uuid.UUID) error {
	row, err := m.q.GetActionPlan(ctx, planID)
	if err != nil {
		return fmt.Errorf("get action plan: %w", err)
	}

	if row.CompanyID != companyID {
		return fmt.Errorf("action plan does not belong to this company")
	}

	if row.Status != "pending" {
		return fmt.Errorf("action plan is not pending (status: %s)", row.Status)
	}

	return m.q.UpdateActionPlanStatus(ctx, sqlc.UpdateActionPlanStatusParams{
		Status:       "cancelled",
		Result:       nil,
		ErrorMessage: nil,
		ID:           planID,
	})
}

// GetPending returns all pending action plans for a thread.
func (m *ActionPlanManager) GetPending(ctx context.Context, threadID uuid.UUID) ([]*ActionPlan, error) {
	rows, err := m.q.ListPendingActionsByThread(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("list pending actions: %w", err)
	}

	plans := make([]*ActionPlan, 0, len(rows))
	for _, row := range rows {
		plans = append(plans, m.actionPlanFromRow(row))
	}
	return plans, nil
}

// TimeoutExpired marks expired pending action plans as timed out.
// Returns the number of plans that were timed out.
func (m *ActionPlanManager) TimeoutExpired(ctx context.Context) (int64, error) {
	n, err := m.q.TimeoutExpiredActions(ctx)
	if err != nil {
		return 0, fmt.Errorf("timeout expired actions: %w", err)
	}
	return n, nil
}

// PersistAsChatMessage saves an action plan as a chat message with
// message_type="action_plan" so the UI can render a confirmation card.
func (m *ActionPlanManager) PersistAsChatMessage(ctx context.Context, plan *ActionPlan) error {
	msgType := "action_plan"
	agentID := plan.AgentID

	// Serialize the plan itself as the message content
	contentBytes, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("marshal action plan content: %w", err)
	}

	_, err = m.q.CreateAgentMessage(ctx, sqlc.CreateAgentMessageParams{
		ID:                uuid.New(),
		CompanyID:         plan.CompanyID,
		UserID:            plan.UserID,
		Role:              "assistant",
		Content:           string(contentBytes),
		ToolCalls:         []byte("[]"),
		ThreadID:          uuidToPgtype(plan.ThreadID),
		AgentID:           &agentID,
		MessageType:       &msgType,
		CitationsJson:     nil,
		ActionResultsJson: nil,
	})
	if err != nil {
		return fmt.Errorf("create agent message: %w", err)
	}
	return nil
}

// generateSummary substitutes {key} placeholders in the template with values
// from the parsed JSON args map. Unknown keys are left as-is.
func (m *ActionPlanManager) generateSummary(tmpl string, args json.RawMessage) string {
	if tmpl == "" {
		return ""
	}
	if len(args) == 0 {
		return tmpl
	}

	var params map[string]interface{}
	if err := json.Unmarshal(args, &params); err != nil {
		return tmpl
	}

	result := tmpl
	for k, v := range params {
		placeholder := "{" + k + "}"
		var value string
		switch val := v.(type) {
		case string:
			value = val
		default:
			b, err := json.Marshal(v)
			if err == nil {
				value = string(b)
			}
		}
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// actionPlanFromRow converts a sqlc ActionPlan row to the domain struct.
// All UUID fields in action_plans are NOT NULL so they use uuid.UUID directly.
func (m *ActionPlanManager) actionPlanFromRow(row sqlc.ActionPlan) *ActionPlan {
	return &ActionPlan{
		ID:        row.ID,
		ThreadID:  row.ThreadID,
		AgentID:   row.AgentID,
		CompanyID: row.CompanyID,
		UserID:    row.UserID,
		ToolName:  row.ToolName,
		ToolArgs:  json.RawMessage(row.ToolArgs),
		Summary:   row.Summary,
		Impact:    json.RawMessage(row.Impact),
		Status:    row.Status,
	}
}

// strPtr returns a pointer to the given string value.
func strPtr(s string) *string {
	return &s
}
