package service

import (
	"testing"
)

func TestCategorySummary_JSON(t *testing.T) {
	cs := CategorySummary{
		Category: "Transport",
		Count:    5,
		Total:    "1500.00",
	}
	if cs.Category != "Transport" {
		t.Errorf("Category = %q, want Transport", cs.Category)
	}
	if cs.Count != 5 {
		t.Errorf("Count = %d, want 5", cs.Count)
	}
}

func TestVendorSummary_JSON(t *testing.T) {
	vs := VendorSummary{
		Vendor: "Grab",
		Count:  10,
		Total:  "3200.50",
	}
	if vs.Vendor != "Grab" {
		t.Errorf("Vendor = %q, want Grab", vs.Vendor)
	}
}

func TestMonthSummary_JSON(t *testing.T) {
	ms := MonthSummary{
		Month: "2026-01",
		Count: 20,
		Total: "15000.00",
	}
	if ms.Month != "2026-01" {
		t.Errorf("Month = %q, want 2026-01", ms.Month)
	}
}

func TestSpendingDashboard_EmptySlices(t *testing.T) {
	d := SpendingDashboard{}
	if d.ByCategory != nil {
		t.Error("ByCategory should be nil by default")
	}
	if d.ByVendor != nil {
		t.Error("ByVendor should be nil by default")
	}
}

func TestSpendingPeriod_Format(t *testing.T) {
	sp := SpendingPeriod{
		From: "2026-01-01",
		To:   "2026-03-31",
	}
	if sp.From != "2026-01-01" {
		t.Errorf("From = %q, want 2026-01-01", sp.From)
	}
	if sp.To != "2026-03-31" {
		t.Errorf("To = %q, want 2026-03-31", sp.To)
	}
}

func TestIncomeExpenseSummary_Types(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
	}{
		{"income", "income"},
		{"expense", "expense"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := IncomeExpenseSummary{Type: tt.typeName, Count: 1, Total: "100"}
			if s.Type != tt.typeName {
				t.Errorf("Type = %q, want %q", s.Type, tt.typeName)
			}
		})
	}
}
