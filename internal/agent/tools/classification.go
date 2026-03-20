package tools

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/agent"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// RegisterClassification registers classification-related tools into the ToolRegistry.
func RegisterClassification(r *agent.ToolRegistry, sessionSvc *service.SessionService, classifierSvc *service.ClassifierService) {
	r.Register(listSessionsTool(sessionSvc))
	r.Register(getSessionSummaryTool(sessionSvc))
	r.Register(previewClassificationTool(sessionSvc, classifierSvc))
	r.Register(classifyTransactionsTool(sessionSvc))
	r.Register(updateTransactionTool(sessionSvc))
}

func listSessionsTool(sessionSvc *service.SessionService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "list_sessions",
		Description: "List upload sessions for the company",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"limit": {
					"type": "integer",
					"description": "Maximum number of sessions to return (default 20, max 50)"
				},
				"offset": {
					"type": "integer",
					"description": "Number of sessions to skip (default 0)"
				}
			},
			"required": []
		}`),
		RiskLevel: agent.RiskLow,
		AgentIDs:  []string{"*"},
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				Limit  int `json:"limit"`
				Offset int `json:"offset"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			limit := params.Limit
			if limit <= 0 {
				limit = 20
			} else if limit > 50 {
				limit = 50
			}

			offset := params.Offset
			if offset < 0 {
				offset = 0
			}

			sessions, total, err := sessionSvc.ListSessions(tc.Ctx, tc.CompanyID, limit, offset)
			if err != nil {
				return nil, fmt.Errorf("failed to list sessions: %w", err)
			}

			return json.Marshal(map[string]interface{}{
				"sessions": sessions,
				"total":    total,
			})
		},
	}
}

func getSessionSummaryTool(sessionSvc *service.SessionService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "get_session_summary",
		Description: "Get details and summary for an upload session",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_id": {
					"type": "string",
					"description": "The session UUID"
				}
			},
			"required": ["session_id"]
		}`),
		RiskLevel: agent.RiskLow,
		AgentIDs:  []string{"*"},
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				SessionID string `json:"session_id"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.SessionID == "" {
				return nil, fmt.Errorf("session_id is required")
			}

			sessionID, err := uuid.Parse(params.SessionID)
			if err != nil {
				return nil, fmt.Errorf("invalid session_id: %w", err)
			}

			session, err := sessionSvc.GetSession(tc.Ctx, sessionID, tc.CompanyID)
			if err != nil {
				return nil, fmt.Errorf("failed to get session: %w", err)
			}

			return json.Marshal(session)
		},
	}
}

func previewClassificationTool(sessionSvc *service.SessionService, classifierSvc *service.ClassifierService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "preview_classification",
		Description: "Preview transaction classifications without modifying data",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_id": {
					"type": "string",
					"description": "The session UUID"
				},
				"limit": {
					"type": "integer",
					"description": "Number of sample transactions to preview (default 5, max 20)"
				}
			},
			"required": ["session_id"]
		}`),
		RiskLevel: agent.RiskLow,
		AgentIDs:  []string{"classifier"},
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				SessionID string `json:"session_id"`
				Limit     int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.SessionID == "" {
				return nil, fmt.Errorf("session_id is required")
			}

			sessionID, err := uuid.Parse(params.SessionID)
			if err != nil {
				return nil, fmt.Errorf("invalid session_id: %w", err)
			}

			limit := params.Limit
			if limit <= 0 {
				limit = 5
			} else if limit > 20 {
				limit = 20
			}

			txns, _, err := sessionSvc.ListTransactions(tc.Ctx, sessionID, tc.CompanyID, limit, 0, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to list transactions: %w", err)
			}

			txnMaps := make([]map[string]interface{}, 0, len(txns))
			for _, t := range txns {
				m := map[string]interface{}{
					"source_type": t.SourceType,
					"amount":      t.Amount,
					"vat_type":    t.VATType,
					"tin":         "",
				}
				if t.Description != nil {
					m["description"] = *t.Description
				}
				if t.Date != nil {
					m["date"] = *t.Date
				}
				if t.TIN != nil {
					m["tin"] = *t.TIN
				}
				txnMaps = append(txnMaps, m)
			}

			jurisdiction := tc.Jurisdiction
			if jurisdiction == "" {
				jurisdiction = "PH"
			}

			results, err := classifierSvc.ClassifyTransactions(tc.Ctx, txnMaps, tc.CompanyID, jurisdiction, "")
			if err != nil {
				return nil, fmt.Errorf("classification preview failed: %w", err)
			}

			return json.Marshal(map[string]interface{}{
				"preview":      results,
				"sample_count": len(txnMaps),
			})
		},
	}
}

func classifyTransactionsTool(sessionSvc *service.SessionService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "classify_transactions",
		Description: "Run AI classification on all transactions in a session",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_id": {
					"type": "string",
					"description": "The session UUID"
				},
				"force": {
					"type": "boolean",
					"description": "Force re-classification even if already classified (default false)"
				}
			},
			"required": ["session_id"]
		}`),
		RiskLevel:   agent.RiskHigh,
		AgentIDs:    []string{"classifier"},
		SummaryTmpl: "Classify transactions in session {session_id}",
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				SessionID string `json:"session_id"`
				Force     bool   `json:"force"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.SessionID == "" {
				return nil, fmt.Errorf("session_id is required")
			}

			sessionID, err := uuid.Parse(params.SessionID)
			if err != nil {
				return nil, fmt.Errorf("invalid session_id: %w", err)
			}

			result, err := sessionSvc.ClassifySession(tc.Ctx, sessionID, tc.CompanyID, params.Force)
			if err != nil {
				return nil, fmt.Errorf("classification failed: %w", err)
			}

			return json.Marshal(result)
		},
	}
}

func updateTransactionTool(sessionSvc *service.SessionService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "update_transaction",
		Description: "Update a transaction's classification or details",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"transaction_id": {
					"type": "string",
					"description": "The transaction UUID"
				},
				"session_id": {
					"type": "string",
					"description": "The session UUID"
				},
				"category": {
					"type": "string",
					"description": "New category for the transaction"
				},
				"vat_type": {
					"type": "string",
					"description": "New VAT type (e.g. vatable, exempt, zero_rated)"
				}
			},
			"required": ["transaction_id", "session_id"]
		}`),
		RiskLevel:   agent.RiskHigh,
		AgentIDs:    []string{"classifier", "reconciliation"},
		SummaryTmpl: "Update transaction {transaction_id}",
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				TransactionID string `json:"transaction_id"`
				SessionID     string `json:"session_id"`
				Category      string `json:"category"`
				VATType       string `json:"vat_type"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.TransactionID == "" {
				return nil, fmt.Errorf("transaction_id is required")
			}
			if params.SessionID == "" {
				return nil, fmt.Errorf("session_id is required")
			}

			txnID, err := uuid.Parse(params.TransactionID)
			if err != nil {
				return nil, fmt.Errorf("invalid transaction_id: %w", err)
			}

			sessionID, err := uuid.Parse(params.SessionID)
			if err != nil {
				return nil, fmt.Errorf("invalid session_id: %w", err)
			}

			updates := make(map[string]interface{})
			if params.Category != "" {
				updates["category"] = params.Category
			}
			if params.VATType != "" {
				updates["vat_type"] = params.VATType
			}

			if len(updates) == 0 {
				return nil, fmt.Errorf("at least one of category or vat_type is required")
			}

			txn, err := sessionSvc.UpdateTransaction(tc.Ctx, txnID, sessionID, tc.CompanyID, updates)
			if err != nil {
				return nil, fmt.Errorf("failed to update transaction: %w", err)
			}

			return json.Marshal(txn)
		},
	}
}
