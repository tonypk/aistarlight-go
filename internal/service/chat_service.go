package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

const chatSystemPrompt = `AIStarlight - AI-powered Philippine tax filing assistant for SMEs.

Your capabilities:
1. Process uploaded financial data (sales/purchase records, bank statements, receipts)
2. Calculate VAT, withholding tax, generate BIR reports
3. AI-powered transaction classification and column mapping
4. Bank & billing auto-reconciliation (CSV/Excel/PDF/image)
5. Receipt OCR scanning and data extraction
6. EWT classification, BIR 2307 certificate generation, SAWT
7. Compliance validation and anomaly detection
8. Remember user preferences for recurring filings
9. Answer questions about Philippine tax regulations (289 knowledge entries)

Supported forms: BIR_2550M, BIR_2550Q, BIR_1601C, BIR_0619E, BIR_2307, SAWT
(BIR 1701, 1702, 2316 coming soon)

Tool routing:
- User asks to generate report → use generate_report tool
- User asks about tax rules → use lookup_tax_rule tool
- User asks about settings/preferences → use get_user_preferences tool

Use language user writes in (English or Filipino).`

var chatTools = []oai.Tool{
	{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "generate_report",
			Description: "Generate a BIR tax report for a specific period",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"report_type": {"type": "string", "enum": ["BIR_2550M", "BIR_2550Q", "BIR_1601C", "BIR_0619E"]},
					"period": {"type": "string", "description": "YYYY-MM format"}
				},
				"required": ["report_type", "period"]
			}`),
		},
	},
	{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "lookup_tax_rule",
			Description: "Look up a Philippine tax regulation or rule",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "The tax rule query"},
					"category": {"type": "string", "enum": ["vat", "income_tax", "withholding", "compliance", "general", "payroll", "incentives"]}
				},
				"required": ["query"]
			}`),
		},
	},
	{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "get_user_preferences",
			Description: "Retrieve saved user preferences for a report type",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"report_type": {"type": "string"}
				},
				"required": ["report_type"]
			}`),
		},
	},
}

// ChatService handles AI agent orchestration with tool calling.
type ChatService struct {
	ai        *openai.Client
	q         *sqlc.Queries
	knowledge *KnowledgeService
}

// NewChatService creates a chat service.
func NewChatService(ai *openai.Client, q *sqlc.Queries, knowledge *KnowledgeService) *ChatService {
	return &ChatService{ai: ai, q: q, knowledge: knowledge}
}

// ToolCallResult holds the result of executing a tool.
type ToolCallResult struct {
	ToolName string `json:"tool_name"`
	ToolID   string `json:"tool_id"`
	Result   string `json:"result"`
}

// ChatResponse is the non-streaming response.
type ChatResponse struct {
	Response  string           `json:"response"`
	ToolCalls []ToolCallResult `json:"tool_calls,omitempty"`
}

// ProcessMessage handles a non-streaming chat message with tool execution.
func (s *ChatService) ProcessMessage(
	ctx context.Context,
	userMessage string,
	history []domain.ChatMessage,
	companyID uuid.UUID,
) (*ChatResponse, error) {
	if s.ai == nil {
		return nil, fmt.Errorf("AI service not configured — set OPENAI_API_KEY to enable chat")
	}

	messages := s.buildMessages(history, userMessage)

	// First LLM call (may include tool calls)
	resp, err := s.ai.ChatCompletionWithTools(ctx, messages, chatTools)
	if err != nil {
		return nil, fmt.Errorf("chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return &ChatResponse{Response: "I couldn't generate a response."}, nil
	}

	choice := resp.Choices[0]

	// Direct response (no tool calls)
	if choice.FinishReason != oai.FinishReasonToolCalls || len(choice.Message.ToolCalls) == 0 {
		return &ChatResponse{Response: choice.Message.Content}, nil
	}

	// Execute tool calls
	messages = append(messages, choice.Message)
	var toolResults []ToolCallResult

	for _, tc := range choice.Message.ToolCalls {
		result := s.executeTool(ctx, tc.Function.Name, tc.Function.Arguments, companyID)
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
	}

	// Follow-up LLM call with tool results
	followUp, err := s.ai.ChatCompletion(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("follow-up completion: %w", err)
	}

	response := ""
	if len(followUp.Choices) > 0 {
		response = followUp.Choices[0].Message.Content
	}

	return &ChatResponse{
		Response:  response,
		ToolCalls: toolResults,
	}, nil
}

// ProcessMessageStream handles a streaming chat message.
// It executes tools first (non-streaming), then streams the final response.
func (s *ChatService) ProcessMessageStream(
	ctx context.Context,
	userMessage string,
	history []domain.ChatMessage,
	companyID uuid.UUID,
) (<-chan string, *[]ToolCallResult, error) {
	if s.ai == nil {
		return nil, nil, fmt.Errorf("AI service not configured — set OPENAI_API_KEY to enable chat")
	}

	messages := s.buildMessages(history, userMessage)

	// First LLM call with tools (non-streaming)
	resp, err := s.ai.ChatCompletionWithTools(ctx, messages, chatTools)
	if err != nil {
		return nil, nil, fmt.Errorf("chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		ch := make(chan string, 1)
		ch <- "I couldn't generate a response."
		close(ch)
		return ch, nil, nil
	}

	choice := resp.Choices[0]
	var toolResults []ToolCallResult

	// If tool calls, execute them first
	if choice.FinishReason == oai.FinishReasonToolCalls && len(choice.Message.ToolCalls) > 0 {
		messages = append(messages, choice.Message)
		for _, tc := range choice.Message.ToolCalls {
			result := s.executeTool(ctx, tc.Function.Name, tc.Function.Arguments, companyID)
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
		}
	} else if choice.Message.Content != "" {
		// No tool calls, just stream the direct response
		ch := make(chan string, 1)
		ch <- choice.Message.Content
		close(ch)
		return ch, nil, nil
	}

	// Stream the follow-up response
	stream, err := s.ai.ChatCompletionStream(ctx, messages)
	if err != nil {
		return nil, nil, fmt.Errorf("stream completion: %w", err)
	}

	tokenCh := openai.StreamTokens(stream)
	return tokenCh, &toolResults, nil
}

// SaveMessage persists a chat message to the database.
func (s *ChatService) SaveMessage(ctx context.Context, companyID, userID uuid.UUID, role, content string, toolCalls []ToolCallResult) error {
	var toolCallsJSON []byte
	if len(toolCalls) > 0 {
		toolCallsJSON, _ = json.Marshal(toolCalls)
	} else {
		toolCallsJSON = []byte("[]")
	}

	_, err := s.q.CreateChatMessage(ctx, sqlc.CreateChatMessageParams{
		ID:        uuid.New(),
		CompanyID: companyID,
		UserID:    userID,
		Role:      role,
		Content:   content,
		ToolCalls: toolCallsJSON,
	})
	return err
}

// ListHistory retrieves chat history for a company.
func (s *ChatService) ListHistory(ctx context.Context, companyID uuid.UUID, limit int) ([]domain.ChatMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q.ListChatMessagesByCompany(ctx, sqlc.ListChatMessagesByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list chat history: %w", err)
	}

	messages := make([]domain.ChatMessage, len(rows))
	for i, r := range rows {
		messages[i] = domain.ChatMessage{
			ID:        r.ID,
			CompanyID: r.CompanyID,
			UserID:    r.UserID,
			Role:      r.Role,
			Content:   r.Content,
			ToolCalls: domain.JSON(r.ToolCalls),
			CreatedAt: r.CreatedAt,
		}
	}
	return messages, nil
}

func (s *ChatService) buildMessages(history []domain.ChatMessage, userMessage string) []oai.ChatCompletionMessage {
	messages := []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: chatSystemPrompt},
	}

	// Add history (last 20 messages, in chronological order)
	maxHistory := 20
	start := 0
	if len(history) > maxHistory {
		start = len(history) - maxHistory
	}
	// History is in DESC order from DB, reverse it
	for i := len(history) - 1; i >= start; i-- {
		msg := history[i]
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

func (s *ChatService) executeTool(ctx context.Context, name, argsJSON string, companyID uuid.UUID) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return jsonError("invalid tool arguments")
	}

	switch name {
	case "generate_report":
		return s.executeGenerateReport(ctx, args, companyID)
	case "lookup_tax_rule":
		return s.executeLookupTaxRule(ctx, args)
	case "get_user_preferences":
		return s.executeGetPreferences(ctx, args, companyID)
	default:
		return jsonError(fmt.Sprintf("unknown tool: %s", name))
	}
}

func (s *ChatService) executeGenerateReport(ctx context.Context, args map[string]interface{}, companyID uuid.UUID) string {
	reportType := toString(args["report_type"])
	period := toString(args["period"])

	// Check if report already exists
	rows, err := s.q.ListReportsByCompanyAndType(ctx, sqlc.ListReportsByCompanyAndTypeParams{
		CompanyID:  companyID,
		ReportType: reportType,
		Limit:      1,
		Offset:     0,
	})
	if err == nil && len(rows) > 0 {
		for _, r := range rows {
			if r.Period == period {
				result, _ := json.Marshal(map[string]interface{}{
					"existing_report_id": r.ID.String(),
					"status":             r.Status,
					"message":            fmt.Sprintf("A %s report for %s already exists.", reportType, period),
				})
				return string(result)
			}
		}
	}

	result, _ := json.Marshal(map[string]interface{}{
		"action":      "upload_required",
		"report_type": reportType,
		"period":      period,
		"message":     fmt.Sprintf("To generate %s for %s, please upload your sales/purchase data first.", reportType, period),
	})
	return string(result)
}

func (s *ChatService) executeLookupTaxRule(ctx context.Context, args map[string]interface{}) string {
	query := toString(args["query"])
	category := toString(args["category"])

	var catPtr *string
	if category != "" {
		catPtr = &category
	}

	chunks, err := s.knowledge.RetrieveRelevant(ctx, query, catPtr, 3)
	if err != nil || len(chunks) == 0 {
		result, _ := json.Marshal(map[string]interface{}{
			"answer":  "No specific regulation found. Please consult the BIR website (www.bir.gov.ph).",
			"sources": []KnowledgeSource{},
		})
		return string(result)
	}

	// Build structured citations from chunks
	sources := make([]KnowledgeSource, 0, len(chunks))
	for _, c := range chunks {
		if c.Source != "" || c.SectionRef != "" {
			sources = append(sources, c.Citation())
		}
	}

	answer, err := s.knowledge.AnswerQuestion(ctx, query, chunks, nil)
	if err != nil {
		slog.Warn("failed to generate knowledge answer", "error", err)
		answer = chunks[0].Content
	}

	result, _ := json.Marshal(map[string]interface{}{
		"answer":  answer,
		"sources": sources,
	})
	return string(result)
}

func (s *ChatService) executeGetPreferences(ctx context.Context, args map[string]interface{}, companyID uuid.UUID) string {
	reportType := toString(args["report_type"])

	prefs, err := s.q.GetUserPreferenceByCompanyAndType(ctx, sqlc.GetUserPreferenceByCompanyAndTypeParams{
		CompanyID:  companyID,
		ReportType: reportType,
	})
	if err != nil {
		result, _ := json.Marshal(map[string]string{
			"message": "No saved preferences for " + reportType,
		})
		return string(result)
	}

	result, _ := json.Marshal(map[string]interface{}{
		"report_type":     prefs.ReportType,
		"column_mappings": json.RawMessage(prefs.ColumnMappings),
		"format_rules":    json.RawMessage(prefs.FormatRules),
	})
	return string(result)
}

func jsonError(msg string) string {
	result, _ := json.Marshal(map[string]string{"error": msg})
	return string(result)
}
