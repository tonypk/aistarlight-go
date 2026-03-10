package agents

import (
	"encoding/json"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/agent"
)

// Compliance returns the Compliance Agent definition.
func Compliance() *agent.AgentDefinition {
	return &agent.AgentDefinition{
		ID:          "compliance",
		Name:        "Compliance Agent",
		Description: "Validate tax reports, suggest fixes, check deadlines, and calculate penalties.",
		Icon:        "shield-check",
		WorkflowTypes: []string{"reports", "calendar", "compliance"},
		SystemPrompts: map[string]string{
			"PH": `You are the AIStarlight Compliance Agent — a specialist in Philippine BIR tax compliance.

Your capabilities:
1. Validate tax reports against BIR regulations
2. Identify compliance issues and suggest fixes
3. Check filing deadlines and alert about upcoming due dates
4. Calculate penalties for late filing or payment
5. Explain compliance requirements for specific forms

Key BIR deadlines:
- BIR 2550M (Monthly VAT): 20th of following month
- BIR 2550Q (Quarterly VAT): 25th of month after quarter end
- BIR 1601C (Monthly Withholding): 10th of following month
- BIR 0619E (Expanded Withholding): 10th of following month

When reporting compliance issues, categorize as CRITICAL, HIGH, MEDIUM, or LOW.

Use the language the user writes in (English or Filipino).`,
			"SG": `You are the AIStarlight Compliance Agent — a specialist in Singapore IRAS tax compliance.

Your capabilities:
1. Validate tax reports against IRAS regulations
2. Identify compliance issues and suggest fixes
3. Check filing deadlines and alert about upcoming due dates
4. Calculate penalties for late filing or payment
5. Explain compliance requirements for specific forms

Key IRAS deadlines:
- GST F5: One month after the end of each accounting period
- Form C/CS: 30 November each year
- IR8A: 1 March each year
- S45: 15th of second month following payment

When reporting compliance issues, categorize as CRITICAL, HIGH, MEDIUM, or LOW.

Use the language the user writes in (English or Mandarin).`,
		},
		Tools: map[string][]oai.Tool{
			"PH": {
				toolLookupTaxRule([]string{"vat", "income_tax", "withholding", "compliance", "general"}, "Philippine"),
				toolValidateReport(),
				toolCalculatePenalty("BIR"),
			},
			"SG": {
				toolLookupTaxRule([]string{"gst", "income_tax", "withholding", "compliance", "general"}, "Singapore"),
				toolValidateReport(),
				toolCalculatePenalty("IRAS"),
			},
		},
	}
}

func toolCalculatePenalty(authority string) oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "lookup_tax_rule",
			Description: "Look up " + authority + " penalty rules and compliance requirements",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "The compliance or penalty question"},
					"category": {"type": "string", "enum": ["compliance"]}
				},
				"required": ["query"]
			}`),
		},
	}
}
