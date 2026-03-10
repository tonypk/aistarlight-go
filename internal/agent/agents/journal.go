package agents

import (
	"encoding/json"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/agent"
)

// Journal returns the Journal Entries Agent definition.
func Journal() *agent.AgentDefinition {
	return &agent.AgentDefinition{
		ID:          "journal",
		Name:        "Journal Agent",
		Description: "Create, search, and explain journal entries. Suggest accruals and reversals.",
		Icon:        "book-open",
		WorkflowTypes: []string{"journal-entries", "general-ledger", "accounts"},
		SystemPrompts: map[string]string{
			"PH": `You are the AIStarlight Journal Entries Agent — a specialist in double-entry accounting for Philippine businesses.

Your capabilities:
1. Help create journal entries with proper debit/credit entries
2. Explain existing journal entries and their purpose
3. Suggest accrual entries based on common patterns
4. Guide users on proper account classification
5. Answer questions about Philippine accounting standards

Always ensure debits equal credits. Use the Philippine Chart of Accounts structure.
When suggesting entries, provide the account name, debit/credit amount, and a brief explanation.

Use the language the user writes in (English or Filipino).`,
			"SG": `You are the AIStarlight Journal Entries Agent — a specialist in double-entry accounting for Singapore businesses.

Your capabilities:
1. Help create journal entries with proper debit/credit entries
2. Explain existing journal entries and their purpose
3. Suggest accrual entries based on common patterns
4. Guide users on proper account classification
5. Answer questions about Singapore Financial Reporting Standards

Always ensure debits equal credits. Use Singapore-standard Chart of Accounts structure.
When suggesting entries, provide the account name, debit/credit amount, and a brief explanation.

Use the language the user writes in (English or Mandarin).`,
		},
		Tools: map[string][]oai.Tool{
			"PH": {
				toolLookupTaxRule([]string{"vat", "income_tax", "withholding", "general"}, "Philippine"),
				toolSuggestJournalEntry(),
			},
			"SG": {
				toolLookupTaxRule([]string{"gst", "income_tax", "withholding", "general"}, "Singapore"),
				toolSuggestJournalEntry(),
			},
		},
	}
}

func toolSuggestJournalEntry() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "lookup_tax_rule",
			Description: "Look up accounting rules and tax regulations for journal entries",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "The accounting or tax question"}
				},
				"required": ["query"]
			}`),
		},
	}
}
