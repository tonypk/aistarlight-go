// Package tools provides shared tool implementations for the agent system.
package tools

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/tonypk/aistarlight-go/internal/agent"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// RegisterShared registers low-risk shared tools into the ToolRegistry.
// These tools wrap existing service methods and are available to all agents.
func RegisterShared(r *agent.ToolRegistry, knowledgeSvc *service.KnowledgeService, dashboardSvc *service.DashboardService, q *sqlc.Queries) {
	r.Register(lookupTaxRuleTool(knowledgeSvc))
	r.Register(searchKnowledgeTool(knowledgeSvc))
	r.Register(getCompanyStatsTool(dashboardSvc))
	r.Register(getUserPreferencesTool(q))
}

func lookupTaxRuleTool(knowledgeSvc *service.KnowledgeService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "lookup_tax_rule",
		Description: "Look up a tax regulation or rule using the RAG knowledge base",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "The tax rule query"
				},
				"category": {
					"type": "string",
					"enum": ["vat", "income_tax", "withholding", "compliance", "general", "payroll", "incentives", "gst", "corporate"],
					"description": "Optional category filter"
				}
			},
			"required": ["query"]
		}`),
		RiskLevel:   agent.RiskLow,
		AgentIDs:    []string{"*"},
		SummaryTmpl: "Look up tax rule: {query}",
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				Query    string `json:"query"`
				Category string `json:"category"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.Query == "" {
				return nil, fmt.Errorf("query is required")
			}

			var catPtr *string
			if params.Category != "" {
				catPtr = &params.Category
			}

			jurisdiction := tc.Jurisdiction
			if jurisdiction == "" {
				jurisdiction = "PH"
			}

			chunks, err := knowledgeSvc.RetrieveRelevant(tc.Ctx, params.Query, catPtr, 3, jurisdiction)
			if err != nil {
				return nil, fmt.Errorf("knowledge retrieval failed: %w", err)
			}
			if len(chunks) == 0 {
				return json.Marshal(map[string]interface{}{
					"answer":  fallbackMessage(jurisdiction),
					"sources": []service.KnowledgeSource{},
				})
			}

			// Build structured citations from chunks
			sources := make([]service.KnowledgeSource, 0, len(chunks))
			for _, c := range chunks {
				if c.Source != "" || c.SectionRef != "" {
					sources = append(sources, c.Citation())
				}
			}

			answer, err := knowledgeSvc.AnswerQuestion(tc.Ctx, params.Query, chunks, nil, jurisdiction)
			if err != nil {
				slog.Warn("failed to generate knowledge answer", "error", err)
				answer = chunks[0].Content
			}

			return json.Marshal(map[string]interface{}{
				"answer":  answer,
				"sources": sources,
			})
		},
	}
}

func searchKnowledgeTool(knowledgeSvc *service.KnowledgeService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "search_knowledge",
		Description: "Search the tax knowledge base for relevant chunks without generating an AI answer",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "The search query"
				},
				"category": {
					"type": "string",
					"description": "Category filter for knowledge chunks"
				},
				"limit": {
					"type": "integer",
					"description": "Maximum number of results to return (default 5)"
				}
			},
			"required": ["query"]
		}`),
		RiskLevel:   agent.RiskLow,
		AgentIDs:    []string{"*"},
		SummaryTmpl: "Search knowledge: {query}",
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				Query    string `json:"query"`
				Category string `json:"category"`
				Limit    int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.Query == "" {
				return nil, fmt.Errorf("query is required")
			}

			var catPtr *string
			if params.Category != "" {
				catPtr = &params.Category
			}

			limit := params.Limit
			if limit <= 0 {
				limit = 5
			} else if limit > 20 {
				limit = 20
			}

			jurisdiction := tc.Jurisdiction
			if jurisdiction == "" {
				jurisdiction = "PH"
			}

			chunks, err := knowledgeSvc.RetrieveRelevant(tc.Ctx, params.Query, catPtr, limit, jurisdiction)
			if err != nil {
				return nil, fmt.Errorf("knowledge search failed: %w", err)
			}

			type chunkResult struct {
				Content    string `json:"content"`
				Source     string `json:"source"`
				Category   string `json:"category"`
				SectionRef string `json:"section_ref,omitempty"`
				LawRef     string `json:"law_ref,omitempty"`
			}
			results := make([]chunkResult, 0, len(chunks))
			for _, c := range chunks {
				results = append(results, chunkResult{
					Content:    c.Content,
					Source:     c.Source,
					Category:   c.Category,
					SectionRef: c.SectionRef,
					LawRef:     c.LawRef,
				})
			}

			return json.Marshal(results)
		},
	}
}

func getCompanyStatsTool(dashboardSvc *service.DashboardService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "get_company_stats",
		Description: "Get dashboard statistics for the current company",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {},
			"required": []
		}`),
		RiskLevel:   agent.RiskLow,
		AgentIDs:    []string{"*"},
		SummaryTmpl: "Get company statistics",
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			stats, err := dashboardSvc.GetStats(tc.Ctx, tc.CompanyID)
			if err != nil {
				return nil, fmt.Errorf("failed to get company stats: %w", err)
			}

			return json.Marshal(stats)
		},
	}
}

func getUserPreferencesTool(q *sqlc.Queries) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "get_user_preferences",
		Description: "Retrieve saved user preferences for a report type",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"report_type": {
					"type": "string",
					"description": "The report type to retrieve preferences for"
				}
			},
			"required": ["report_type"]
		}`),
		RiskLevel:   agent.RiskLow,
		AgentIDs:    []string{"*"},
		SummaryTmpl: "Get preferences for {report_type}",
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				ReportType string `json:"report_type"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.ReportType == "" {
				return nil, fmt.Errorf("report_type is required")
			}

			prefs, err := q.GetUserPreferenceByCompanyAndType(tc.Ctx, sqlc.GetUserPreferenceByCompanyAndTypeParams{
				CompanyID:  tc.CompanyID,
				ReportType: params.ReportType,
			})
			if err != nil {
				return json.Marshal(map[string]string{
					"message": "No saved preferences for " + params.ReportType,
				})
			}

			return json.Marshal(map[string]interface{}{
				"report_type":      prefs.ReportType,
				"column_mappings":  json.RawMessage(prefs.ColumnMappings),
				"format_rules":     json.RawMessage(prefs.FormatRules),
				"auto_fill_rules":  json.RawMessage(prefs.AutoFillRules),
			})
		},
	}
}

// fallbackMessage returns a jurisdiction-specific fallback message when no knowledge is found.
func fallbackMessage(jurisdiction string) string {
	switch jurisdiction {
	case "SG":
		return "No specific regulation found. Please consult the IRAS website (www.iras.gov.sg)."
	case "LK":
		return "No specific regulation found. Please consult the IRD website (www.ird.gov.lk)."
	default:
		return "No specific regulation found. Please consult the BIR website (www.bir.gov.ph)."
	}
}
