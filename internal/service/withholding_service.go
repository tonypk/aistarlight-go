package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// EWT exempt keywords — transactions matching these are not subject to EWT.
var ewtExemptKeywords = []string{
	"salary", "wages", "payroll",
	"government", "bir", "sss", "philhealth", "pag-ibig",
	"utility", "electric bill", "water bill", "internet bill",
	"bank charge", "bank fee", "interest expense",
}

// EWTClassificationResult holds the classification output for a single transaction.
type EWTClassificationResult struct {
	EWTApplicable        bool    `json:"ewt_applicable"`
	ATCCode              string  `json:"atc_code,omitempty"`
	EWTRate              float64 `json:"ewt_rate,omitempty"`
	IncomeType           string  `json:"income_type,omitempty"`
	Confidence           float64 `json:"confidence"`
	ClassificationSource string  `json:"classification_source"` // rule, learned, ai
}

// WithholdingService handles EWT classification and certificate generation.
type WithholdingService struct {
	q *sqlc.Queries
}

// NewWithholdingService creates a WithholdingService.
func NewWithholdingService(q *sqlc.Queries) *WithholdingService {
	return &WithholdingService{q: q}
}

// ClassifyEWTTransactions classifies transactions for EWT applicability.
func (s *WithholdingService) ClassifyEWTTransactions(
	ctx context.Context,
	transactions []map[string]interface{},
	companyID uuid.UUID,
) ([]EWTClassificationResult, error) {
	results := make([]EWTClassificationResult, len(transactions))

	// Load supplier lookup
	suppliers, _ := s.q.ListSuppliersByCompany(ctx, sqlc.ListSuppliersByCompanyParams{
		CompanyID: companyID,
		Limit:     1000,
		Offset:    0,
	})
	supplierByTIN := make(map[string]sqlc.Supplier)
	for _, sup := range suppliers {
		supplierByTIN[sup.Tin] = sup
	}

	// Load learned rules
	rules, _ := s.q.ListActiveCorrectionRulesByCompany(ctx, companyID)
	ewtRules := filterEWTRules(rules)

	for i, tx := range transactions {
		// Phase 1: Rule-based
		result := classifyEWTRuleBased(tx, supplierByTIN)
		if result != nil {
			results[i] = *result
			continue
		}

		// Phase 1.5: Learned rules
		result = applyLearnedEWTRules(tx, ewtRules)
		if result != nil {
			results[i] = *result
			continue
		}

		// Default: not applicable
		results[i] = EWTClassificationResult{
			EWTApplicable:        false,
			Confidence:           0.30,
			ClassificationSource: "rule",
		}
	}

	return results, nil
}

// CreateCertificate creates a withholding certificate record.
func (s *WithholdingService) CreateCertificate(
	ctx context.Context,
	companyID uuid.UUID,
	supplierID uuid.UUID,
	sessionID *uuid.UUID,
	period, quarter, atcCode string,
	incomeAmount, ewtRate, taxWithheld decimal.Decimal,
) (*sqlc.WithholdingCertificate, error) {
	sessID := pgtype.UUID{}
	if sessionID != nil {
		sessID = pgtype.UUID{Bytes: *sessionID, Valid: true}
	}

	incomeNumeric := pgtype.Numeric{}
	_ = incomeNumeric.Scan(incomeAmount.String())
	rateNumeric := pgtype.Numeric{}
	_ = rateNumeric.Scan(ewtRate.String())
	taxNumeric := pgtype.Numeric{}
	_ = taxNumeric.Scan(taxWithheld.String())

	cert, err := s.q.CreateWithholdingCertificate(ctx, sqlc.CreateWithholdingCertificateParams{
		ID:           uuid.New(),
		CompanyID:    companyID,
		SessionID:    sessID,
		SupplierID:   supplierID,
		Period:       period,
		Quarter:      quarter,
		AtcCode:      atcCode,
		IncomeType:   GetEWTIncomeType(atcCode),
		IncomeAmount: incomeNumeric,
		EwtRate:      rateNumeric,
		TaxWithheld:  taxNumeric,
		Status:       "draft",
	})
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}
	return &cert, nil
}

// ListCertificates returns certificates for a company.
func (s *WithholdingService) ListCertificates(ctx context.Context, companyID uuid.UUID, limit, offset int) ([]sqlc.WithholdingCertificate, int64, error) {
	certs, err := s.q.ListWithholdingByCompany(ctx, sqlc.ListWithholdingByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}

	total, err := s.q.CountWithholdingByCompany(ctx, companyID)
	if err != nil {
		return nil, 0, err
	}

	return certs, total, nil
}

// GetCertificate returns a certificate by ID.
func (s *WithholdingService) GetCertificate(ctx context.Context, id uuid.UUID) (*sqlc.WithholdingCertificate, error) {
	cert, err := s.q.GetWithholdingCertificateByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("certificate not found: %w", err)
	}
	return &cert, nil
}

func classifyEWTRuleBased(tx map[string]interface{}, suppliers map[string]sqlc.Supplier) *EWTClassificationResult {
	desc := strings.ToLower(toString(tx["description"]))
	tin := toString(tx["tin"])

	// Check exempt keywords
	for _, kw := range ewtExemptKeywords {
		if strings.Contains(desc, kw) {
			return &EWTClassificationResult{
				EWTApplicable:        false,
				Confidence:           0.90,
				ClassificationSource: "rule",
			}
		}
	}

	// Check supplier default
	if tin != "" {
		if sup, ok := suppliers[tin]; ok && sup.DefaultAtcCode != nil {
			rate, err := GetEWTRate(*sup.DefaultAtcCode)
			if err == nil {
				return &EWTClassificationResult{
					EWTApplicable:        true,
					ATCCode:              *sup.DefaultAtcCode,
					EWTRate:              rate.InexactFloat64(),
					IncomeType:           GetEWTIncomeType(*sup.DefaultAtcCode),
					Confidence:           0.85,
					ClassificationSource: "rule",
				}
			}
		}
	}

	// Keyword matching
	supplierType := toString(tx["supplier_type"])
	if supplierType == "" {
		supplierType = "corporation"
	}
	atc := FindATCByKeywords(desc, supplierType)
	if atc != "" {
		rate, err := GetEWTRate(atc)
		if err == nil {
			return &EWTClassificationResult{
				EWTApplicable:        true,
				ATCCode:              atc,
				EWTRate:              rate.InexactFloat64(),
				IncomeType:           GetEWTIncomeType(atc),
				Confidence:           0.70,
				ClassificationSource: "rule",
			}
		}
	}

	return nil
}

func applyLearnedEWTRules(tx map[string]interface{}, rules []sqlc.CorrectionRule) *EWTClassificationResult {
	desc := strings.ToLower(toString(tx["description"]))

	for _, rule := range rules {
		var criteria map[string]interface{}
		if err := json.Unmarshal(rule.MatchCriteria, &criteria); err != nil {
			continue
		}

		operator := toString(criteria["operator"])
		values, _ := criteria["value"].([]interface{})

		matched := false
		switch operator {
		case "contains_any":
			for _, v := range values {
				if strings.Contains(desc, strings.ToLower(fmt.Sprint(v))) {
					matched = true
					break
				}
			}
		case "in":
			tin := toString(tx["tin"])
			for _, v := range values {
				if tin == fmt.Sprint(v) {
					matched = true
					break
				}
			}
		}

		if matched {
			switch rule.CorrectionField {
			case "atc_code":
				rate, err := GetEWTRate(rule.CorrectionValue)
				if err == nil {
					return &EWTClassificationResult{
						EWTApplicable:        true,
						ATCCode:              rule.CorrectionValue,
						EWTRate:              rate.InexactFloat64(),
						IncomeType:           GetEWTIncomeType(rule.CorrectionValue),
						Confidence:           0.80,
						ClassificationSource: "learned",
					}
				}
			case "ewt_applicable":
				if rule.CorrectionValue == "false" {
					return &EWTClassificationResult{
						EWTApplicable:        false,
						Confidence:           0.80,
						ClassificationSource: "learned",
					}
				}
			}
		}
	}

	return nil
}

func filterEWTRules(rules []sqlc.CorrectionRule) []sqlc.CorrectionRule {
	var filtered []sqlc.CorrectionRule
	for _, r := range rules {
		if r.CorrectionField == "atc_code" || r.CorrectionField == "ewt_rate" || r.CorrectionField == "ewt_applicable" {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
