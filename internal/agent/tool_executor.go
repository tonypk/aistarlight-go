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
func NewToolExecutor(registry *ToolRegistry, plans *ActionPlanManager, chatSvc *service.ChatService) *ToolExecutor {
	return &ToolExecutor{registry: registry, plans: plans, chatSvc: chatSvc}
}

// ExecuteResult holds the outcome of a tool execution attempt.
type ExecuteResult struct {
	Result     json.RawMessage
	ActionPlan *ActionPlan
	Error      error
}

// Execute runs a tool call, routing by risk level.
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
	plan, err := e.plans.Create(tc.Ctx, tc.ThreadID, tc.AgentID, tc.CompanyID, tc.UserID, toolName, args, impact)
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
// Only routes through ChatService fallback (no ActionPlans).
func (e *ToolExecutor) MakeExecuteFunc() ToolExecuteFunc {
	return func(ctx context.Context, agentID, toolName string, args json.RawMessage, companyID uuid.UUID, userID uuid.UUID, jurisdiction string) (string, error) {
		if e.chatSvc != nil {
			result := e.chatSvc.ExecuteTool(ctx, toolName, string(args), companyID, jurisdiction, userID)
			return result, nil
		}
		return `{"error":"tool not available in legacy mode"}`, fmt.Errorf("tool %s not available", toolName)
	}
}

func generateImpact(tool *ToolDef, args json.RawMessage) json.RawMessage {
	var m map[string]interface{}
	_ = json.Unmarshal(args, &m)
	impact := map[string]interface{}{
		"affected_count": 0,
		"details":        []string{},
	}
	for _, key := range []string{"count", "affected_count", "transaction_count"} {
		if v, ok := m[key]; ok {
			impact["affected_count"] = v
		}
	}
	result, _ := json.Marshal(impact)
	return result
}

