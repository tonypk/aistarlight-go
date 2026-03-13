package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// VendorMemoryService manages vendor posting policies that learn from user feedback.
type VendorMemoryService struct {
	q *sqlc.Queries
}

// NewVendorMemoryService creates a VendorMemoryService.
func NewVendorMemoryService(q *sqlc.Queries) *VendorMemoryService {
	return &VendorMemoryService{q: q}
}

// VendorPolicy is the API-friendly representation of a vendor posting policy.
type VendorPolicy struct {
	ID               uuid.UUID `json:"id"`
	CompanyID        uuid.UUID `json:"company_id"`
	VendorNormalized string    `json:"vendor_normalized"`
	Aliases          []string  `json:"aliases"`
	DefaultCategory  string    `json:"default_category"`
	AccountCode      string    `json:"account_code"`
	TaxCode          string    `json:"tax_code"`
	Department       string    `json:"department"`
	Project          string    `json:"project"`
	UsageCount       int       `json:"usage_count"`
	AcceptCount      int       `json:"accept_count"`
	CorrectionCount  int       `json:"correction_count"`
	ConfidenceScore  float64   `json:"confidence_score"`
}

// RuleSuggestion represents a vendor pattern that could be promoted to a rule.
type RuleSuggestion struct {
	VendorNormalized string `json:"vendor_normalized"`
	CorrectionCount  int    `json:"correction_count"`
	AcceptCount      int    `json:"accept_count"`
	ConfidenceScore  float64 `json:"confidence_score"`
	DefaultCategory  string `json:"default_category"`
	AccountCode      string `json:"account_code"`
	TaxCode          string `json:"tax_code"`
	Message          string `json:"message"`
}

// vendor name normalization patterns
var (
	vendorSuffixRe  = regexp.MustCompile(`(?i)\s*(pte\.?\s*ltd\.?|inc\.?|llc\.?|corp\.?|co\.?\s*ltd\.?|sdn\.?\s*bhd\.?|limited|corporation)\s*$`)
	vendorPrefixRe  = regexp.MustCompile(`(?i)^(sq\s*\*|grab\s*\*|paypal\s*\*|stripe\s*\*|goog\s*\*|amzn\s*\*|apple\.com/bill)\s*`)
	multiSpaceRe    = regexp.MustCompile(`\s+`)
)

// NormalizeVendor cleans raw OCR vendor text into a canonical form.
func NormalizeVendor(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = vendorSuffixRe.ReplaceAllString(s, "")
	s = vendorPrefixRe.ReplaceAllString(s, "")
	s = multiSpaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	return s
}

// MatchVendor tries to find a vendor policy for the given raw vendor text.
// Priority: exact normalized match > alias match > fuzzy search.
func (s *VendorMemoryService) MatchVendor(ctx context.Context, companyID uuid.UUID, rawVendor string) (*VendorPolicy, error) {
	normalized := NormalizeVendor(rawVendor)
	if normalized == "" {
		return nil, nil
	}

	// 1. Exact normalized match
	row, err := s.q.GetVendorPolicy(ctx, sqlc.GetVendorPolicyParams{
		CompanyID:        companyID,
		VendorNormalized: normalized,
	})
	if err == nil {
		return toVendorPolicy(row), nil
	}

	// 2. Alias match (check if raw text is in any vendor's aliases)
	row, err = s.q.GetVendorPolicyByAlias(ctx, sqlc.GetVendorPolicyByAliasParams{
		CompanyID: companyID,
		Column2:   rawVendor,
	})
	if err == nil {
		return toVendorPolicy(row), nil
	}

	// 3. Fuzzy search (ILIKE)
	rows, err := s.q.SearchVendorByName(ctx, sqlc.SearchVendorByNameParams{
		CompanyID: companyID,
		Column2:   &normalized,
	})
	if err == nil && len(rows) == 1 {
		return toVendorPolicy(rows[0]), nil
	}

	return nil, nil
}

// RecordAcceptance is called when a user confirms the suggested classification.
func (s *VendorMemoryService) RecordAcceptance(
	ctx context.Context,
	companyID uuid.UUID,
	rawVendor string,
	category, accountCode, taxCode, department, project string,
) error {
	normalized := NormalizeVendor(rawVendor)
	if normalized == "" {
		return nil
	}

	aliasArr := []string{rawVendor}
	aliasJSON, _ := json.Marshal(aliasArr)

	_, err := s.q.UpsertVendorPolicy(ctx, sqlc.UpsertVendorPolicyParams{
		CompanyID:          companyID,
		VendorNormalized:   normalized,
		Aliases:            aliasJSON,
		DefaultCategory:    &category,
		DefaultAccountCode: &accountCode,
		DefaultTaxCode:     &taxCode,
		DefaultDepartment:  &department,
		DefaultProject:     &project,
		UsageCount:         1,
		AcceptCount:        1,
		ConfidenceScore:    numericFromFloat(1.0),
	})
	if err != nil {
		slog.Error("vendor memory: record acceptance failed", "vendor", normalized, "error", err)
		return fmt.Errorf("record acceptance: %w", err)
	}
	return nil
}

// RecordCorrection is called when a user changes a suggested value.
func (s *VendorMemoryService) RecordCorrection(
	ctx context.Context,
	companyID uuid.UUID,
	rawVendor string,
	correctedCategory, correctedAccountCode, correctedTaxCode string,
) error {
	normalized := NormalizeVendor(rawVendor)
	if normalized == "" {
		return nil
	}

	// First ensure the vendor exists
	aliasArr := []string{rawVendor}
	aliasJSON, _ := json.Marshal(aliasArr)

	_, err := s.q.UpsertVendorPolicy(ctx, sqlc.UpsertVendorPolicyParams{
		CompanyID:          companyID,
		VendorNormalized:   normalized,
		Aliases:            aliasJSON,
		DefaultCategory:    strPtr(correctedCategory),
		DefaultAccountCode: strPtr(correctedAccountCode),
		DefaultTaxCode:     strPtr(correctedTaxCode),
		DefaultDepartment:  strPtr(""),
		DefaultProject:     strPtr(""),
		UsageCount:         1,
		AcceptCount:        0,
		ConfidenceScore:    numericFromFloat(0.5),
	})
	if err != nil {
		slog.Error("vendor memory: upsert on correction failed", "vendor", normalized, "error", err)
		return fmt.Errorf("record correction upsert: %w", err)
	}

	// Increment correction count (this also recalculates confidence)
	err = s.q.IncrementVendorCorrection(ctx, sqlc.IncrementVendorCorrectionParams{
		CompanyID:        companyID,
		VendorNormalized: normalized,
	})
	if err != nil {
		slog.Error("vendor memory: increment correction failed", "vendor", normalized, "error", err)
		return fmt.Errorf("increment correction: %w", err)
	}
	return nil
}

// SuggestRules returns vendors with repeated corrections that could become rules.
func (s *VendorMemoryService) SuggestRules(ctx context.Context, companyID uuid.UUID, minCorrections int32) ([]RuleSuggestion, error) {
	if minCorrections <= 0 {
		minCorrections = 5
	}

	rows, err := s.q.ListRuleSuggestions(ctx, sqlc.ListRuleSuggestionsParams{
		CompanyID:       companyID,
		CorrectionCount: minCorrections,
	})
	if err != nil {
		return nil, fmt.Errorf("list rule suggestions: %w", err)
	}

	suggestions := make([]RuleSuggestion, 0, len(rows))
	for _, r := range rows {
		cat := deref(r.DefaultCategory)
		suggestions = append(suggestions, RuleSuggestion{
			VendorNormalized: r.VendorNormalized,
			CorrectionCount:  int(r.CorrectionCount),
			AcceptCount:      int(r.AcceptCount),
			ConfidenceScore:  numericToFloat(r.ConfidenceScore),
			DefaultCategory:  cat,
			AccountCode:      deref(r.DefaultAccountCode),
			TaxCode:          deref(r.DefaultTaxCode),
			Message:          fmt.Sprintf("You've categorized \"%s\" as \"%s\" multiple times. Set as default?", r.VendorNormalized, cat),
		})
	}
	return suggestions, nil
}

// PromoteRule resets correction count and boosts confidence for a vendor.
func (s *VendorMemoryService) PromoteRule(ctx context.Context, companyID uuid.UUID, vendorNormalized string) error {
	err := s.q.IncrementVendorAcceptance(ctx, sqlc.IncrementVendorAcceptanceParams{
		CompanyID:        companyID,
		VendorNormalized: vendorNormalized,
	})
	if err != nil {
		return fmt.Errorf("promote rule: %w", err)
	}
	return nil
}

// ListPolicies returns all vendor policies for a company.
func (s *VendorMemoryService) ListPolicies(ctx context.Context, companyID uuid.UUID, limit, offset int32) ([]VendorPolicy, error) {
	rows, err := s.q.ListVendorPolicies(ctx, sqlc.ListVendorPoliciesParams{
		CompanyID: companyID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list vendor policies: %w", err)
	}

	policies := make([]VendorPolicy, 0, len(rows))
	for _, r := range rows {
		policies = append(policies, *toVendorPolicy(r))
	}
	return policies, nil
}

// GetPolicy returns a single vendor policy by ID.
func (s *VendorMemoryService) GetPolicy(ctx context.Context, id, companyID uuid.UUID) (*VendorPolicy, error) {
	row, err := s.q.GetVendorPolicyByID(ctx, sqlc.GetVendorPolicyByIDParams{
		ID:        id,
		CompanyID: companyID,
	})
	if err != nil {
		return nil, fmt.Errorf("get vendor policy: %w", err)
	}
	return toVendorPolicy(row), nil
}

// DeletePolicy removes a vendor policy.
func (s *VendorMemoryService) DeletePolicy(ctx context.Context, id, companyID uuid.UUID) error {
	return s.q.DeleteVendorPolicy(ctx, sqlc.DeleteVendorPolicyParams{
		ID:        id,
		CompanyID: companyID,
	})
}

// UpdateDefaults manually updates vendor defaults.
func (s *VendorMemoryService) UpdateDefaults(
	ctx context.Context,
	companyID uuid.UUID,
	vendorNormalized, category, accountCode, taxCode, department, project string,
) error {
	return s.q.UpdateVendorDefaults(ctx, sqlc.UpdateVendorDefaultsParams{
		CompanyID:        companyID,
		VendorNormalized: vendorNormalized,
		Column3:          &category,
		Column4:          &accountCode,
		Column5:          &taxCode,
		Column6:          &department,
		Column7:          &project,
	})
}

// --- helpers ---

func toVendorPolicy(r sqlc.VendorPostingPolicy) *VendorPolicy {
	var aliases []string
	if len(r.Aliases) > 0 {
		_ = json.Unmarshal(r.Aliases, &aliases)
	}
	return &VendorPolicy{
		ID:               r.ID,
		CompanyID:        r.CompanyID,
		VendorNormalized: r.VendorNormalized,
		Aliases:          aliases,
		DefaultCategory:  deref(r.DefaultCategory),
		AccountCode:      deref(r.DefaultAccountCode),
		TaxCode:          deref(r.DefaultTaxCode),
		Department:       deref(r.DefaultDepartment),
		Project:          deref(r.DefaultProject),
		UsageCount:       int(r.UsageCount),
		AcceptCount:      int(r.AcceptCount),
		CorrectionCount:  int(r.CorrectionCount),
		ConfidenceScore:  numericToFloat(r.ConfidenceScore),
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func numericFromFloat(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(fmt.Sprintf("%.4f", f))
	return n
}

func numericToFloat(n pgtype.Numeric) float64 {
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return math.Round(f.Float64*10000) / 10000
}
