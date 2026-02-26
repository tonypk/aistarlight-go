package service

import (
	"context"
	"log/slog"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// ChecklistItem represents a single compliance check loaded from DB.
type ChecklistItem struct {
	CheckID     string `json:"check_id"`
	CheckName   string `json:"check_name"`
	Severity    string `json:"severity"`
	Description string `json:"description,omitempty"`
	RuleRef     string `json:"rule_ref,omitempty"`
	SortOrder   int32  `json:"sort_order"`
}

// RuleResolver provides compliance checks from the database with hardcoded fallback.
type RuleResolver struct {
	q *sqlc.Queries
}

// NewRuleResolver creates a rule resolver.
func NewRuleResolver(q *sqlc.Queries) *RuleResolver {
	return &RuleResolver{q: q}
}

// Resolve loads compliance checklist items for a given form type.
// Returns from DB if available, otherwise returns nil (caller should fallback to hardcoded).
func (r *RuleResolver) Resolve(ctx context.Context, formType string) []ChecklistItem {
	rows, err := r.q.ListChecklistsByFormType(ctx, formType)
	if err != nil || len(rows) == 0 {
		slog.Debug("no DB checklists found, will use hardcoded", "form_type", formType)
		return nil
	}

	items := make([]ChecklistItem, len(rows))
	for i, row := range rows {
		items[i] = ChecklistItem{
			CheckID:     row.CheckID,
			CheckName:   row.CheckName,
			Severity:    row.Severity,
			Description: derefString(row.Description),
			RuleRef:     derefString(row.RuleRef),
			SortOrder:   derefInt32(row.SortOrder),
		}
	}
	return items
}

// ListAll returns all active checklist items across all form types.
func (r *RuleResolver) ListAll(ctx context.Context) ([]ChecklistItem, error) {
	rows, err := r.q.ListAllChecklists(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]ChecklistItem, len(rows))
	for i, row := range rows {
		items[i] = ChecklistItem{
			CheckID:     row.CheckID,
			CheckName:   row.CheckName,
			Severity:    row.Severity,
			Description: derefString(row.Description),
			RuleRef:     derefString(row.RuleRef),
			SortOrder:   derefInt32(row.SortOrder),
		}
	}
	return items, nil
}

// ListByFormType returns checklist items for a specific form type.
func (r *RuleResolver) ListByFormType(ctx context.Context, formType string) ([]ChecklistItem, error) {
	items := r.Resolve(ctx, formType)
	if items == nil {
		// Return hardcoded check IDs as fallback info
		return getHardcodedChecklistInfo(formType), nil
	}
	return items, nil
}

func derefInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

// getHardcodedChecklistInfo returns basic info about hardcoded checks.
func getHardcodedChecklistInfo(formType string) []ChecklistItem {
	// These mirror the hardcoded checks in compliance_rules.go
	checks := []ChecklistItem{
		{CheckID: "required_fields", CheckName: "Required Fields", Severity: "critical", SortOrder: 1},
		{CheckID: "cross_field", CheckName: "Cross-field Consistency", Severity: "critical", SortOrder: 2},
		{CheckID: "output_vat", CheckName: "Output VAT Accuracy", Severity: "high", SortOrder: 3},
		{CheckID: "govt_vat", CheckName: "Government VAT Rate", Severity: "high", SortOrder: 4},
		{CheckID: "amount_ranges", CheckName: "Amount Ranges", Severity: "high", SortOrder: 5},
		{CheckID: "tin_format", CheckName: "TIN Format", Severity: "medium", SortOrder: 6},
		{CheckID: "filing_deadline", CheckName: "Filing Deadline", Severity: "medium", SortOrder: 7},
		{CheckID: "period_anomaly", CheckName: "Period-over-Period Anomaly", Severity: "medium", SortOrder: 8},
		{CheckID: "duplicate", CheckName: "Duplicate Report", Severity: "medium", SortOrder: 9},
		{CheckID: "capital_goods", CheckName: "Capital Goods Threshold", Severity: "low", SortOrder: 10},
		{CheckID: "zero_filing", CheckName: "Zero Filing Warning", Severity: "low", SortOrder: 11},
	}

	switch formType {
	case "BIR_2550M", "BIR_2550Q":
		return checks
	case "BIR_1601C":
		return checks[:1] // just required_fields
	case "BIR_0619E":
		return checks[:1]
	default:
		return checks[:1]
	}
}
