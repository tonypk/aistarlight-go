package agents

import (
	"encoding/json"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/agent"
)

// Filing returns the Filing Agent definition for tax report generation.
func Filing() *agent.AgentDefinition {
	return &agent.AgentDefinition{
		ID:          "filing",
		Name:        "Filing Agent",
		Description: "Generate, calculate, and validate tax reports. Explain line items and suggest corrections.",
		Icon:        "file-text",
		Color:       "#0891b2",
		Hint:        "Generate and validate tax reports",
		SampleQuestions: []string{
			"Generate my BIR 2550M for this month",
			"What does Line 20 on the 2550Q mean?",
			"Validate my latest filing for errors",
			"Compare this month's VAT with last month",
		},
		WorkflowTypes: []string{"reports", "form-router", "statements", "tax-bridge"},
		SystemPrompts: map[string]string{
			"PH": `You are the AIStarlight Filing Agent — a specialist in Philippine BIR tax report generation and filing.

Your capabilities:
1. Generate BIR tax reports (2550M, 2550Q, 1601C, 0619E, 2307, SAWT)
2. Calculate tax amounts from uploaded financial data
3. Validate reports for compliance issues
4. Explain specific line items and calculations
5. Look up relevant tax regulations

When the user asks about a report, always check if one exists first.
If data is needed, guide the user to upload it.
Be precise with amounts — use Philippine Peso (PHP) formatting.

Use the language the user writes in (English or Filipino).`,
			"SG": `You are the AIStarlight Filing Agent — a specialist in Singapore IRAS tax report generation and filing.

Your capabilities:
1. Generate IRAS tax reports (GST F5, Form C/CS, Form B, IR8A, S45)
2. Calculate tax amounts from uploaded financial data
3. Validate reports for compliance issues
4. Explain specific line items and calculations
5. Look up relevant tax regulations

When the user asks about a report, always check if one exists first.
If data is needed, guide the user to upload it.
Be precise with amounts — use Singapore Dollar (SGD) formatting.

Use the language the user writes in (English or Mandarin).`,
		},
		Tools: map[string][]oai.Tool{
			"PH": {
				toolGenerateReport([]string{"BIR_2550M", "BIR_2550Q", "BIR_1601C", "BIR_0619E"}),
				toolLookupTaxRule([]string{"vat", "income_tax", "withholding", "compliance", "general"}, "Philippine"),
				toolValidateReport(),
				toolGetPreferences(),
			},
			"SG": {
				toolGenerateReport([]string{"IRAS_GST_F5", "IRAS_FORM_C", "IRAS_FORM_CS", "IRAS_FORM_B", "IRAS_IR8A", "IRAS_S45"}),
				toolLookupTaxRule([]string{"gst", "income_tax", "withholding", "compliance", "general"}, "Singapore"),
				toolValidateReport(),
				toolGetPreferences(),
			},
		},
	}
}

func toolValidateReport() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "validate_report",
			Description: "Run compliance validation on a tax report",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"report_id": {"type": "string", "description": "UUID of the report to validate"}
				},
				"required": ["report_id"]
			}`),
		},
	}
}
