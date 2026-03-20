package tools

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/agent"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// RegisterReconciliation registers reconciliation-related tools into the ToolRegistry.
func RegisterReconciliation(r *agent.ToolRegistry, sessionSvc *service.SessionService) {
	r.Register(listAnomaliesTool(sessionSvc))
	r.Register(resolveAnomalyTool(sessionSvc))
	r.Register(runReconciliationTool(sessionSvc))
}

func listAnomaliesTool(sessionSvc *service.SessionService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "list_anomalies",
		Description: "List anomalies detected in a reconciliation session",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_id": {
					"type": "string",
					"description": "The session UUID"
				},
				"limit": {
					"type": "integer",
					"description": "Maximum number of anomalies to return (default 20, max 50)"
				},
				"status": {
					"type": "string",
					"description": "Filter anomalies by status (e.g. pending, resolved, rejected)"
				}
			},
			"required": ["session_id"]
		}`),
		RiskLevel: agent.RiskLow,
		AgentIDs:  []string{"reconciliation", "audit", "*"},
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				SessionID string `json:"session_id"`
				Limit     int    `json:"limit"`
				Status    string `json:"status"`
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
				limit = 20
			} else if limit > 50 {
				limit = 50
			}

			var statusPtr *string
			if params.Status != "" {
				statusPtr = &params.Status
			}

			anomalies, total, err := sessionSvc.ListAnomalies(tc.Ctx, sessionID, tc.CompanyID, limit, 0, statusPtr)
			if err != nil {
				return nil, fmt.Errorf("failed to list anomalies: %w", err)
			}

			return json.Marshal(map[string]interface{}{
				"anomalies": anomalies,
				"total":     total,
			})
		},
	}
}

func resolveAnomalyTool(sessionSvc *service.SessionService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "resolve_anomaly",
		Description: "Resolve or reject an anomaly in a reconciliation session",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"anomaly_id": {
					"type": "string",
					"description": "The anomaly UUID to resolve"
				},
				"session_id": {
					"type": "string",
					"description": "The session UUID"
				},
				"status": {
					"type": "string",
					"enum": ["resolved", "rejected"],
					"description": "Resolution status"
				},
				"note": {
					"type": "string",
					"description": "Optional resolution note"
				}
			},
			"required": ["anomaly_id", "session_id", "status"]
		}`),
		RiskLevel:   agent.RiskHigh,
		AgentIDs:    []string{"reconciliation"},
		SummaryTmpl: "Resolve anomaly {anomaly_id} as {status}",
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				AnomalyID string `json:"anomaly_id"`
				SessionID string `json:"session_id"`
				Status    string `json:"status"`
				Note      string `json:"note"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.AnomalyID == "" {
				return nil, fmt.Errorf("anomaly_id is required")
			}
			if params.SessionID == "" {
				return nil, fmt.Errorf("session_id is required")
			}
			if params.Status != "resolved" && params.Status != "rejected" {
				return nil, fmt.Errorf("status must be 'resolved' or 'rejected'")
			}

			anomalyID, err := uuid.Parse(params.AnomalyID)
			if err != nil {
				return nil, fmt.Errorf("invalid anomaly_id: %w", err)
			}

			sessionID, err := uuid.Parse(params.SessionID)
			if err != nil {
				return nil, fmt.Errorf("invalid session_id: %w", err)
			}

			var notePtr *string
			if params.Note != "" {
				notePtr = &params.Note
			}

			anomaly, err := sessionSvc.ResolveAnomaly(tc.Ctx, anomalyID, sessionID, tc.CompanyID, tc.UserID, params.Status, notePtr)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve anomaly: %w", err)
			}

			return json.Marshal(anomaly)
		},
	}
}

func runReconciliationTool(sessionSvc *service.SessionService) *agent.ToolDef {
	return &agent.ToolDef{
		Name:        "run_reconciliation",
		Description: "Run the reconciliation pipeline on a session to match and validate transactions",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_id": {
					"type": "string",
					"description": "The session UUID to reconcile"
				},
				"report_id": {
					"type": "string",
					"description": "Optional report UUID to reconcile against"
				},
				"amount_tolerance": {
					"type": "number",
					"description": "Amount matching tolerance (default 0.01)"
				},
				"date_tolerance_days": {
					"type": "integer",
					"description": "Date matching tolerance in days (default 3)"
				}
			},
			"required": ["session_id"]
		}`),
		RiskLevel:   agent.RiskHigh,
		AgentIDs:    []string{"reconciliation"},
		SummaryTmpl: "Run reconciliation for session {session_id}",
		Execute: func(tc agent.ToolContext, args json.RawMessage) (json.RawMessage, error) {
			var params struct {
				SessionID         string  `json:"session_id"`
				ReportID          string  `json:"report_id"`
				AmountTolerance   float64 `json:"amount_tolerance"`
				DateToleranceDays int     `json:"date_tolerance_days"`
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

			var reportIDPtr *uuid.UUID
			if params.ReportID != "" {
				reportID, err := uuid.Parse(params.ReportID)
				if err != nil {
					return nil, fmt.Errorf("invalid report_id: %w", err)
				}
				reportIDPtr = &reportID
			}

			amountTol := params.AmountTolerance
			if amountTol <= 0 {
				amountTol = 0.01
			}

			dateTolDays := params.DateToleranceDays
			if dateTolDays <= 0 {
				dateTolDays = 3
			}

			result, err := sessionSvc.ReconcileSession(tc.Ctx, sessionID, tc.CompanyID, reportIDPtr, amountTol, dateTolDays)
			if err != nil {
				return nil, fmt.Errorf("reconciliation failed: %w", err)
			}

			return json.Marshal(result)
		},
	}
}
