package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// TaxRuleService provides versioned tax rate and penalty lookups from the database.
type TaxRuleService struct {
	q *sqlc.Queries
}

// NewTaxRuleService creates a tax rule service.
func NewTaxRuleService(q *sqlc.Queries) *TaxRuleService {
	return &TaxRuleService{q: q}
}

// TaxRuleDTO is the API-facing representation of a tax rule.
type TaxRuleDTO struct {
	ID            string      `json:"id"`
	RuleType      string      `json:"rule_type"`
	RuleKey       string      `json:"rule_key"`
	Value         interface{} `json:"value"`
	EffectiveFrom string      `json:"effective_from"`
	EffectiveTo   *string     `json:"effective_to,omitempty"`
	SourceRef     string      `json:"source_ref,omitempty"`
	Description   string      `json:"description,omitempty"`
}

// PenaltyInput holds inputs for penalty calculation.
type PenaltyInput struct {
	FormType string          `json:"form_type"`
	Period   string          `json:"period"`
	DaysLate int             `json:"days_late"`
	TaxDue   decimal.Decimal `json:"tax_due"`
}

// PenaltyResult holds computed penalty breakdown.
type PenaltyResult struct {
	Surcharge    decimal.Decimal `json:"surcharge"`
	Interest     decimal.Decimal `json:"interest"`
	Compromise   decimal.Decimal `json:"compromise"`
	TotalPenalty decimal.Decimal `json:"total_penalty"`
	Details      PenaltyDetails  `json:"details"`
}

// PenaltyDetails provides the explanation for each penalty component.
type PenaltyDetails struct {
	SurchargeRate    decimal.Decimal `json:"surcharge_rate"`
	InterestRate     decimal.Decimal `json:"interest_rate"`
	SurchargeRef     string          `json:"surcharge_ref"`
	InterestRef      string          `json:"interest_ref"`
	CompromiseRef    string          `json:"compromise_ref"`
}

func toPgDate(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

// GetRate retrieves the current tax rate for a given rule key.
func (s *TaxRuleService) GetRate(ctx context.Context, ruleKey string, asOf time.Time, jurisdiction string) (decimal.Decimal, error) {
	if jurisdiction == "" {
		jurisdiction = "PH"
	}
	rule, err := s.q.GetActiveRule(ctx, sqlc.GetActiveRuleParams{
		RuleType:      "rate",
		RuleKey:       ruleKey,
		EffectiveFrom: toPgDate(asOf),
		Jurisdiction:  jurisdiction,
	})
	if err != nil {
		return decimal.Zero, fmt.Errorf("rule %s not found: %w", ruleKey, err)
	}

	var val map[string]interface{}
	if err := json.Unmarshal(rule.Value, &val); err != nil {
		return decimal.Zero, fmt.Errorf("parse rule value: %w", err)
	}

	rate, ok := val["rate"]
	if !ok {
		return decimal.Zero, fmt.Errorf("rule %s has no rate field", ruleKey)
	}

	return decimal.NewFromFloat(toFloat64(rate)), nil
}

// CalculatePenalty computes penalties for late filing/payment.
func (s *TaxRuleService) CalculatePenalty(ctx context.Context, input PenaltyInput, jurisdiction string) (*PenaltyResult, error) {
	if jurisdiction == "" {
		jurisdiction = "PH"
	}
	now := time.Now()

	// 1. Get surcharge rate
	surchargeRate := decimal.NewFromFloat(0.25) // default
	surchargeRef := "NIRC Sec 248(A)"
	surchargeRule, err := s.q.GetActiveRule(ctx, sqlc.GetActiveRuleParams{
		RuleType: "penalty", RuleKey: "LATE_FILING_SURCHARGE", EffectiveFrom: toPgDate(now), Jurisdiction: jurisdiction,
	})
	if err == nil {
		var v map[string]interface{}
		if json.Unmarshal(surchargeRule.Value, &v) == nil {
			if r, ok := v["rate"]; ok {
				surchargeRate = decimal.NewFromFloat(toFloat64(r))
			}
		}
		surchargeRef = derefString(surchargeRule.SourceRef)
	}

	// 2. Get interest rate
	interestRate := decimal.NewFromFloat(0.12) // default
	interestRef := "NIRC Sec 249"
	interestRule, err := s.q.GetActiveRule(ctx, sqlc.GetActiveRuleParams{
		RuleType: "penalty", RuleKey: "INTEREST_RATE", EffectiveFrom: toPgDate(now), Jurisdiction: jurisdiction,
	})
	if err == nil {
		var v map[string]interface{}
		if json.Unmarshal(interestRule.Value, &v) == nil {
			if r, ok := v["rate"]; ok {
				interestRate = decimal.NewFromFloat(toFloat64(r))
			}
		}
		interestRef = derefString(interestRule.SourceRef)
	}

	// 3. Calculate surcharge
	surcharge := input.TaxDue.Mul(surchargeRate)

	// 4. Calculate interest (pro-rated by days)
	dailyRate := interestRate.Div(decimal.NewFromInt(365))
	interest := input.TaxDue.Mul(dailyRate).Mul(decimal.NewFromInt(int64(input.DaysLate)))

	// 5. Get compromise penalty
	compromise := decimal.Zero
	compromiseRef := "RMO 7-2015"
	compromiseRule, err := s.q.GetActiveRule(ctx, sqlc.GetActiveRuleParams{
		RuleType: "penalty", RuleKey: "COMPROMISE_LATE_PAYMENT", EffectiveFrom: toPgDate(now), Jurisdiction: jurisdiction,
	})
	if err == nil {
		var v map[string]interface{}
		if json.Unmarshal(compromiseRule.Value, &v) == nil {
			compromise = lookupCompromiseTier(v, input.TaxDue)
		}
		compromiseRef = derefString(compromiseRule.SourceRef)
	}

	total := surcharge.Add(interest).Add(compromise)

	return &PenaltyResult{
		Surcharge:    surcharge.Round(2),
		Interest:     interest.Round(2),
		Compromise:   compromise.Round(2),
		TotalPenalty: total.Round(2),
		Details: PenaltyDetails{
			SurchargeRate: surchargeRate,
			InterestRate:  interestRate,
			SurchargeRef:  surchargeRef,
			InterestRef:   interestRef,
			CompromiseRef: compromiseRef,
		},
	}, nil
}

// ListRules returns all rules of a given type.
func (s *TaxRuleService) ListRules(ctx context.Context, ruleType string, jurisdiction string) ([]TaxRuleDTO, error) {
	if jurisdiction == "" {
		jurisdiction = "PH"
	}
	rules, err := s.q.ListRulesByType(ctx, sqlc.ListRulesByTypeParams{
		RuleType:     ruleType,
		Jurisdiction: jurisdiction,
	})
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	return mapTaxRules(rules), nil
}

// ListActiveRules returns currently active rules of a given type.
func (s *TaxRuleService) ListActiveRules(ctx context.Context, ruleType string, jurisdiction string) ([]TaxRuleDTO, error) {
	if jurisdiction == "" {
		jurisdiction = "PH"
	}
	rules, err := s.q.ListActiveRulesByType(ctx, sqlc.ListActiveRulesByTypeParams{
		RuleType:      ruleType,
		EffectiveFrom: toPgDate(time.Now()),
		Jurisdiction:  jurisdiction,
	})
	if err != nil {
		return nil, fmt.Errorf("list active rules: %w", err)
	}
	return mapTaxRules(rules), nil
}

func mapTaxRules(rules []sqlc.TaxRule) []TaxRuleDTO {
	dtos := make([]TaxRuleDTO, len(rules))
	for i, r := range rules {
		var val interface{}
		_ = json.Unmarshal(r.Value, &val)

		dto := TaxRuleDTO{
			ID:            r.ID.String(),
			RuleType:      r.RuleType,
			RuleKey:       r.RuleKey,
			Value:         val,
			EffectiveFrom: r.EffectiveFrom.Time.Format("2006-01-02"),
			SourceRef:     derefString(r.SourceRef),
			Description:   derefString(r.Description),
		}
		if r.EffectiveTo.Valid {
			s := r.EffectiveTo.Time.Format("2006-01-02")
			dto.EffectiveTo = &s
		}
		dtos[i] = dto
	}
	return dtos
}

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func lookupCompromiseTier(v map[string]interface{}, taxDue decimal.Decimal) decimal.Decimal {
	tiersRaw, ok := v["tiers"]
	if !ok {
		return decimal.Zero
	}

	tiersJSON, err := json.Marshal(tiersRaw)
	if err != nil {
		return decimal.Zero
	}

	var tiers []struct {
		Min     float64 `json:"min"`
		Max     float64 `json:"max"`
		Penalty float64 `json:"penalty"`
	}
	if err := json.Unmarshal(tiersJSON, &tiers); err != nil {
		return decimal.Zero
	}

	for _, t := range tiers {
		minD := decimal.NewFromFloat(t.Min)
		maxD := decimal.NewFromFloat(t.Max)

		if taxDue.GreaterThanOrEqual(minD) && (maxD.IsZero() || taxDue.LessThanOrEqual(maxD)) {
			return decimal.NewFromFloat(t.Penalty)
		}
	}

	return decimal.Zero
}
