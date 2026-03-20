package tools

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/agent"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// RegisterReporting registers reporting-related tools into the ToolRegistry.
func RegisterReporting(r *agent.ToolRegistry, reportSvc *service.ReportService, complianceSvc *service.ComplianceService) {
	r.Register(listReportsTool(reportSvc))
	r.Register(getReportDetailTool(reportSvc))
	r.Register(generateReportTool(reportSvc))
	r.Register(validateReportTool(complianceSvc))
}

func listReportsTool(reportSvc *service.ReportService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "list_reports",
		Description: "List tax reports for the company with pagination",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"limit": {
					"type": "integer",
					"description": "Maximum number of reports to return (default 20, max 50)"
				},
				"offset": {
					"type": "integer",
					"description": "Number of reports to skip (default 0)"
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
			reports, total, err := reportSvc.ListByCompany(tc.Ctx, tc.CompanyID, p)
			if err != nil {
				return nil, fmt.Errorf("failed to list reports: %w", err)
			}

			return json.Marshal(map[string]interface{}{
				"reports": reports,
				"total":   total,
			})
		},
	}
}

func getReportDetailTool(reportSvc *service.ReportService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "get_report_detail",
		Description: "Get detailed information for a specific tax report",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"report_id": {
					"type": "string",
					"description": "The report UUID"
				}
			},
			"required": ["report_id"]
		}`),
		RiskLevel: agent.RiskLow,
		AgentIDs:  []string{"*"},
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				ReportID string `json:"report_id"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.ReportID == "" {
				return nil, fmt.Errorf("report_id is required")
			}

			reportID, err := uuid.Parse(params.ReportID)
			if err != nil {
				return nil, fmt.Errorf("invalid report_id: %w", err)
			}

			report, err := reportSvc.GetByID(tc.Ctx, reportID)
			if err != nil {
				return nil, fmt.Errorf("failed to get report: %w", err)
			}
			if report.CompanyID != tc.CompanyID {
				return nil, fmt.Errorf("report not found")
			}

			return json.Marshal(report)
		},
	}
}

func generateReportTool(reportSvc *service.ReportService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "generate_report",
		Description: "Generate a tax report from a reconciliation session",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_id": {
					"type": "string",
					"description": "The session UUID to generate the report from"
				},
				"report_type": {
					"type": "string",
					"description": "The type of report to generate (e.g. 2550M, 2550Q, 1601C)"
				}
			},
			"required": ["session_id", "report_type"]
		}`),
		RiskLevel:   agent.RiskHigh,
		AgentIDs:    []string{"reporting"},
		SummaryTmpl: "Generate {report_type} report from session {session_id}",
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				SessionID  string `json:"session_id"`
				ReportType string `json:"report_type"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.SessionID == "" {
				return nil, fmt.Errorf("session_id is required")
			}
			if params.ReportType == "" {
				return nil, fmt.Errorf("report_type is required")
			}

			sessionID, err := uuid.Parse(params.SessionID)
			if err != nil {
				return nil, fmt.Errorf("invalid session_id: %w", err)
			}

			report, err := reportSvc.GenerateFromSession(tc.Ctx, sessionID, tc.CompanyID, tc.UserID, params.ReportType)
			if err != nil {
				return nil, fmt.Errorf("failed to generate report: %w", err)
			}

			return json.Marshal(report)
		},
	}
}

func validateReportTool(complianceSvc *service.ComplianceService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "validate_report",
		Description: "Validate a report against tax compliance rules and return findings",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"report_id": {
					"type": "string",
					"description": "The report UUID to validate"
				}
			},
			"required": ["report_id"]
		}`),
		RiskLevel: agent.RiskLow,
		AgentIDs:  []string{"reporting", "compliance", "*"},
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				ReportID string `json:"report_id"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.ReportID == "" {
				return nil, fmt.Errorf("report_id is required")
			}

			reportID, err := uuid.Parse(params.ReportID)
			if err != nil {
				return nil, fmt.Errorf("invalid report_id: %w", err)
			}

			validation, err := complianceSvc.ValidateReport(tc.Ctx, reportID, tc.CompanyID)
			if err != nil {
				return nil, fmt.Errorf("validation failed: %w", err)
			}

			return json.Marshal(validation)
		},
	}
}
