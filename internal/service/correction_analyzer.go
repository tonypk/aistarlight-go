package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

const ruleThreshold = 3 // Minimum same-pattern corrections to generate a rule

// CorrectionAnalyzer detects patterns in corrections and generates rules.
type CorrectionAnalyzer struct {
	q *sqlc.Queries
}

// NewCorrectionAnalyzer creates a CorrectionAnalyzer.
func NewCorrectionAnalyzer(q *sqlc.Queries) *CorrectionAnalyzer {
	return &CorrectionAnalyzer{q: q}
}

// RuleCandidate represents a detected pattern from corrections.
type RuleCandidate struct {
	RuleType              string                 `json:"rule_type"`
	MatchCriteria         map[string]interface{} `json:"match_criteria"`
	CorrectionField       string                 `json:"correction_field"`
	CorrectionValue       string                 `json:"correction_value"`
	Confidence            float64                `json:"confidence"`
	SourceCorrectionCount int                    `json:"source_correction_count"`
}

// AnalyzeCorrections loads corrections and detects patterns.
func (a *CorrectionAnalyzer) AnalyzeCorrections(ctx context.Context, companyID uuid.UUID) ([]RuleCandidate, error) {
	corrections, err := a.q.ListCorrectionsByCompany(ctx, sqlc.ListCorrectionsByCompanyParams{
		CompanyID: companyID,
		Limit:     500,
		Offset:    0,
	})
	if err != nil {
		return nil, fmt.Errorf("load corrections: %w", err)
	}

	if len(corrections) < ruleThreshold {
		return nil, nil
	}

	return detectPatterns(corrections), nil
}

// PersistCandidateRules saves rule candidates to the database.
func (a *CorrectionAnalyzer) PersistCandidateRules(ctx context.Context, companyID uuid.UUID, candidates []RuleCandidate) ([]sqlc.CorrectionRule, error) {
	var persisted []sqlc.CorrectionRule

	for _, c := range candidates {
		criteriaJSON, _ := json.Marshal(c.MatchCriteria)

		confidenceNumeric := pgtype.Numeric{}
		_ = confidenceNumeric.Scan(fmt.Sprintf("%.2f", c.Confidence))

		rule, err := a.q.CreateCorrectionRule(ctx, sqlc.CreateCorrectionRuleParams{
			ID:                    uuid.New(),
			CompanyID:             companyID,
			RuleType:              c.RuleType,
			MatchCriteria:         criteriaJSON,
			CorrectionField:       c.CorrectionField,
			CorrectionValue:       c.CorrectionValue,
			Confidence:            confidenceNumeric,
			SourceCorrectionCount: int32(c.SourceCorrectionCount),
			IsActive:              true,
		})
		if err != nil {
			continue // skip failed inserts
		}
		persisted = append(persisted, rule)
	}

	return persisted, nil
}

// GetLearningStats returns summary statistics.
func (a *CorrectionAnalyzer) GetLearningStats(ctx context.Context, companyID uuid.UUID) (map[string]interface{}, error) {
	totalCorrections, err := a.q.CountCorrectionsByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}

	rules, err := a.q.ListActiveCorrectionRulesByCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_corrections": totalCorrections,
		"total_rules":       len(rules),
		"active_rules":      len(rules),
	}, nil
}

type correctionGroup struct {
	fieldName string
	newValue  string
	items     []sqlc.Correction
}

func detectPatterns(corrections []sqlc.Correction) []RuleCandidate {
	// Group by (field_name, new_value)
	groups := make(map[string]*correctionGroup)
	for _, c := range corrections {
		key := c.FieldName + ":" + c.NewValue
		g, ok := groups[key]
		if !ok {
			g = &correctionGroup{fieldName: c.FieldName, newValue: c.NewValue}
			groups[key] = g
		}
		g.items = append(g.items, c)
	}

	var candidates []RuleCandidate
	for _, g := range groups {
		if len(g.items) < ruleThreshold {
			continue
		}

		candidate := buildCandidate(g)
		candidates = append(candidates, candidate)
	}

	return candidates
}

func buildCandidate(g *correctionGroup) RuleCandidate {
	confidence := math.Min(0.95, 0.70+float64(len(g.items))*0.05)

	// Extract descriptions from context data
	var descriptions []string
	var tins []string
	var amounts []float64

	for _, c := range g.items {
		if len(c.ContextData) > 0 {
			var ctx map[string]interface{}
			_ = json.Unmarshal(c.ContextData, &ctx)

			if desc := toString(ctx["description"]); desc != "" {
				descriptions = append(descriptions, desc)
			}
			if tin := toString(ctx["tin"]); tin != "" {
				tins = append(tins, tin)
			}
			if amt := parseAmount(ctx["amount"]); amt != 0 {
				amounts = append(amounts, amt)
			}
		}
	}

	// Determine rule type
	keywords := extractKeywords(descriptions)
	if len(keywords) > 0 {
		return RuleCandidate{
			RuleType: "keyword_override",
			MatchCriteria: map[string]interface{}{
				"field":    "description",
				"operator": "contains_any",
				"value":    keywords,
			},
			CorrectionField:       g.fieldName,
			CorrectionValue:       g.newValue,
			Confidence:            confidence,
			SourceCorrectionCount: len(g.items),
		}
	}

	uniqueTINs := uniqueStrings(tins)
	if len(uniqueTINs) > 0 && len(uniqueTINs) <= 3 {
		return RuleCandidate{
			RuleType: "supplier_default",
			MatchCriteria: map[string]interface{}{
				"field":    "tin",
				"operator": "in",
				"value":    uniqueTINs,
			},
			CorrectionField:       g.fieldName,
			CorrectionValue:       g.newValue,
			Confidence:            confidence,
			SourceCorrectionCount: len(g.items),
		}
	}

	if len(amounts) >= ruleThreshold {
		minAmt, maxAmt := minMax(amounts)
		return RuleCandidate{
			RuleType: "amount_threshold",
			MatchCriteria: map[string]interface{}{
				"field":    "amount",
				"operator": "between",
				"value":    []float64{minAmt * 0.8, maxAmt * 1.2},
			},
			CorrectionField:       g.fieldName,
			CorrectionValue:       g.newValue,
			Confidence:            confidence,
			SourceCorrectionCount: len(g.items),
		}
	}

	// Generic fallback
	var oldValues []string
	for _, c := range g.items {
		if c.OldValue != nil {
			oldValues = append(oldValues, *c.OldValue)
		}
	}
	return RuleCandidate{
		RuleType: "generic",
		MatchCriteria: map[string]interface{}{
			"field":    g.fieldName,
			"operator": "was",
			"value":    uniqueStrings(oldValues),
		},
		CorrectionField:       g.fieldName,
		CorrectionValue:       g.newValue,
		Confidence:            confidence,
		SourceCorrectionCount: len(g.items),
	}
}

func extractKeywords(descriptions []string) []string {
	if len(descriptions) < ruleThreshold {
		return nil
	}

	wordCount := make(map[string]int)
	for _, desc := range descriptions {
		words := strings.Fields(strings.ToLower(desc))
		seen := make(map[string]bool)
		for _, w := range words {
			w = strings.Trim(w, ".,;:!?()\"'")
			if len(w) < 3 || seen[w] {
				continue
			}
			seen[w] = true
			wordCount[w]++
		}
	}

	threshold := len(descriptions) / 2
	if threshold < 1 {
		threshold = 1
	}

	var keywords []string
	for word, count := range wordCount {
		if count >= threshold {
			keywords = append(keywords, word)
		}
	}

	// Cap at 10
	if len(keywords) > 10 {
		keywords = keywords[:10]
	}
	return keywords
}

func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range ss {
		if s != "" && !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func minMax(vals []float64) (float64, float64) {
	min, max := vals[0], vals[0]
	for _, v := range vals[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}
