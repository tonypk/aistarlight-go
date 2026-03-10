package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// ToolExecutor routes tool calls from agents to existing service methods.
type ToolExecutor struct {
	knowledge   *service.KnowledgeService
	q           interface{ GetUserPreferenceByCompanyAndType(ctx context.Context, arg interface{}) (interface{}, error) }
	reportQ     reportQuerier
	chatSvc     *service.ChatService // reuse existing tool execution for backward compat
}

type reportQuerier interface {
	ListReportsByCompanyAndType(ctx context.Context, arg interface{}) (interface{}, error)
}

// NewToolExecutor creates a tool executor that bridges to existing services.
// For simplicity, it wraps the existing ChatService tool execution.
func NewToolExecutor(chatSvc *service.ChatService) *ToolExecutor {
	return &ToolExecutor{chatSvc: chatSvc}
}

// Execute routes a tool call to the appropriate service method.
func (te *ToolExecutor) Execute(ctx context.Context, agentID, toolName string, args json.RawMessage, companyID uuid.UUID, userID uuid.UUID, jurisdiction string) (string, error) {
	// Delegate to ChatService's existing tool execution for shared tools
	result := te.chatSvc.ExecuteTool(ctx, toolName, string(args), companyID, jurisdiction)
	return result, nil
}

// MakeExecuteFunc creates a ToolExecuteFunc that uses this executor.
func (te *ToolExecutor) MakeExecuteFunc() ToolExecuteFunc {
	return func(ctx context.Context, agentID, toolName string, args json.RawMessage, companyID uuid.UUID, userID uuid.UUID, jurisdiction string) (string, error) {
		return te.Execute(ctx, agentID, toolName, args, companyID, userID, jurisdiction)
	}
}

func toStringFromMap(args map[string]interface{}, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
