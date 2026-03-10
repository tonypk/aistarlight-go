package agents

import (
	"encoding/json"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/agent"
)

// General returns the General Tax Assistant agent definition.
func General() *agent.AgentDefinition {
	return &agent.AgentDefinition{
		ID:          "general",
		Name:        "Tax Assistant",
		Description: "General AI assistant for tax questions, report generation, and preferences",
		Icon:        "chat",
		WorkflowTypes: []string{"*"},
		SystemPrompts: map[string]string{
			"PH": `AIStarlight - AI-powered Philippine tax filing assistant for SMEs.

Your capabilities:
1. Process uploaded financial data (sales/purchase records, bank statements, receipts)
2. Calculate VAT, withholding tax, generate BIR reports
3. AI-powered transaction classification and column mapping
4. Bank & billing auto-reconciliation (CSV/Excel/PDF/image)
5. Receipt OCR scanning and data extraction
6. EWT classification, BIR 2307 certificate generation, SAWT
7. Compliance validation and anomaly detection
8. Remember user preferences for recurring filings
9. Answer questions about Philippine tax regulations (289 knowledge entries)

Supported forms: BIR_2550M, BIR_2550Q, BIR_1601C, BIR_0619E, BIR_2307, SAWT
(BIR 1701, 1702, 2316 coming soon)

Tool routing:
- User asks to generate report → use generate_report tool
- User asks about tax rules → use lookup_tax_rule tool
- User asks about settings/preferences → use get_user_preferences tool

Use language user writes in (English or Filipino).`,
			"SG": `AIStarlight - AI-powered Singapore tax filing assistant for SMEs.

Your capabilities:
1. Process uploaded financial data (sales/purchase records, bank statements, receipts)
2. Calculate GST, corporate/individual income tax, generate IRAS reports
3. AI-powered transaction classification and column mapping
4. Bank & billing auto-reconciliation (CSV/Excel/PDF/image)
5. Receipt OCR scanning and data extraction
6. S45 withholding tax on non-resident payments
7. Compliance validation and anomaly detection
8. Remember user preferences for recurring filings
9. Answer questions about Singapore tax regulations

Supported forms: IRAS_GST_F5, IRAS_FORM_C, IRAS_FORM_CS, IRAS_FORM_B, IRAS_IR8A, IRAS_S45

Tool routing:
- User asks to generate report → use generate_report tool
- User asks about tax rules → use lookup_tax_rule tool
- User asks about settings/preferences → use get_user_preferences tool

Use language user writes in (English or Mandarin).`,
		},
		Tools: map[string][]oai.Tool{
			"PH": {
				toolGenerateReport([]string{"BIR_2550M", "BIR_2550Q", "BIR_1601C", "BIR_0619E"}),
				toolLookupTaxRule([]string{"vat", "income_tax", "withholding", "compliance", "general", "payroll", "incentives"}, "Philippine"),
				toolGetPreferences(),
			},
			"SG": {
				toolGenerateReport([]string{"IRAS_GST_F5", "IRAS_FORM_C", "IRAS_FORM_CS", "IRAS_FORM_B", "IRAS_IR8A", "IRAS_S45"}),
				toolLookupTaxRule([]string{"gst", "income_tax", "withholding", "compliance", "general", "payroll", "corporate"}, "Singapore"),
				toolGetPreferences(),
			},
		},
	}
}

func toolGenerateReport(reportTypes []string) oai.Tool {
	enumJSON, _ := json.Marshal(reportTypes)
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "generate_report",
			Description: "Generate a tax report for a specific period",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"report_type": {"type": "string", "enum": ` + string(enumJSON) + `},
					"period": {"type": "string", "description": "YYYY-MM format"}
				},
				"required": ["report_type", "period"]
			}`),
		},
	}
}

func toolLookupTaxRule(categories []string, jurisdiction string) oai.Tool {
	catJSON, _ := json.Marshal(categories)
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "lookup_tax_rule",
			Description: "Look up a " + jurisdiction + " tax regulation or rule",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "The tax rule query"},
					"category": {"type": "string", "enum": ` + string(catJSON) + `}
				},
				"required": ["query"]
			}`),
		},
	}
}

func toolGetPreferences() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        "get_user_preferences",
			Description: "Retrieve saved user preferences for a report type",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"report_type": {"type": "string"}
				},
				"required": ["report_type"]
			}`),
		},
	}
}
