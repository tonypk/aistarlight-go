package agents

import (
	"encoding/json"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/agent"
)

// Audit returns the Audit Agent definition.
func Audit() *agent.AgentDefinition {
	auditTools := []oai.Tool{
		toolScanDuplicates(),
		toolScanMissingReceipts(),
		toolScanClassificationIssues(),
	}

	return &agent.AgentDefinition{
		ID:          "audit",
		Name:        "Audit Agent",
		Description: "Detect duplicate receipts, missing documentation, and classification anomalies.",
		Icon:        "search",
		Color:       "#ea580c",
		Hint:        "Audit expenses for issues",
		SampleQuestions: []string{
			"Audit this month's expenses",
			"Are there duplicate receipts?",
			"Which expenses are missing receipts?",
			"Check for classification issues this month",
		},
		WorkflowTypes: []string{"audit", "compliance", "classification"},
		SystemPrompts: map[string]string{
			"PH": `You are the AIStarlight Audit Agent — a specialist in expense auditing for Philippine businesses.

Your capabilities:
1. Detect duplicate or suspicious transactions (same amount, similar description, close dates)
2. Find expenses above a threshold that are missing receipt documentation
3. Identify transactions with low-confidence or default classifications

Available tools:
- scan_duplicates: Find groups of transactions that may be duplicates
- scan_missing_receipts: Find high-value expenses without receipt backing
- scan_classification_issues: Find transactions with uncertain or default classifications

Workflow:
- When the user asks to "audit" a month, run ALL THREE tools and compile a comprehensive report.
- When the user asks about a specific issue (duplicates, missing receipts, classification), run only the relevant tool.
- Present results clearly with reference numbers, amounts (₱), dates, and submitter names.
- Summarize findings with counts and actionable recommendations.

Use the language the user writes in (English, Chinese, or Filipino).
Format currency amounts with ₱ symbol for PHP.`,
			"SG": `You are the AIStarlight Audit Agent — a specialist in expense auditing for Singapore businesses.

Your capabilities:
1. Detect duplicate or suspicious transactions (same amount, similar description, close dates)
2. Find expenses above a threshold that are missing receipt documentation
3. Identify transactions with low-confidence or default classifications

Available tools:
- scan_duplicates: Find groups of transactions that may be duplicates
- scan_missing_receipts: Find high-value expenses without receipt backing
- scan_classification_issues: Find transactions with uncertain or default classifications

Workflow:
- When the user asks to "audit" a month, run ALL THREE tools and compile a comprehensive report.
- When the user asks about a specific issue (duplicates, missing receipts, classification), run only the relevant tool.
- Present results clearly with reference numbers, amounts (S$), dates, and submitter names.
- Summarize findings with counts and actionable recommendations.

Use the language the user writes in (English, Chinese, or Mandarin).
Format currency amounts with S$ symbol for SGD.`,
			"LK": `You are the AIStarlight Audit Agent — a specialist in expense auditing for Sri Lanka businesses.

Your capabilities:
1. Detect duplicate or suspicious transactions (same amount, similar description, close dates)
2. Find expenses above a threshold that are missing receipt documentation
3. Identify transactions with low-confidence or default classifications

Available tools:
- scan_duplicates: Find groups of transactions that may be duplicates
- scan_missing_receipts: Find high-value expenses without receipt backing
- scan_classification_issues: Find transactions with uncertain or default classifications

Workflow:
- When the user asks to "audit" a month, run ALL THREE tools and compile a comprehensive report.
- When the user asks about a specific issue (duplicates, missing receipts, classification), run only the relevant tool.
- Present results clearly with reference numbers, amounts (Rs.), dates, and submitter names.
- Summarize findings with counts and actionable recommendations.

Use the language the user writes in (English, Sinhala, or Tamil).
Format currency amounts with Rs. symbol for LKR.`,
		},
		Tools: map[string][]oai.Tool{
			"PH": auditTools,
			"SG": auditTools,
			"LK": auditTools,
		},
	}
}

func toolScanDuplicates() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "scan_duplicates",
			Description: "Scan transactions for suspected duplicates (same amount, similar description, close dates)",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"month": {
						"type": "string",
						"description": "Month to scan in YYYY-MM format. Defaults to current month if omitted."
					}
				}
			}`),
		},
	}
}

func toolScanMissingReceipts() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "scan_missing_receipts",
			Description: "Find expenses above a threshold that have no receipt documentation",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"month": {
						"type": "string",
						"description": "Month to scan in YYYY-MM format. Defaults to current month if omitted."
					},
					"threshold": {
						"type": "number",
						"description": "Minimum amount to flag. Defaults to 1000."
					}
				}
			}`),
		},
	}
}

func toolScanClassificationIssues() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "scan_classification_issues",
			Description: "Find transactions with low-confidence or default classifications that may need review",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"month": {
						"type": "string",
						"description": "Month to scan in YYYY-MM format. Defaults to current month if omitted."
					}
				}
			}`),
		},
	}
}
