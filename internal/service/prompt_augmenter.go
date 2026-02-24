package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

const (
	maxAugmentRules       = 15
	maxRecentCorrections  = 10
	maxSanitizedTextLen   = 200
)

var injectionPattern = regexp.MustCompile(`(?i)(ignore|disregard|forget).*(previous|above|all)`)
var codeBlockPattern = regexp.MustCompile("(?s)```.*?```")

// PromptAugmenter generates tenant-specific prompt supplements from correction history.
type PromptAugmenter struct {
	q *sqlc.Queries
}

// NewPromptAugmenter creates a prompt augmenter.
func NewPromptAugmenter(q *sqlc.Queries) *PromptAugmenter {
	return &PromptAugmenter{q: q}
}

// GeneratePromptSupplement builds a prompt supplement from learned rules and corrections.
func (a *PromptAugmenter) GeneratePromptSupplement(ctx context.Context, companyID uuid.UUID) string {
	rules, err := a.q.ListActiveCorrectionRulesByCompany(ctx, companyID)
	if err != nil || len(rules) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n--- Tenant-Specific Classification Rules ---\n")

	count := 0
	for _, rule := range rules {
		if count >= maxAugmentRules {
			break
		}

		var criteria struct {
			Field    string      `json:"field"`
			Operator string      `json:"operator"`
			Value    interface{} `json:"value"`
		}
		if err := json.Unmarshal(rule.MatchCriteria, &criteria); err != nil {
			continue
		}

		valueStr := sanitize(fmt.Sprintf("%v", criteria.Value))
		conf := 0.0
		if rule.Confidence.Valid {
			if f, err := rule.Confidence.Float64Value(); err == nil {
				conf = f.Float64 * 100
			}
		}

		fmt.Fprintf(&sb, "When %s %s [%s]: set %s=%s (confidence: %.0f%%, from %d corrections)\n",
			sanitize(criteria.Field),
			sanitize(criteria.Operator),
			valueStr,
			sanitize(rule.CorrectionField),
			sanitize(rule.CorrectionValue),
			conf,
			rule.SourceCorrectionCount,
		)
		count++
	}

	return sb.String()
}

func sanitize(text string) string {
	// Remove prompt injection patterns
	text = injectionPattern.ReplaceAllString(text, "")
	// Remove code blocks
	text = codeBlockPattern.ReplaceAllString(text, "")
	// Collapse whitespace
	text = strings.Join(strings.Fields(text), " ")
	// Truncate
	if len(text) > maxSanitizedTextLen {
		text = text[:maxSanitizedTextLen]
	}
	return text
}
