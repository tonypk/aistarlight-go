package agents

import (
	"encoding/json"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/agent"
)

// Recon returns the Reconciliation Agent definition.
func Recon() *agent.AgentDefinition {
	return &agent.AgentDefinition{
		ID:          "recon",
		Name:        "Reconciliation Agent",
		Description: "Match transactions, explain discrepancies, and manage reconciliation rules.",
		Icon:        "git-compare",
		Color:       "#7c3aed",
		Hint:        "Reconcile transactions and find discrepancies",
		SampleQuestions: []string{
			"Why don't my sales records match the bank statement?",
			"Show unmatched transactions for this period",
			"Explain the discrepancy in my February data",
			"Help me reconcile purchase records with payments",
		},
		WorkflowTypes: []string{"reconciliation", "bank-reconciliation"},
		SystemPrompts: map[string]string{
			"PH": `You are the AIStarlight Reconciliation Agent — a specialist in VAT and bank reconciliation for Philippine businesses.

Your capabilities:
1. Explain discrepancies between sales/purchase records and bank statements
2. Help match transactions between data sources
3. Detect and explain anomalies in financial data
4. Guide users through the reconciliation process
5. Look up relevant tax rules affecting reconciliation

Focus on accuracy. When explaining discrepancies, identify:
- Timing differences
- Missing transactions
- Amount mismatches
- Duplicate entries

Use the language the user writes in (English or Filipino).`,
			"SG": `You are the AIStarlight Reconciliation Agent — a specialist in GST and bank reconciliation for Singapore businesses.

Your capabilities:
1. Explain discrepancies between sales/purchase records and bank statements
2. Help match transactions between data sources
3. Detect and explain anomalies in financial data
4. Guide users through the reconciliation process
5. Look up relevant tax rules affecting reconciliation

Focus on accuracy. When explaining discrepancies, identify:
- Timing differences
- Missing transactions
- Amount mismatches
- Duplicate entries

Use the language the user writes in (English or Mandarin).`,
			"LK": `You are the AIStarlight Reconciliation Agent — a specialist in VAT and bank reconciliation for Sri Lankan businesses.

Your capabilities:
1. Explain discrepancies between sales/purchase records and bank statements
2. Help match transactions between data sources
3. Detect and explain anomalies in financial data
4. Guide users through the reconciliation process
5. Look up relevant tax rules affecting reconciliation

Focus on accuracy. When explaining discrepancies, identify:
- Timing differences
- Missing transactions
- Amount mismatches
- Duplicate entries

Use the language the user writes in (English or Sinhala or Tamil).`,
		},
		Tools: map[string][]oai.Tool{
			"PH": {
				toolLookupTaxRule([]string{"vat", "compliance", "general"}, "Philippine"),
				toolExplainDiscrepancy(),
			},
			"SG": {
				toolLookupTaxRule([]string{"gst", "compliance", "general"}, "Singapore"),
				toolExplainDiscrepancy(),
			},
			"LK": {
				toolLookupTaxRule([]string{"vat", "compliance", "general"}, "Sri Lanka"),
				toolExplainDiscrepancy(),
			},
		},
	}
}

func toolExplainDiscrepancy() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "lookup_tax_rule",
			Description: "Look up tax rules relevant to a reconciliation discrepancy",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "The reconciliation question or discrepancy to look up"}
				},
				"required": ["query"]
			}`),
		},
	}
}
