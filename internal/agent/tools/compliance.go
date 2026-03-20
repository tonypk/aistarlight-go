package tools

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/agent"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// RegisterCompliance registers compliance-related tools into the ToolRegistry.
func RegisterCompliance(r *agent.ToolRegistry, complianceSvc *service.ComplianceService, reportSvc *service.ReportService) {
	r.Register(runComplianceCheckTool(complianceSvc))
	r.Register(applyFixTool(reportSvc))
}

func runComplianceCheckTool(complianceSvc *service.ComplianceService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "run_compliance_check",
		Description: "Run a compliance check on a report to identify regulatory issues and calculate compliance score",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"report_id": {
					"type": "string",
					"description": "The report UUID to check for compliance"
				}
			},
			"required": ["report_id"]
		}`),
		RiskLevel: agent.RiskLow,
		AgentIDs:  []string{"compliance", "reporting", "*"},
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
				return nil, fmt.Errorf("compliance check failed: %w", err)
			}

			return json.Marshal(validation)
		},
	}
}

func applyFixTool(reportSvc *service.ReportService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "apply_fix",
		Description: "Apply field overrides to a report to fix compliance issues",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"report_id": {
					"type": "string",
					"description": "The report UUID to apply fixes to"
				},
				"overrides": {
					"type": "object",
					"description": "Map of field names to corrected values",
					"additionalProperties": {
						"type": "string"
					}
				}
			},
			"required": ["report_id", "overrides"]
		}`),
		RiskLevel:   agent.RiskHigh,
		AgentIDs:    []string{"compliance"},
		SummaryTmpl: "Apply compliance fix to report {report_id}",
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				ReportID  string            `json:"report_id"`
				Overrides map[string]string `json:"overrides"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.ReportID == "" {
				return nil, fmt.Errorf("report_id is required")
			}
			if len(params.Overrides) == 0 {
				return nil, fmt.Errorf("overrides is required and must not be empty")
			}

			reportID, err := uuid.Parse(params.ReportID)
			if err != nil {
				return nil, fmt.Errorf("invalid report_id: %w", err)
			}

			// Fetch the current report to get its version for optimistic locking
			report, err := reportSvc.GetByID(tc.Ctx, reportID)
			if err != nil {
				return nil, fmt.Errorf("failed to get report: %w", err)
			}
			if report.CompanyID != tc.CompanyID {
				return nil, fmt.Errorf("report not found")
			}

			updated, err := reportSvc.ApplyOverrides(tc.Ctx, service.OverrideInput{
				ReportID:  reportID,
				UserID:    tc.UserID,
				Overrides: params.Overrides,
				Version:   int32(report.Version),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to apply fix: %w", err)
			}

			return json.Marshal(updated)
		},
	}
}
