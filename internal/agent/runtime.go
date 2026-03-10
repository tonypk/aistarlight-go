package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// Runtime executes agent interactions using the shared LLM client.
type Runtime struct {
	registry *Registry
	ai       *openai.Client
	q        *sqlc.Queries
	execTool ToolExecuteFunc
}

// NewRuntime creates an agent runtime.
func NewRuntime(registry *Registry, ai *openai.Client, q *sqlc.Queries, execTool ToolExecuteFunc) *Runtime {
	return &Runtime{
		registry: registry,
		ai:       ai,
		q:        q,
		execTool: execTool,
	}
}

// Registry returns the agent registry.
func (rt *Runtime) Registry() *Registry {
	return rt.registry
}

// ToolCallResult holds the result of executing a tool.
type ToolCallResult struct {
	ToolName string `json:"tool_name"`
	ToolID   string `json:"tool_id"`
	Result   string `json:"result"`
}

// ProcessStream handles a streaming agent interaction.
func (rt *Runtime) ProcessStream(ctx context.Context, req AgentRequest) (<-chan StreamEvent, error) {
	if rt.ai == nil {
		return nil, fmt.Errorf("AI service not configured — set OPENAI_API_KEY to enable agents")
	}

	// Resolve agent
	agentDef, ok := rt.registry.Get(req.AgentID)
	if !ok {
		return nil, fmt.Errorf("unknown agent: %s", req.AgentID)
	}

	// Get or create thread
	threadID, isNew, err := rt.resolveThread(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("resolve thread: %w", err)
	}

	// Load thread history
	history, err := rt.q.ListMessagesByThread(ctx, sqlc.ListMessagesByThreadParams{
		ThreadID: uuidToPgtype(threadID),
		Limit:    20,
	})
	if err != nil {
		slog.Warn("failed to load thread history", "error", err)
		history = nil
	}

	// Build messages with context injection
	messages := rt.buildMessages(agentDef, req.Jurisdiction, req.Context, history, req.Content)
	tools := agentDef.ToolsFor(req.Jurisdiction)

	// Save user message
	rt.saveMessage(ctx, req.CompanyID, req.UserID, threadID, req.AgentID, "user", req.Content, nil)

	// Auto-title thread on first message
	if isNew {
		title := req.Content
		if len(title) > 60 {
			title = title[:57] + "..."
		}
		_ = rt.q.UpdateAgentThread(ctx, sqlc.UpdateAgentThreadParams{
			ID:      threadID,
			Column2: title,
		})
	}

	// First LLM call with tools (non-streaming)
	resp, err := rt.ai.ChatCompletionWithTools(ctx, messages, tools)
	if err != nil {
		return nil, fmt.Errorf("agent completion: %w", err)
	}

	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)

		tid := threadID.String()

		if len(resp.Choices) == 0 {
			ch <- StreamEvent{Token: "I couldn't generate a response."}
			ch <- StreamEvent{Done: true, Content: "I couldn't generate a response.", ThreadID: tid}
			return
		}

		choice := resp.Choices[0]
		var toolResults []ToolCallResult

		// Execute tool calls if present
		if choice.FinishReason == oai.FinishReasonToolCalls && len(choice.Message.ToolCalls) > 0 {
			messages = append(messages, choice.Message)
			for _, tc := range choice.Message.ToolCalls {
				// Send executing event for each tool
				execEvt, _ := json.Marshal([]map[string]string{{"tool_name": tc.Function.Name, "status": "executing"}})
				ch <- StreamEvent{ToolCalls: execEvt}

				result, execErr := rt.execTool(ctx, req.AgentID, tc.Function.Name, json.RawMessage(tc.Function.Arguments), req.CompanyID, req.UserID, req.Jurisdiction)
				status := "success"
				if execErr != nil {
					result = jsonError(execErr.Error())
					status = "error"
				}
				toolResults = append(toolResults, ToolCallResult{
					ToolName: tc.Function.Name,
					ToolID:   tc.ID,
					Result:   result,
				})
				messages = append(messages, oai.ChatCompletionMessage{
					Role:       oai.ChatMessageRoleTool,
					Content:    result,
					ToolCallID: tc.ID,
				})

				// Log action to audit table
				rt.logAction(ctx, threadID, req.CompanyID, req.UserID, req.AgentID, tc.Function.Name, tc.Function.Arguments, result, status)
			}

			// Send completed tool_calls event
			if tcJSON, err := json.Marshal(toolResults); err == nil {
				ch <- StreamEvent{ToolCalls: tcJSON}
			}

			// Extract actions from tool results and emit as Actions event
			if actions := extractActions(toolResults); len(actions) > 0 {
				if actJSON, err := json.Marshal(actions); err == nil {
					ch <- StreamEvent{Actions: actJSON}
				}
			}
		} else if choice.Message.Content != "" {
			// No tool calls, direct response
			content := choice.Message.Content
			ch <- StreamEvent{Token: content}
			ch <- StreamEvent{Done: true, Content: content, ThreadID: tid}
			rt.saveMessage(ctx, req.CompanyID, req.UserID, threadID, req.AgentID, "assistant", content, toolResults)
			return
		}

		// Stream follow-up response
		stream, err := rt.ai.ChatCompletionStream(ctx, messages)
		if err != nil {
			ch <- StreamEvent{Error: err.Error()}
			return
		}

		tokenCh := openai.StreamTokens(stream)
		var fullContent string
		for token := range tokenCh {
			fullContent += token
			ch <- StreamEvent{Token: token}
		}

		ch <- StreamEvent{Done: true, Content: fullContent, ThreadID: tid}
		rt.saveMessage(ctx, req.CompanyID, req.UserID, threadID, req.AgentID, "assistant", fullContent, toolResults)
	}()

	return ch, nil
}

// ListThreads returns agent threads for a company.
func (rt *Runtime) ListThreads(ctx context.Context, companyID uuid.UUID, agentID string, limit, offset int32) ([]sqlc.AgentThread, error) {
	return rt.q.ListAgentThreads(ctx, sqlc.ListAgentThreadsParams{
		CompanyID: companyID,
		Column2:   agentID,
		Limit:     limit,
		Offset:    offset,
	})
}

// ThreadMessages returns messages for a specific thread.
func (rt *Runtime) ThreadMessages(ctx context.Context, threadID uuid.UUID, limit int32) ([]sqlc.ChatMessage, error) {
	return rt.q.ListMessagesByThread(ctx, sqlc.ListMessagesByThreadParams{
		ThreadID: uuidToPgtype(threadID),
		Limit:    limit,
	})
}

func (rt *Runtime) resolveThread(ctx context.Context, req AgentRequest) (uuid.UUID, bool, error) {
	// If thread ID provided, use it
	if req.ThreadID != nil {
		return *req.ThreadID, false, nil
	}

	// Try to find existing thread for this agent + entity
	entityType := req.EntityType
	var entityID uuid.UUID
	if req.EntityID != nil {
		entityID = *req.EntityID
	}

	thread, err := rt.q.FindAgentThread(ctx, sqlc.FindAgentThreadParams{
		CompanyID: req.CompanyID,
		AgentID:   req.AgentID,
		Column3:   entityType,
		Column4:   entityID,
	})
	if err == nil {
		return thread.ID, false, nil
	}

	// Create new thread
	newID := uuid.New()
	var wt, et *string
	if req.WorkflowType != "" {
		wt = &req.WorkflowType
	}
	if entityType != "" {
		et = &entityType
	}

	ctxJSON, _ := json.Marshal(req.Context)
	if ctxJSON == nil {
		ctxJSON = []byte("{}")
	}

	thread, err = rt.q.CreateAgentThread(ctx, sqlc.CreateAgentThreadParams{
		ID:           newID,
		CompanyID:    req.CompanyID,
		UserID:       req.UserID,
		AgentID:      req.AgentID,
		WorkflowType: wt,
		EntityType:   et,
		EntityID:     uuidToPgtype(entityID),
		ContextJson:  ctxJSON,
	})
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("create thread: %w", err)
	}
	return thread.ID, true, nil
}

func (rt *Runtime) buildMessages(def *AgentDefinition, jurisdiction string, reqContext map[string]interface{}, history []sqlc.ChatMessage, userMessage string) []oai.ChatCompletionMessage {
	// Build system prompt with context injection
	systemPrompt := def.SystemPrompt(jurisdiction)
	if len(reqContext) > 0 {
		ctxParts := "\n\nCurrent user context:"
		if page, ok := reqContext["current_page"].(string); ok && page != "" {
			ctxParts += fmt.Sprintf("\n- Current page: %s", page)
		}
		if wf, ok := reqContext["workflow_type"].(string); ok && wf != "" {
			ctxParts += fmt.Sprintf("\n- Workflow: %s", wf)
		}
		if eid, ok := reqContext["entity_id"].(string); ok && eid != "" {
			ctxParts += fmt.Sprintf("\n- Entity ID: %s", eid)
		}
		systemPrompt += ctxParts
	}

	messages := []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: systemPrompt},
	}

	// Add thread history (already in ASC order)
	for _, msg := range history {
		messages = append(messages, oai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	messages = append(messages, oai.ChatCompletionMessage{
		Role:    oai.ChatMessageRoleUser,
		Content: userMessage,
	})

	return messages
}

func (rt *Runtime) logAction(ctx context.Context, threadID, companyID, userID uuid.UUID, agentID, actionName, input, result, status string) {
	inputJSON, _ := json.Marshal(map[string]string{"arguments": input})
	resultJSON, _ := json.Marshal(map[string]string{"result": result})
	_, err := rt.q.CreateAgentActionLog(ctx, sqlc.CreateAgentActionLogParams{
		ID:           uuid.New(),
		ThreadID:     uuidToPgtype(threadID),
		CompanyID:    companyID,
		UserID:       userID,
		AgentID:      agentID,
		ActionName:   actionName,
		ActionInput:  inputJSON,
		ActionResult: resultJSON,
		Status:       status,
	})
	if err != nil {
		slog.Warn("failed to log agent action", "error", err, "action", actionName)
	}
}

func (rt *Runtime) saveMessage(ctx context.Context, companyID, userID, threadID uuid.UUID, agentID, role, content string, toolCalls []ToolCallResult) {
	var toolCallsJSON []byte
	if len(toolCalls) > 0 {
		toolCallsJSON, _ = json.Marshal(toolCalls)
	} else {
		toolCallsJSON = []byte("[]")
	}

	msgType := "text"
	_, err := rt.q.CreateAgentMessage(ctx, sqlc.CreateAgentMessageParams{
		ID:          uuid.New(),
		CompanyID:   companyID,
		UserID:      userID,
		Role:        role,
		Content:     content,
		ToolCalls:   toolCallsJSON,
		ThreadID:    uuidToPgtype(threadID),
		AgentID:     &agentID,
		MessageType: &msgType,
	})
	if err != nil {
		slog.Warn("failed to save agent message", "error", err)
	}
}

func uuidToPgtype(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

// ActionButton represents a clickable action in the chat UI.
type ActionButton struct {
	Label string `json:"label"`
	Route string `json:"route"`
	Type  string `json:"type"` // "navigate", "action"
}

// extractActions inspects tool results for action fields and returns UI buttons.
func extractActions(results []ToolCallResult) []ActionButton {
	var actions []ActionButton
	for _, r := range results {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(r.Result), &data); err != nil {
			continue
		}
		action, _ := data["action"].(string)
		if action == "" {
			continue
		}
		label, _ := data["action_label"].(string)
		route, _ := data["action_route"].(string)
		if label == "" {
			// Default labels for known actions
			switch action {
			case "upload_required":
				label = "Upload Data"
				if route == "" {
					route = "/upload"
				}
			case "view_report":
				label = "View Report"
			default:
				label = action
			}
		}
		if route == "" {
			continue
		}
		actions = append(actions, ActionButton{
			Label: label,
			Route: route,
			Type:  "navigate",
		})
	}
	return actions
}

func jsonError(msg string) string {
	result, _ := json.Marshal(map[string]string{"error": msg})
	return string(result)
}
