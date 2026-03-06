package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

const classificationBatchSize = 25

var govtTINPrefixes = []string{"000", "001", "002"}

var validVATTypes = map[string]bool{
	"vatable": true, "exempt": true, "zero_rated": true, "government": true,
}

var validCategories = map[string]bool{
	"goods": true, "services": true, "capital": true, "imports": true, "sale": true,
}

const classificationSystemPrompt = `Expert Philippine tax accountant. Classify each transaction for BIR VAT filing.

For each transaction, determine:
1. vat_type: "vatable" | "exempt" | "zero_rated" | "government"
2. category: "goods" | "services" | "capital" | "imports" | "sale"
3. confidence: 0.00-1.00

IMPORTANT: Each transaction has a "source_type" field:
- "sales_record": This is a SALES transaction. Use category="sale" for domestic/export/government sales.
- "purchase_record": This is a PURCHASE transaction. Use category="goods", "services", "capital", or "imports". NEVER use category="sale" for purchases.

Rules:
- Sales to government: vat_type="government", category="sale" (only if source_type="sales_record")
- Export sales: vat_type="zero_rated", category="sale" (only if source_type="sales_record")
- Domestic sales: vat_type="vatable", category="sale" (only if source_type="sales_record")
- Purchases from government: vat_type="government", category="goods" or "services"
- Equipment/machinery > 1M PHP: category="capital"
- Imported goods: category="imports"
- Agricultural/exempt: vat_type="exempt"

Respond ONLY with valid JSON array: [{"index": 0, "vat_type": "...", "category": "...", "confidence": 0.90}, ...]`

// ClassifierService handles transaction classification (rules + LLM).
type ClassifierService struct {
	ai *openai.Client
	q  *sqlc.Queries
}

// NewClassifierService creates a classifier.
func NewClassifierService(ai *openai.Client, q *sqlc.Queries) *ClassifierService {
	return &ClassifierService{ai: ai, q: q}
}

// ClassificationResult holds the output for a single transaction.
type ClassificationResult struct {
	VATType              string  `json:"vat_type"`
	Category             string  `json:"category"`
	Confidence           float64 `json:"confidence"`
	ClassificationSource string  `json:"classification_source"`
}

// ClassifyTransactions runs a two-phase classification pipeline.
// Phase 1: deterministic rules. Phase 2: LLM batch classification.
func (s *ClassifierService) ClassifyTransactions(
	ctx context.Context,
	transactions []map[string]interface{},
	companyID uuid.UUID,
	promptSupplement string,
) ([]ClassificationResult, error) {
	results := make([]ClassificationResult, len(transactions))
	var ambiguous []int // indices needing LLM

	// Phase 1: Rule-based classification
	for i, tx := range transactions {
		if r := applyRuleBasedClassification(tx); r != nil {
			results[i] = *r
		} else {
			ambiguous = append(ambiguous, i)
		}
	}

	// Phase 1.5: Learned rules from corrections DB
	rules, _ := s.q.ListActiveCorrectionRulesByCompany(ctx, companyID)
	if len(rules) > 0 {
		var stillAmbiguous []int
		for _, idx := range ambiguous {
			if r := applyLearnedRules(transactions[idx], rules); r != nil {
				results[idx] = *r
			} else {
				stillAmbiguous = append(stillAmbiguous, idx)
			}
		}
		ambiguous = stillAmbiguous
	}

	ruleCount := len(transactions) - len(ambiguous)
	slog.Info("classification phase 1 complete",
		"rule", ruleCount,
		"ambiguous", len(ambiguous),
		"total", len(transactions))

	// Phase 2: LLM batch classification
	if len(ambiguous) > 0 && s.ai != nil {
		batches := batchIndices(ambiguous, classificationBatchSize)
		for _, batch := range batches {
			items := make([]map[string]interface{}, len(batch))
			for i, idx := range batch {
				items[i] = transactions[idx]
			}
			llmResults, err := s.classifyBatch(ctx, items, promptSupplement)
			if err != nil {
				slog.Warn("LLM classification failed, using defaults", "error", err)
				for _, idx := range batch {
					results[idx] = ClassificationResult{
						VATType: "vatable", Category: "goods",
						Confidence: 0.30, ClassificationSource: "default",
					}
				}
				continue
			}
			for i, idx := range batch {
				if i < len(llmResults) {
					results[idx] = llmResults[i]
				}
			}
		}
	}

	return results, nil
}

// applyRuleBasedClassification applies deterministic rules without LLM.
func applyRuleBasedClassification(tx map[string]interface{}) *ClassificationResult {
	desc := strings.ToLower(toString(tx["description"]))
	tin := toString(tx["tin"])
	sourceType := toString(tx["source_type"])
	isPurchase := sourceType == "purchase_record"

	// Determine default category based on source_type
	salesCategory := "sale"
	if isPurchase {
		salesCategory = "goods"
	}

	// Government entity detection
	if isGovernmentEntity(desc, tin) {
		return &ClassificationResult{
			VATType: "government", Category: salesCategory,
			Confidence: 0.95, ClassificationSource: "rule",
		}
	}

	// Export-related keywords
	exportKW := []string{"export", "foreign buyer", "ecofree", "peza"}
	for _, kw := range exportKW {
		if strings.Contains(desc, kw) {
			return &ClassificationResult{
				VATType: "zero_rated", Category: salesCategory,
				Confidence: 0.90, ClassificationSource: "rule",
			}
		}
	}

	// VAT-exempt keywords
	exemptKW := []string{"agricultural", "senior citizen", "pwd discount", "educational", "cooperative"}
	for _, kw := range exemptKW {
		if strings.Contains(desc, kw) {
			return &ClassificationResult{
				VATType: "exempt", Category: "goods",
				Confidence: 0.85, ClassificationSource: "rule",
			}
		}
	}

	return nil
}

func isGovernmentEntity(desc, tin string) bool {
	govtKW := []string{"bir", "sss", "philhealth", "pag-ibig", "hdmf", "lgu", "municipality", "dti", "sec"}
	for _, kw := range govtKW {
		if strings.Contains(desc, kw) {
			return true
		}
	}
	for _, prefix := range govtTINPrefixes {
		if strings.HasPrefix(tin, prefix) {
			return true
		}
	}
	return false
}

func applyLearnedRules(tx map[string]interface{}, rules []sqlc.CorrectionRule) *ClassificationResult {
	for _, rule := range rules {
		var criteria struct {
			Field    string      `json:"field"`
			Operator string      `json:"operator"`
			Value    interface{} `json:"value"`
		}
		if err := json.Unmarshal(rule.MatchCriteria, &criteria); err != nil {
			continue
		}

		fieldVal := strings.ToLower(toString(tx[criteria.Field]))
		if fieldVal == "" {
			continue
		}

		matched := false
		switch criteria.Operator {
		case "contains":
			if s, ok := criteria.Value.(string); ok {
				matched = strings.Contains(fieldVal, strings.ToLower(s))
			}
		case "contains_any":
			if arr, ok := criteria.Value.([]interface{}); ok {
				for _, v := range arr {
					if s, ok := v.(string); ok && strings.Contains(fieldVal, strings.ToLower(s)) {
						matched = true
						break
					}
				}
			}
		}

		if matched {
			conf := 0.80
			if rule.Confidence.Valid {
				if f, err := rule.Confidence.Float64Value(); err == nil {
					conf = f.Float64
				}
			}
			result := &ClassificationResult{
				VATType:              toString(tx["vat_type"]),
				Category:             toString(tx["category"]),
				Confidence:           conf,
				ClassificationSource: "learned_rule",
			}
			switch rule.CorrectionField {
			case "category":
				result.Category = rule.CorrectionValue
			case "vat_type":
				result.VATType = rule.CorrectionValue
			default:
				result.VATType = rule.CorrectionValue
			}
			return result
		}
	}
	return nil
}

func (s *ClassifierService) classifyBatch(ctx context.Context, items []map[string]interface{}, supplement string) ([]ClassificationResult, error) {
	// Build batch input
	var sb strings.Builder
	sb.WriteString("[")
	for i, item := range items {
		if i > 0 {
			sb.WriteString(",")
		}
		desc := toString(item["description"])
		if len(desc) > 200 {
			desc = desc[:200]
		}
		fmt.Fprintf(&sb, `{"index":%d,"date":"%s","description":"%s","amount":"%s","tin":"%s","source_type":"%s"}`,
			i, toString(item["date"]), desc, toString(item["amount"]), toString(item["tin"]), toString(item["source_type"]))
	}
	sb.WriteString("]")

	systemPrompt := classificationSystemPrompt
	if supplement != "" {
		systemPrompt += "\n\n" + supplement
	}

	resp, err := s.ai.ChatCompletion(ctx, []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: oai.ChatMessageRoleUser, Content: sb.String()},
	}, openai.WithTemperature(0.1), openai.WithMaxTokens(2048))

	if err != nil {
		return nil, fmt.Errorf("LLM classification: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	// Parse response
	var llmResults []struct {
		Index      int     `json:"index"`
		VATType    string  `json:"vat_type"`
		Category   string  `json:"category"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &llmResults); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	results := make([]ClassificationResult, len(items))
	for _, lr := range llmResults {
		if lr.Index < 0 || lr.Index >= len(results) {
			continue
		}
		vatType := lr.VATType
		if !validVATTypes[vatType] {
			vatType = "vatable"
		}
		category := lr.Category
		if !validCategories[category] {
			category = "goods"
		}
		// Enforce: purchase_record transactions must never have category="sale"
		if lr.Index < len(items) && toString(items[lr.Index]["source_type"]) == "purchase_record" && category == "sale" {
			category = "goods"
		}
		conf := lr.Confidence
		if conf < 0 || conf > 1 {
			conf = 0.5
		}
		results[lr.Index] = ClassificationResult{
			VATType: vatType, Category: category,
			Confidence: conf, ClassificationSource: "llm",
		}
	}

	// Fill defaults for any missing indices
	for i := range results {
		if results[i].ClassificationSource == "" {
			results[i] = ClassificationResult{
				VATType: "vatable", Category: "goods",
				Confidence: 0.30, ClassificationSource: "default",
			}
		}
	}

	return results, nil
}

func batchIndices(indices []int, batchSize int) [][]int {
	var batches [][]int
	for i := 0; i < len(indices); i += batchSize {
		end := i + batchSize
		if end > len(indices) {
			end = len(indices)
		}
		batches = append(batches, indices[i:end])
	}
	return batches
}
