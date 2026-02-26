package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	oai "github.com/sashabaranov/go-openai"
	openai "github.com/tonypk/aistarlight-go/internal/platform/openai"
)

// MatchAnalyzer uses AI to analyze unmatched entries and suggest potential matches.
type MatchAnalyzer struct {
	ai *openai.Client
}

// NewMatchAnalyzer creates a MatchAnalyzer.
func NewMatchAnalyzer(ai *openai.Client) *MatchAnalyzer {
	return &MatchAnalyzer{ai: ai}
}

// AISuggestion represents an AI-suggested match for an unmatched entry.
type AISuggestion struct {
	RecordID    string  `json:"record_id"`
	BankID      string  `json:"bank_id"`
	Confidence  float64 `json:"confidence"`
	Explanation string  `json:"explanation"`
	MatchType   string  `json:"match_type"` // split, delayed, partial, description_match
}

// AIExplanation explains why an entry remains unmatched.
type AIExplanation struct {
	EntryID     string `json:"entry_id"`
	EntryType   string `json:"entry_type"` // record or bank
	Explanation string `json:"explanation"`
	Category    string `json:"category"` // timing, amount_mismatch, missing_counterpart, duplicate, fee_adjustment
}

// AnalyzeUnmatched sends unmatched entries to AI for fuzzy matching analysis.
func (m *MatchAnalyzer) AnalyzeUnmatched(
	ctx context.Context,
	unmatchedRecords []UnmatchedEntry,
	unmatchedBank []UnmatchedEntry,
) ([]AISuggestion, []AIExplanation, error) {
	if m.ai == nil {
		return nil, nil, nil
	}
	if len(unmatchedRecords) == 0 && len(unmatchedBank) == 0 {
		return nil, nil, nil
	}

	var suggestions []AISuggestion
	var explanations []AIExplanation

	// Process in batches of 10
	batchSize := 10
	recBatches := batchEntries(unmatchedRecords, batchSize)
	bankBatches := batchEntries(unmatchedBank, batchSize)

	for i, recBatch := range recBatches {
		var bankBatch []UnmatchedEntry
		if i < len(bankBatches) {
			bankBatch = bankBatches[i]
		}

		s, e, err := m.analyzeBatch(ctx, recBatch, bankBatch)
		if err != nil {
			continue // skip failed batches
		}
		suggestions = append(suggestions, s...)
		explanations = append(explanations, e...)
	}

	// Handle remaining bank batches
	for i := len(recBatches); i < len(bankBatches); i++ {
		s, e, err := m.analyzeBatch(ctx, nil, bankBatches[i])
		if err != nil {
			continue
		}
		suggestions = append(suggestions, s...)
		explanations = append(explanations, e...)
	}

	return suggestions, explanations, nil
}

func (m *MatchAnalyzer) analyzeBatch(
	ctx context.Context,
	records []UnmatchedEntry,
	bank []UnmatchedEntry,
) ([]AISuggestion, []AIExplanation, error) {
	recJSON, _ := json.Marshal(records)
	bankJSON, _ := json.Marshal(bank)

	prompt := fmt.Sprintf(`You are analyzing Philippine business financial entries for reconciliation.

CONTEXT:
- These are from a Philippine company doing BIR tax compliance
- Common Philippine banks: BDO, BPI, Metrobank, Landbank, GCash, Maya
- VAT rate is 12%% — amounts may differ by exactly 12%% (net vs gross)
- Withholding tax (EWT) commonly 1%%, 2%%, or 5%% — deducted from payments
- Bank fees, DST (documentary stamp tax), and penalties are common deductions
- Reference formats: OR (Official Receipt), SI (Sales Invoice), CR (Collection Receipt)
- ATC codes (e.g., WC010, WI010) indicate withholding tax type

UNMATCHED RECORDS (from accounting system):
%s

UNMATCHED BANK ENTRIES:
%s

Analyze each entry and:
1. Find potential matches considering:
   - Split transactions (one payment covering multiple invoices)
   - Delayed transactions (timing differences, common in PH banking: 1-5 days)
   - Net-of-tax payments (amount minus 1-5%% withholding tax)
   - Bank fee adjustments (amount minus bank charges, typically ₱25-500)
   - Description-based matches (same supplier/customer, different amounts)
   - Reference number matches (check no, OR no, invoice no in description)

2. For unmatched entries, explain likely reasons:
   - Bank fees/charges (DST, service charge, penalty)
   - Government payments (BIR, SSS, PhilHealth, Pag-IBIG)
   - Inter-company transfers
   - Timing differences (month-end cutoff)
   - Missing invoice/receipt

3. Suggest VAT classification when possible:
   - "vatable" (standard 12%% VAT)
   - "zero_rated" (export sales, BOI-registered)
   - "exempt" (basic commodities, educational, health)

Respond ONLY in valid JSON:
{
  "suggestions": [
    {"record_id": "...", "bank_id": "...", "confidence": 0.0-1.0, "explanation": "...", "match_type": "split|delayed|partial|description_match|net_of_tax", "next_action": "description of what CPA should verify"}
  ],
  "explanations": [
    {"entry_id": "...", "entry_type": "record|bank", "explanation": "...", "category": "timing|amount_mismatch|missing_counterpart|duplicate|fee_adjustment|bank_charge|government_payment|withholding_tax", "suggested_vat_type": "vatable|zero_rated|exempt|null"}
  ]
}`, string(recJSON), string(bankJSON))

	messages := []oai.ChatCompletionMessage{
		{
			Role:    oai.ChatMessageRoleSystem,
			Content: "You are a Philippine CPA specializing in BIR tax compliance and bank reconciliation. You understand EWT (expanded withholding tax), VAT classifications, ATC codes, and common Philippine banking patterns. Analyze unmatched entries and suggest potential matches or explain discrepancies. Always respond in valid JSON.",
		},
		{
			Role:    oai.ChatMessageRoleUser,
			Content: prompt,
		},
	}

	resp, err := m.ai.ChatCompletion(ctx, messages, openai.WithTemperature(0.1), openai.WithMaxTokens(2000))
	if err != nil {
		return nil, nil, fmt.Errorf("AI analysis failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, nil, fmt.Errorf("empty AI response")
	}

	content := resp.Choices[0].Message.Content
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result struct {
		Suggestions  []AISuggestion  `json:"suggestions"`
		Explanations []AIExplanation `json:"explanations"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	return result.Suggestions, result.Explanations, nil
}

func batchEntries(entries []UnmatchedEntry, size int) [][]UnmatchedEntry {
	var batches [][]UnmatchedEntry
	for i := 0; i < len(entries); i += size {
		end := i + size
		if end > len(entries) {
			end = len(entries)
		}
		batches = append(batches, entries[i:end])
	}
	return batches
}
