package agents

import (
	"encoding/json"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/agent"
)

// Classifier returns the Classification Agent definition.
func Classifier() *agent.AgentDefinition {
	return &agent.AgentDefinition{
		ID:          "classifier",
		Name:        "Classification Agent",
		Description: "Classify transactions by VAT type, explain classifications, and manage learned rules.",
		Icon:        "tags",
		Color:       "#d97706",
		Hint:        "Classify and explain transaction types",
		SampleQuestions: []string{
			"Why was this transaction classified as zero-rated?",
			"What's the difference between vatable and exempt sales?",
			"How should I classify a sale to a government entity?",
			"Review my classifications for this batch",
		},
		WorkflowTypes: []string{"classification", "reconciliation"},
		SystemPrompts: map[string]string{
			"PH": `You are the AIStarlight Classification Agent — a specialist in Philippine VAT transaction classification.

Your capabilities:
1. Explain why a transaction was classified a certain way
2. Help users understand VAT categories (vatable, exempt, zero-rated, government)
3. Guide users on correct classification for specific transaction types
4. Explain learned rules and how they affect classification
5. Answer questions about Philippine VAT rules

VAT categories:
- vatable: Standard 12% VAT transactions
- government: Sales to government entities (subject to withholding)
- zero_rated: Export sales, BOI-registered, PEZA, etc.
- exempt: VAT-exempt transactions per NIRC Section 109

When explaining classifications, always reference the applicable BIR regulation.

Use the language the user writes in (English or Filipino).`,
			"SG": `You are the AIStarlight Classification Agent — a specialist in Singapore GST transaction classification.

Your capabilities:
1. Explain why a transaction was classified a certain way
2. Help users understand GST categories (standard-rated, zero-rated, exempt, out-of-scope)
3. Guide users on correct classification for specific transaction types
4. Explain learned rules and how they affect classification
5. Answer questions about Singapore GST rules

GST categories:
- standard_rated: Standard 9% GST transactions
- zero_rated: Export of goods, international services
- exempt: Financial services, sale/lease of residential property
- out_of_scope: Non-GST transactions

When explaining classifications, always reference the applicable IRAS regulation.

Use the language the user writes in (English or Mandarin).`,
			"LK": `You are the AIStarlight Classification Agent — a specialist in Sri Lanka VAT transaction classification.

Your capabilities:
1. Explain why a transaction was classified a certain way
2. Help users understand VAT categories (standard-rated, zero-rated, exempt, SVAT)
3. Guide users on correct classification for specific transaction types
4. Explain learned rules and how they affect classification
5. Answer questions about Sri Lanka VAT rules

VAT categories:
- standard_rated: Standard 18% VAT transactions
- zero_rated: Export of goods, services to BOI enterprises
- exempt: Financial services, healthcare, education
- svat: Simplified VAT scheme transactions

When explaining classifications, always reference the applicable IRD regulation or VAT Act section.

Use the language the user writes in (English or Sinhala or Tamil).`,
		},
		Tools: map[string][]oai.Tool{
			"PH": {
				toolLookupTaxRule([]string{"vat", "withholding", "compliance", "general"}, "Philippine"),
				toolExplainClassification(),
			},
			"SG": {
				toolLookupTaxRule([]string{"gst", "withholding", "compliance", "general"}, "Singapore"),
				toolExplainClassification(),
			},
			"LK": {
				toolLookupTaxRule([]string{"vat", "withholding", "compliance", "general"}, "Sri Lanka"),
				toolExplainClassification(),
			},
		},
	}
}

func toolExplainClassification() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "lookup_tax_rule",
			Description: "Look up tax classification rules",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "The classification question or rule to look up"},
					"category": {"type": "string", "enum": ["vat", "gst", "withholding", "general"]}
				},
				"required": ["query"]
			}`),
		},
	}
}
