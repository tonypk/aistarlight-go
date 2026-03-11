package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

const chatSystemPromptPH = `AIStarlight — Your smart bookkeeping assistant for Philippine businesses.

You help users manage their business finances through natural conversation. You can:
1. Record expenses and income from text descriptions
2. Search and filter past transactions
3. Show spending summaries and category breakdowns
4. Process receipt photos (just send a photo)
5. Record P2P forex exchanges (/exchange command)
6. Export monthly bookkeeping to Excel (/export)
7. Generate BIR tax reports and check compliance
8. Answer questions about Philippine tax regulations

Supported forms: BIR_2550M, BIR_2550Q, BIR_1601C, BIR_0619E, BIR_2307, SAWT

Tool routing:
- User describes an expense or income → use record_expense tool
- User asks about spending or totals → use get_spending_summary tool
- User searches for specific transactions → use search_transactions tool
- User wants to see recent activity → use list_recent_transactions tool
- User says amount/description/date is wrong → first search for the transaction, then use update_transaction tool
- User wants to delete a wrong transaction → first search/list to find it, then use delete_transaction tool
- User asks to generate report → use generate_report tool
- User asks about tax rules → use lookup_tax_rule tool
- User asks about settings/preferences → use get_user_preferences tool

When correcting transactions: ALWAYS search or list recent transactions first to find the ID, then update or delete.

Respond concisely. Use the user's language (English/Chinese/Filipino).
Format currency amounts with ₱ symbol for PHP.`

const chatSystemPromptSG = `AIStarlight — Your smart bookkeeping assistant for Singapore businesses.

You help users manage their business finances through natural conversation. You can:
1. Record expenses and income from text descriptions
2. Search and filter past transactions
3. Show spending summaries and category breakdowns
4. Process receipt photos (just send a photo)
5. Record P2P forex exchanges (/exchange command)
6. Export monthly bookkeeping to Excel (/export)
7. Generate IRAS tax reports and check compliance
8. Answer questions about Singapore tax regulations

Supported forms: IRAS_GST_F5, IRAS_FORM_C, IRAS_FORM_CS, IRAS_FORM_B, IRAS_IR8A, IRAS_S45

Tool routing:
- User describes an expense or income → use record_expense tool
- User asks about spending or totals → use get_spending_summary tool
- User searches for specific transactions → use search_transactions tool
- User wants to see recent activity → use list_recent_transactions tool
- User says amount/description/date is wrong → first search for the transaction, then use update_transaction tool
- User wants to delete a wrong transaction → first search/list to find it, then use delete_transaction tool
- User asks to generate report → use generate_report tool
- User asks about tax rules → use lookup_tax_rule tool
- User asks about settings/preferences → use get_user_preferences tool

When correcting transactions: ALWAYS search or list recent transactions first to find the ID, then update or delete.

Respond concisely. Use the user's language (English/Chinese/Mandarin).
Format currency amounts with S$ symbol for SGD.`

const chatSystemPromptLK = `AIStarlight — Your smart bookkeeping assistant for Sri Lanka businesses.

You help users manage their business finances through natural conversation. You can:
1. Record expenses and income from text descriptions
2. Search and filter past transactions
3. Show spending summaries and category breakdowns
4. Process receipt photos (just send a photo)
5. Record P2P forex exchanges (/exchange command)
6. Export monthly bookkeeping to Excel (/export)
7. Generate IRD tax reports and check compliance
8. Answer questions about Sri Lanka tax regulations (Inland Revenue Act)

Supported forms: IRDSL_VAT_RETURN, IRDSL_CIT, IRDSL_IT_RETURN, IRDSL_PAYE, IRDSL_WHT, IRDSL_APIT, IRDSL_SSCL, IRDSL_SVAT

Tool routing:
- User describes an expense or income → use record_expense tool
- User asks about spending or totals → use get_spending_summary tool
- User searches for specific transactions → use search_transactions tool
- User wants to see recent activity → use list_recent_transactions tool
- User says amount/description/date is wrong → first search for the transaction, then use update_transaction tool
- User wants to delete a wrong transaction → first search/list to find it, then use delete_transaction tool
- User asks to generate report → use generate_report tool
- User asks about tax rules → use lookup_tax_rule tool
- User asks about settings/preferences → use get_user_preferences tool

When correcting transactions: ALWAYS search or list recent transactions first to find the ID, then update or delete.

Respond concisely. Use the user's language (English/Chinese/Sinhala/Tamil).
Format currency amounts with Rs. symbol for LKR.`

var chatToolsPH = []oai.Tool{
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

var chatToolsSG = []oai.Tool{
	{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "generate_report",
			Description: "Generate an IRAS tax report for a specific period",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"report_type": {"type": "string", "enum": ["IRAS_GST_F5", "IRAS_FORM_C", "IRAS_FORM_CS", "IRAS_FORM_B", "IRAS_IR8A", "IRAS_S45"]},
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
			Description: "Look up a Singapore tax regulation or rule",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "The tax rule query"},
					"category": {"type": "string", "enum": ["gst", "income_tax", "withholding", "compliance", "general", "payroll", "corporate"]}
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

var chatToolsLK = []oai.Tool{
	{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "generate_report",
			Description: "Generate an IRD Sri Lanka tax report for a specific period",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"report_type": {"type": "string", "enum": ["IRDSL_VAT_RETURN", "IRDSL_CIT", "IRDSL_IT_RETURN", "IRDSL_PAYE", "IRDSL_WHT", "IRDSL_APIT", "IRDSL_SSCL"]},
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
			Description: "Look up a Sri Lanka tax regulation or rule",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "The tax rule query"},
					"category": {"type": "string", "enum": ["vat", "income_tax", "withholding", "compliance", "general", "payroll", "corporate"]}
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

// bookkeepingTools are shared across all jurisdictions.
var bookkeepingTools = []oai.Tool{
	{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "record_expense",
			Description: "Record a manual expense or income transaction from the user's text description",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"amount": {"type": "number", "description": "Transaction amount (positive number)"},
					"description": {"type": "string", "description": "What the expense/income is for"},
					"category": {"type": "string", "description": "Category: goods, services, utilities, rent, transport, food, office, salary, sales, other"},
					"date": {"type": "string", "description": "Transaction date in YYYY-MM-DD format (defaults to today)"},
					"project_tag": {"type": "string", "description": "Optional project tag for categorization"}
				},
				"required": ["amount", "description"]
			}`),
		},
	},
	{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "search_transactions",
			Description: "Search past transactions by keyword, date range, or description",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "Search keyword to match in description"},
					"from_date": {"type": "string", "description": "Start date in YYYY-MM-DD format"},
					"to_date": {"type": "string", "description": "End date in YYYY-MM-DD format"},
					"limit": {"type": "integer", "description": "Max results to return (default 10)"}
				},
				"required": ["query"]
			}`),
		},
	},
	{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "get_spending_summary",
			Description: "Get spending breakdown by category or month for a date range",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"group_by": {"type": "string", "enum": ["category", "month"], "description": "Group results by category or month"},
					"from_date": {"type": "string", "description": "Start date in YYYY-MM-DD format (defaults to start of current month)"},
					"to_date": {"type": "string", "description": "End date in YYYY-MM-DD format (defaults to today)"}
				},
				"required": ["group_by"]
			}`),
		},
	},
	{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "list_recent_transactions",
			Description: "List the most recent transactions",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"limit": {"type": "integer", "description": "Number of transactions to show (default 5, max 20)"}
				}
			}`),
		},
	},
	{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "update_transaction",
			Description: "Update/correct an existing transaction. Use when the user says an amount is wrong, wants to change the description, date, or category of a recent transaction. Search for the transaction first if you don't have the ID.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"transaction_id": {"type": "string", "description": "UUID of the transaction to update"},
					"amount": {"type": "number", "description": "New corrected amount (only if user wants to change amount)"},
					"description": {"type": "string", "description": "New description (only if user wants to change description)"},
					"date": {"type": "string", "description": "New date in YYYY-MM-DD format (only if user wants to change date)"},
					"category": {"type": "string", "description": "New category (only if user wants to change category)"},
					"vat_amount": {"type": "number", "description": "New VAT amount (only if user wants to change VAT)"}
				},
				"required": ["transaction_id"]
			}`),
		},
	},
	{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "delete_transaction",
			Description: "Delete a transaction that was recorded incorrectly. Use when the user says a transaction should be removed entirely.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"transaction_id": {"type": "string", "description": "UUID of the transaction to delete"}
				},
				"required": ["transaction_id"]
			}`),
		},
	},
}

func chatToolsForJurisdiction(jurisdiction string) []oai.Tool {
	var base []oai.Tool
	switch jurisdiction {
	case "SG":
		base = chatToolsSG
	case "LK":
		base = chatToolsLK
	default:
		base = chatToolsPH
	}
	return append(base, bookkeepingTools...)
}

func chatSystemPrompt(jurisdiction string) string {
	switch jurisdiction {
	case "SG":
		return chatSystemPromptSG
	case "LK":
		return chatSystemPromptLK
	default:
		return chatSystemPromptPH
	}
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
	jurisdiction string,
	userID ...uuid.UUID,
) (*ChatResponse, error) {
	if s.ai == nil {
		return nil, fmt.Errorf("AI service not configured — set OPENAI_API_KEY to enable chat")
	}

	uid := uuid.Nil
	if len(userID) > 0 {
		uid = userID[0]
	}

	messages := s.buildMessages(history, userMessage, jurisdiction)
	tools := chatToolsForJurisdiction(jurisdiction)

	// First LLM call (may include tool calls)
	resp, err := s.ai.ChatCompletionWithTools(ctx, messages, tools)
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
		result := s.executeTool(ctx, tc.Function.Name, tc.Function.Arguments, companyID, jurisdiction, uid)
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
	jurisdiction string,
	userID ...uuid.UUID,
) (<-chan string, *[]ToolCallResult, error) {
	if s.ai == nil {
		return nil, nil, fmt.Errorf("AI service not configured — set OPENAI_API_KEY to enable chat")
	}

	uid := uuid.Nil
	if len(userID) > 0 {
		uid = userID[0]
	}

	messages := s.buildMessages(history, userMessage, jurisdiction)
	tools := chatToolsForJurisdiction(jurisdiction)

	// First LLM call with tools (non-streaming)
	resp, err := s.ai.ChatCompletionWithTools(ctx, messages, tools)
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
			result := s.executeTool(ctx, tc.Function.Name, tc.Function.Arguments, companyID, jurisdiction, uid)
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

func (s *ChatService) buildMessages(history []domain.ChatMessage, userMessage string, jurisdiction string) []oai.ChatCompletionMessage {
	messages := []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: chatSystemPrompt(jurisdiction)},
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

// ExecuteTool executes a named tool with JSON arguments. Exported for use by agent runtime.
func (s *ChatService) ExecuteTool(ctx context.Context, name, argsJSON string, companyID uuid.UUID, jurisdiction string, userID ...uuid.UUID) string {
	uid := uuid.Nil
	if len(userID) > 0 {
		uid = userID[0]
	}
	return s.executeTool(ctx, name, argsJSON, companyID, jurisdiction, uid)
}

func (s *ChatService) executeTool(ctx context.Context, name, argsJSON string, companyID uuid.UUID, jurisdiction string, userID uuid.UUID) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return jsonError("invalid tool arguments")
	}

	switch name {
	case "generate_report":
		return s.executeGenerateReport(ctx, args, companyID)
	case "lookup_tax_rule":
		return s.executeLookupTaxRule(ctx, args, jurisdiction)
	case "get_user_preferences":
		return s.executeGetPreferences(ctx, args, companyID)
	case "validate_report":
		return s.executeValidateReport(ctx, args, companyID)
	case "record_expense":
		return s.executeRecordExpense(ctx, args, companyID, userID)
	case "search_transactions":
		return s.executeSearchTransactions(ctx, args, companyID)
	case "get_spending_summary":
		return s.executeGetSpendingSummary(ctx, args, companyID)
	case "list_recent_transactions":
		return s.executeListRecentTransactions(ctx, args, companyID)
	case "update_transaction":
		return s.executeUpdateTransaction(ctx, args, companyID)
	case "delete_transaction":
		return s.executeDeleteTransaction(ctx, args, companyID)
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
					"action":             "view_report",
					"action_label":       "View Report",
					"action_route":       fmt.Sprintf("/reports/%s", r.ID.String()),
				})
				return string(result)
			}
		}
	}

	result, _ := json.Marshal(map[string]interface{}{
		"action":       "upload_required",
		"action_label": "Upload Data",
		"action_route": "/upload",
		"report_type":  reportType,
		"period":       period,
		"message":      fmt.Sprintf("To generate %s for %s, please upload your sales/purchase data first.", reportType, period),
	})
	return string(result)
}

func (s *ChatService) executeLookupTaxRule(ctx context.Context, args map[string]interface{}, jurisdiction string) string {
	query := toString(args["query"])
	category := toString(args["category"])

	var catPtr *string
	if category != "" {
		catPtr = &category
	}

	chunks, err := s.knowledge.RetrieveRelevant(ctx, query, catPtr, 3, jurisdiction)
	if err != nil || len(chunks) == 0 {
		noResultMsg := "No specific regulation found. Please consult the BIR website (www.bir.gov.ph)."
		if jurisdiction == "SG" {
			noResultMsg = "No specific regulation found. Please consult the IRAS website (www.iras.gov.sg)."
		}
		result, _ := json.Marshal(map[string]interface{}{
			"answer":  noResultMsg,
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

	answer, err := s.knowledge.AnswerQuestion(ctx, query, chunks, nil, jurisdiction)
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

func (s *ChatService) executeValidateReport(ctx context.Context, args map[string]interface{}, companyID uuid.UUID) string {
	reportIDStr := toString(args["report_id"])
	reportID, err := uuid.Parse(reportIDStr)
	if err != nil {
		return jsonError("invalid report_id format")
	}

	report, err := s.q.GetReportByID(ctx, reportID)
	if err != nil {
		return jsonError("report not found")
	}
	if report.CompanyID != companyID {
		return jsonError("report not found")
	}

	var calcData map[string]interface{}
	if len(report.CalculatedData) > 0 {
		_ = json.Unmarshal(report.CalculatedData, &calcData)
	}
	if calcData == nil {
		calcData = make(map[string]interface{})
	}
	calcData["period"] = report.Period
	calcData["report_type"] = report.ReportType

	checks := RunAllChecks(calcData, report.ReportType, nil, nil)

	critical := 0
	high := 0
	issues := make([]map[string]string, 0, len(checks))
	for _, c := range checks {
		if !c.Passed {
			issues = append(issues, map[string]string{
				"check":    c.CheckName,
				"severity": c.Severity,
				"message":  c.Message,
			})
			switch c.Severity {
			case "critical":
				critical++
			case "high":
				high++
			}
		}
	}

	status := "compliant"
	if critical > 0 {
		status = "critical_issues"
	} else if high > 0 {
		status = "issues_found"
	} else if len(issues) > 0 {
		status = "minor_issues"
	}

	result, _ := json.Marshal(map[string]interface{}{
		"report_id":   report.ID.String(),
		"report_type": report.ReportType,
		"period":      report.Period,
		"status":      status,
		"total_checks": len(checks),
		"issues":      issues,
		"action":      "view_report",
		"action_label": "View Report Details",
		"action_route": fmt.Sprintf("/reports/%s", report.ID.String()),
	})
	return string(result)
}

func (s *ChatService) executeRecordExpense(ctx context.Context, args map[string]interface{}, companyID, userID uuid.UUID) string {
	amountF, ok := args["amount"].(float64)
	if !ok || amountF == 0 {
		return jsonError("amount is required and must be a number")
	}
	description := toString(args["description"])
	if description == "" {
		return jsonError("description is required")
	}

	category := toString(args["category"])
	if category == "" {
		category = "other"
	}

	dateStr := toString(args["date"])
	txDate := time.Now()
	if dateStr != "" {
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			txDate = parsed
		}
	}

	projectTag := toString(args["project_tag"])

	// Get or create session for current period.
	period := txDate.Format("2006-01")
	sessionID, err := s.getOrCreateSession(ctx, companyID, userID, period)
	if err != nil {
		slog.Error("failed to get/create session for expense", "error", err)
		return jsonError("failed to create session")
	}

	// Build pgtype.Numeric from float.
	amountNum := floatToNumeric(amountF)
	confNum := floatToNumeric(1.0)

	var descPtr *string
	if description != "" {
		descPtr = &description
	}
	var projPtr *string
	if projectTag != "" {
		projPtr = &projectTag
	}

	submittedByPg := pgtype.UUID{}
	if userID != uuid.Nil {
		submittedByPg = pgtype.UUID{Bytes: userID, Valid: true}
	}

	tx, err := s.q.CreateTransaction(ctx, sqlc.CreateTransactionParams{
		ID:                   uuid.New(),
		CompanyID:            companyID,
		SessionID:            sessionID,
		SourceType:           "manual",
		SourceFileID:         "telegram_chat",
		RowIndex:             0,
		Date:                 pgtype.Date{Time: txDate, Valid: true},
		Description:          descPtr,
		Amount:               amountNum,
		VatType:              "none",
		Category:             category,
		Confidence:           confNum,
		ClassificationSource: "manual",
		RawData:              []byte("{}"),
		MatchStatus:          "unmatched",
		ProjectTag:           projPtr,
		SubmittedBy:          submittedByPg,
	})
	if err != nil {
		slog.Error("failed to create manual transaction", "error", err)
		return jsonError("failed to record transaction")
	}

	result, _ := json.Marshal(map[string]interface{}{
		"success":        true,
		"transaction_id": tx.ID.String(),
		"amount":         amountF,
		"description":    description,
		"category":       category,
		"date":           txDate.Format("2006-01-02"),
		"project_tag":    projectTag,
		"message":        fmt.Sprintf("Recorded: %s — %.2f (%s)", description, amountF, category),
	})
	return string(result)
}

func (s *ChatService) executeSearchTransactions(ctx context.Context, args map[string]interface{}, companyID uuid.UUID) string {
	query := toString(args["query"])
	limit := toIntDefault(args["limit"], 10)
	if limit > 50 {
		limit = 50
	}

	fromDate := parseDateArg(args["from_date"])
	toDate := parseDateArg(args["to_date"])

	rows, err := s.q.SearchTransactionsByCompany(ctx, sqlc.SearchTransactionsByCompanyParams{
		CompanyID: companyID,
		Column2:   query,
		Column3:   fromDate,
		Column4:   toDate,
		Limit:     int32(limit),
		Offset:    0,
	})
	if err != nil {
		slog.Error("search transactions failed", "error", err)
		return jsonError("failed to search transactions")
	}

	items := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		items = append(items, formatTransactionSummary(r))
	}

	result, _ := json.Marshal(map[string]interface{}{
		"count":        len(items),
		"transactions": items,
		"query":        query,
	})
	return string(result)
}

func (s *ChatService) executeGetSpendingSummary(ctx context.Context, args map[string]interface{}, companyID uuid.UUID) string {
	groupBy := toString(args["group_by"])
	if groupBy == "" {
		groupBy = "category"
	}

	now := time.Now()
	fromDate := parseDateArg(args["from_date"])
	toDate := parseDateArg(args["to_date"])

	// Default to current month.
	if !fromDate.Valid {
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		fromDate = pgtype.Date{Time: monthStart, Valid: true}
	}
	if !toDate.Valid {
		toDate = pgtype.Date{Time: now, Valid: true}
	}

	if groupBy == "month" {
		rows, err := s.q.GetSpendingSummaryByMonth(ctx, sqlc.GetSpendingSummaryByMonthParams{
			CompanyID: companyID,
			Date:      fromDate,
			Date_2:    toDate,
		})
		if err != nil {
			slog.Error("spending summary by month failed", "error", err)
			return jsonError("failed to get spending summary")
		}
		items := make([]map[string]interface{}, 0, len(rows))
		for _, r := range rows {
			items = append(items, map[string]interface{}{
				"month": r.Month,
				"count": r.Count,
				"total": r.Total,
			})
		}
		result, _ := json.Marshal(map[string]interface{}{
			"group_by":  "month",
			"from_date": fromDate.Time.Format("2006-01-02"),
			"to_date":   toDate.Time.Format("2006-01-02"),
			"breakdown": items,
		})
		return string(result)
	}

	// Default: by category.
	rows, err := s.q.GetSpendingSummaryByCategory(ctx, sqlc.GetSpendingSummaryByCategoryParams{
		CompanyID: companyID,
		Date:      fromDate,
		Date_2:    toDate,
	})
	if err != nil {
		slog.Error("spending summary by category failed", "error", err)
		return jsonError("failed to get spending summary")
	}
	items := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		items = append(items, map[string]interface{}{
			"category": r.Category,
			"count":    r.Count,
			"total":    r.Total,
		})
	}
	result, _ := json.Marshal(map[string]interface{}{
		"group_by":  "category",
		"from_date": fromDate.Time.Format("2006-01-02"),
		"to_date":   toDate.Time.Format("2006-01-02"),
		"breakdown": items,
	})
	return string(result)
}

func (s *ChatService) executeListRecentTransactions(ctx context.Context, args map[string]interface{}, companyID uuid.UUID) string {
	limit := toIntDefault(args["limit"], 5)
	if limit > 20 {
		limit = 20
	}

	rows, err := s.q.GetRecentTransactionsByCompany(ctx, sqlc.GetRecentTransactionsByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(limit),
	})
	if err != nil {
		slog.Error("list recent transactions failed", "error", err)
		return jsonError("failed to list transactions")
	}

	items := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		items = append(items, formatTransactionSummary(r))
	}

	result, _ := json.Marshal(map[string]interface{}{
		"count":        len(items),
		"transactions": items,
	})
	return string(result)
}

func (s *ChatService) executeUpdateTransaction(ctx context.Context, args map[string]interface{}, companyID uuid.UUID) string {
	txnIDStr := toString(args["transaction_id"])
	txnID, err := uuid.Parse(txnIDStr)
	if err != nil {
		return jsonError("invalid transaction_id format")
	}

	// Verify ownership.
	txn, err := s.q.GetTransactionByID(ctx, txnID)
	if err != nil {
		return jsonError("transaction not found")
	}
	if txn.CompanyID != companyID {
		return jsonError("transaction not found")
	}

	params := sqlc.UpdateTransactionFieldsParams{
		ID:        txnID,
		CompanyID: companyID,
	}

	changes := make([]string, 0)

	if amountF, ok := args["amount"].(float64); ok {
		params.SetAmount = true
		params.NewAmount = floatToNumeric(amountF)
		changes = append(changes, fmt.Sprintf("amount → %.2f", amountF))
	}
	if desc := toString(args["description"]); desc != "" {
		params.SetDescription = true
		params.NewDescription = desc
		changes = append(changes, fmt.Sprintf("description → %s", desc))
	}
	if dateStr := toString(args["date"]); dateStr != "" {
		if parsed, parseErr := time.Parse("2006-01-02", dateStr); parseErr == nil {
			params.SetDate = true
			params.NewDate = pgtype.Date{Time: parsed, Valid: true}
			changes = append(changes, fmt.Sprintf("date → %s", dateStr))
		}
	}
	if cat := toString(args["category"]); cat != "" {
		params.SetCategory = true
		params.NewCategory = cat
		changes = append(changes, fmt.Sprintf("category → %s", cat))
	}
	if vatF, ok := args["vat_amount"].(float64); ok {
		params.SetVatAmount = true
		params.NewVatAmount = floatToNumeric(vatF)
		changes = append(changes, fmt.Sprintf("vat_amount → %.2f", vatF))
	}

	if len(changes) == 0 {
		return jsonError("no fields to update — specify amount, description, date, category, or vat_amount")
	}

	updated, err := s.q.UpdateTransactionFields(ctx, params)
	if err != nil {
		slog.Error("failed to update transaction", "error", err, "id", txnID)
		return jsonError("failed to update transaction")
	}

	desc := ""
	if updated.Description != nil {
		desc = *updated.Description
	}
	var amt float64
	if f, fErr := updated.Amount.Float64Value(); fErr == nil {
		amt = f.Float64
	}

	result, _ := json.Marshal(map[string]interface{}{
		"success":        true,
		"transaction_id": updated.ID.String(),
		"description":    desc,
		"amount":         amt,
		"changes":        changes,
		"message":        fmt.Sprintf("Updated transaction: %s", strings.Join(changes, ", ")),
	})
	return string(result)
}

func (s *ChatService) executeDeleteTransaction(ctx context.Context, args map[string]interface{}, companyID uuid.UUID) string {
	txnIDStr := toString(args["transaction_id"])
	txnID, err := uuid.Parse(txnIDStr)
	if err != nil {
		return jsonError("invalid transaction_id format")
	}

	// Verify ownership.
	txn, err := s.q.GetTransactionByID(ctx, txnID)
	if err != nil {
		return jsonError("transaction not found")
	}
	if txn.CompanyID != companyID {
		return jsonError("transaction not found")
	}

	desc := ""
	if txn.Description != nil {
		desc = *txn.Description
	}
	var amt float64
	if f, fErr := txn.Amount.Float64Value(); fErr == nil {
		amt = f.Float64
	}

	err = s.q.DeleteTransactionByIDAndCompany(ctx, sqlc.DeleteTransactionByIDAndCompanyParams{
		ID:        txnID,
		CompanyID: companyID,
	})
	if err != nil {
		slog.Error("failed to delete transaction", "error", err, "id", txnID)
		return jsonError("failed to delete transaction")
	}

	result, _ := json.Marshal(map[string]interface{}{
		"success":     true,
		"deleted_id":  txnID.String(),
		"description": desc,
		"amount":      amt,
		"message":     fmt.Sprintf("Deleted transaction: %s (%.2f)", desc, amt),
	})
	return string(result)
}

// getOrCreateSession finds or creates a reconciliation session for the period.
func (s *ChatService) getOrCreateSession(ctx context.Context, companyID, userID uuid.UUID, period string) (uuid.UUID, error) {
	session, err := s.q.GetActiveSessionByCompanyAndPeriod(ctx, sqlc.GetActiveSessionByCompanyAndPeriodParams{
		CompanyID: companyID,
		Period:    period,
	})
	if err == nil {
		return session.ID, nil
	}

	createdBy := userID
	if createdBy == uuid.Nil {
		createdBy = companyID // fallback: use companyID as placeholder
	}

	sourceFiles, _ := json.Marshal([]map[string]string{{"source": "telegram_bot"}})
	newSession, err := s.q.CreateReconciliationSession(ctx, sqlc.CreateReconciliationSessionParams{
		ID:          uuid.New(),
		CompanyID:   companyID,
		CreatedBy:   createdBy,
		Period:      period,
		Status:      "active",
		SourceFiles: sourceFiles,
	})
	if err != nil {
		// TOCTOU: retry lookup once.
		session, retryErr := s.q.GetActiveSessionByCompanyAndPeriod(ctx, sqlc.GetActiveSessionByCompanyAndPeriodParams{
			CompanyID: companyID,
			Period:    period,
		})
		if retryErr == nil {
			return session.ID, nil
		}
		return uuid.Nil, fmt.Errorf("create session: %w", err)
	}
	return newSession.ID, nil
}

// formatTransactionSummary builds a concise map for a transaction.
func formatTransactionSummary(t sqlc.Transaction) map[string]interface{} {
	m := map[string]interface{}{
		"id":          t.ID.String(),
		"source_type": t.SourceType,
		"category":    t.Category,
	}
	if t.Description != nil {
		m["description"] = *t.Description
	}
	if t.Amount.Valid {
		f, _ := t.Amount.Float64Value()
		m["amount"] = math.Round(f.Float64*100) / 100
	}
	if t.Date.Valid {
		m["date"] = t.Date.Time.Format("2006-01-02")
	}
	if t.ProjectTag != nil {
		m["project_tag"] = *t.ProjectTag
	}
	return m
}

// parseDateArg parses a date string arg from LLM tool call.
func parseDateArg(v interface{}) pgtype.Date {
	s := toString(v)
	if s == "" {
		return pgtype.Date{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t, Valid: true}
}

// toInt extracts an integer from interface{} with a default.
func toIntDefault(v interface{}, def int) int {
	if v == nil {
		return def
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case string:
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return def
}

// floatToNumeric converts a float64 to pgtype.Numeric.
func floatToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(fmt.Sprintf("%.2f", f))
	return n
}

func jsonError(msg string) string {
	result, _ := json.Marshal(map[string]string{"error": msg})
	return string(result)
}
