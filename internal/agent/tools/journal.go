package tools

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/agent"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// RegisterJournal registers journal-related tools into the ToolRegistry.
func RegisterJournal(r *agent.ToolRegistry, journalSvc *service.JournalService, accountSvc *service.AccountService, journalGen *service.JournalGenerator) {
	r.Register(listJournalEntriesTool(journalSvc))
	r.Register(getChartOfAccountsTool(accountSvc))
	r.Register(createJournalEntriesTool(journalGen))
}

func listJournalEntriesTool(journalSvc *service.JournalService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "list_journal_entries",
		Description: "List journal entries for the company with pagination",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"limit": {
					"type": "integer",
					"description": "Maximum number of entries to return (default 20, max 50)"
				},
				"offset": {
					"type": "integer",
					"description": "Number of entries to skip (default 0)"
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

			p := pagination.Params{Page: 1, Limit: limit, Offset: offset}
			entries, total, err := journalSvc.List(tc.Ctx, tc.CompanyID, p)
			if err != nil {
				return nil, fmt.Errorf("failed to list journal entries: %w", err)
			}

			return json.Marshal(map[string]interface{}{
				"entries": entries,
				"total":   total,
			})
		},
	}
}

func getChartOfAccountsTool(accountSvc *service.AccountService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "get_chart_of_accounts",
		Description: "Get the chart of accounts for the company",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"limit": {
					"type": "integer",
					"description": "Maximum number of accounts to return (default 100, max 200)"
				}
			},
			"required": []
		}`),
		RiskLevel: agent.RiskLow,
		AgentIDs:  []string{"*"},
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				Limit int `json:"limit"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			limit := params.Limit
			if limit <= 0 {
				limit = 100
			} else if limit > 200 {
				limit = 200
			}

			p := pagination.Params{Page: 1, Limit: limit, Offset: 0}
			accounts, total, err := accountSvc.List(tc.Ctx, tc.CompanyID, p)
			if err != nil {
				return nil, fmt.Errorf("failed to list accounts: %w", err)
			}

			return json.Marshal(map[string]interface{}{
				"accounts": accounts,
				"total":    total,
			})
		},
	}
}

func createJournalEntriesTool(journalGen *service.JournalGenerator) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "create_journal_entries",
		Description: "Generate journal entries from a reconciliation session's transactions",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_id": {
					"type": "string",
					"description": "The session UUID to generate journal entries from"
				}
			},
			"required": ["session_id"]
		}`),
		RiskLevel:   agent.RiskHigh,
		AgentIDs:    []string{"journal"},
		SummaryTmpl: "Generate journal entries from session {session_id}",
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

			entries, err := journalGen.GenerateFromSession(tc.Ctx, tc.CompanyID, sessionID, tc.UserID)
			if err != nil {
				return nil, fmt.Errorf("failed to generate journal entries: %w", err)
			}

			return json.Marshal(map[string]interface{}{
				"entries_created": len(entries),
				"entries":         entries,
			})
		},
	}
}
