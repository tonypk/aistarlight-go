package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service/birpdf"
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
	q      *sqlc.Queries
	vendor *VendorService
}

// NewWithholdingService creates a WithholdingService.
func NewWithholdingService(q *sqlc.Queries, vendor *VendorService) *WithholdingService {
	return &WithholdingService{q: q, vendor: vendor}
}

// ClassifyEWTTransactions classifies transactions for EWT applicability.
func (s *WithholdingService) ClassifyEWTTransactions(
	ctx context.Context,
	transactions []map[string]interface{},
	companyID uuid.UUID,
) ([]EWTClassificationResult, error) {
	results := make([]EWTClassificationResult, len(transactions))

	// Load vendor lookup
	vendors, _ := s.q.ListVendorsByCompany(ctx, sqlc.ListVendorsByCompanyParams{
		CompanyID: companyID,
		Limit:     1000,
		Offset:    0,
	})
	vendorByTIN := make(map[string]sqlc.Vendor)
	for _, v := range vendors {
		vendorByTIN[v.Tin] = v
	}

	// Load learned rules
	rules, _ := s.q.ListActiveCorrectionRulesByCompany(ctx, companyID)
	ewtRules := filterEWTRules(rules)

	for i, tx := range transactions {
		// Auto-discover vendor if TIN present but not in map
		if s.vendor != nil {
			tin := toString(tx["tin"])
			name := toString(tx["vendor_name"])
			if name == "" {
				name = toString(tx["supplier_name"]) // backward compat
			}
			if tin != "" {
				if _, ok := vendorByTIN[tin]; !ok {
					if v, err := s.vendor.FindOrCreate(ctx, companyID, tin, name); err == nil {
						fetched, fetchErr := s.q.GetVendorByTIN(ctx, sqlc.GetVendorByTINParams{
							CompanyID: companyID,
							Tin:       v.TIN,
						})
						if fetchErr == nil {
							vendorByTIN[tin] = fetched
						}
					} else {
						slog.Warn("auto-create vendor from EWT failed", "tin", tin, "error", err)
					}
				}
			}
		}

		// Phase 1: Rule-based
		result := classifyEWTRuleBased(tx, vendorByTIN)
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
	vendorID uuid.UUID,
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
		VendorID:     vendorID,
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

// GenerateCertificatePDF writes BIR 2307 PDF for the given certificate to w.
func (s *WithholdingService) GenerateCertificatePDF(ctx context.Context, cert *sqlc.WithholdingCertificate, company *sqlc.Company, vendor *sqlc.Vendor) ([]byte, error) {
	data := map[string]string{
		"period":  cert.Period,
		"quarter": cert.Quarter,
	}

	// Payee (vendor) info
	if vendor != nil {
		data["payee_tin"] = vendor.Tin
		data["payee_name"] = vendor.Name
		if vendor.Address != nil {
			data["payee_address"] = *vendor.Address
		}
	}

	// Single line item from certificate
	incAmt := "0.00"
	if f, err := cert.IncomeAmount.Float64Value(); err == nil {
		incAmt = fmt.Sprintf("%.2f", f.Float64)
	}
	ewtRate := "0.00"
	if f, err := cert.EwtRate.Float64Value(); err == nil {
		ewtRate = fmt.Sprintf("%.4f", f.Float64) // stored as decimal fraction
	}
	taxWithheld := "0.00"
	if f, err := cert.TaxWithheld.Float64Value(); err == nil {
		taxWithheld = fmt.Sprintf("%.2f", f.Float64)
	}

	data["total_items"] = "1"
	data["item_1_seq_no"] = "1"
	data["item_1_atc_code"] = cert.AtcCode
	data["item_1_income_amount"] = incAmt
	data["item_1_tax_rate"] = ewtRate
	data["item_1_tax_withheld"] = taxWithheld
	data["total_income_amount"] = incAmt
	data["total_tax_withheld"] = taxWithheld

	companyData := birpdf.CompanyData{}
	if company != nil {
		companyData.Name = company.CompanyName
		if company.TinNumber != nil {
			companyData.TIN = *company.TinNumber
		}
		if company.RdoCode != nil {
			companyData.RDOCode = *company.RdoCode
		}
		if company.Address != nil {
			companyData.Address = *company.Address
		}
	}

	var buf bytes.Buffer
	if err := birpdf.Generate2307(&buf, data, companyData); err != nil {
		return nil, fmt.Errorf("generate 2307 PDF: %w", err)
	}
	return buf.Bytes(), nil
}

// GetCompanyForPDF returns the company record for PDF generation.
func (s *WithholdingService) GetCompanyForPDF(ctx context.Context, companyID uuid.UUID) (*sqlc.Company, error) {
	company, err := s.q.GetCompanyByID(ctx, companyID)
	if err != nil {
		return nil, err
	}
	return &company, nil
}

// GetVendorByID returns the raw vendor record.
func (s *WithholdingService) GetVendorByID(ctx context.Context, id uuid.UUID) (sqlc.Vendor, error) {
	return s.q.GetVendorByID(ctx, id)
}

func classifyEWTRuleBased(tx map[string]interface{}, vendors map[string]sqlc.Vendor) *EWTClassificationResult {
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

	// Check vendor default
	if tin != "" {
		if v, ok := vendors[tin]; ok && v.DefaultAtcCode != nil {
			rate, err := GetEWTRate(*v.DefaultAtcCode)
			if err == nil {
				return &EWTClassificationResult{
					EWTApplicable:        true,
					ATCCode:              *v.DefaultAtcCode,
					EWTRate:              rate.InexactFloat64(),
					IncomeType:           GetEWTIncomeType(*v.DefaultAtcCode),
					Confidence:           0.85,
					ClassificationSource: "rule",
				}
			}
		}
	}

	// Keyword matching
	vendorType := toString(tx["vendor_type"])
	if vendorType == "" {
		vendorType = toString(tx["supplier_type"]) // backward compat
	}
	if vendorType == "" {
		vendorType = "corporation"
	}
	atc := FindATCByKeywords(desc, vendorType)
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
