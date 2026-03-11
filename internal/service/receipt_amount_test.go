package service

import (
	"testing"
)

func TestSelectAmountByInstruction_DirectMatch(t *testing.T) {
	amounts := []DetectedAmount{
		{Label: "NET TOTAL", Amount: 1500, IsLikelyTotal: true},
		{Label: "TOTAL", Amount: 2000, IsLikelyTotal: true},
		{Label: "SUBTOTAL", Amount: 1800, IsLikelyTotal: false},
	}

	tests := []struct {
		instruction string
		wantLabel   string
		wantOK      bool
	}{
		{"record the net total", "NET TOTAL", true},
		{"use total", "TOTAL", true},
		{"记录净额", "NET TOTAL", true},
		{"小计", "SUBTOTAL", true},
		{"I want the subtotal", "SUBTOTAL", true},
		{"random text", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		da, ok := SelectAmountByInstruction(amounts, tt.instruction)
		if ok != tt.wantOK {
			t.Errorf("SelectAmountByInstruction(%q) ok=%v, want %v", tt.instruction, ok, tt.wantOK)
			continue
		}
		if ok && da.Label != tt.wantLabel {
			t.Errorf("SelectAmountByInstruction(%q) label=%q, want %q", tt.instruction, da.Label, tt.wantLabel)
		}
	}
}

func TestSelectBestAmount_SingleAmount(t *testing.T) {
	amounts := []DetectedAmount{
		{Label: "TOTAL", Amount: 1000, IsLikelyTotal: true},
	}
	da, ok := SelectBestAmount(amounts)
	if !ok {
		t.Fatal("expected ok=true for single amount")
	}
	if da.Amount != 1000 {
		t.Errorf("amount=%f, want 1000", da.Amount)
	}
}

func TestSelectBestAmount_PrioritizesNetTotal(t *testing.T) {
	amounts := []DetectedAmount{
		{Label: "NET TOTAL", Amount: 1500, IsLikelyTotal: true},
		{Label: "GRAND TOTAL", Amount: 2000, IsLikelyTotal: true},
		{Label: "SUBTOTAL", Amount: 1800, IsLikelyTotal: false},
	}
	da, ok := SelectBestAmount(amounts)
	if !ok {
		t.Fatal("expected ok=true")
	}
	// NET TOTAL appears first in allAmountLabels, so it has priority.
	if da.Label != "NET TOTAL" {
		t.Errorf("label=%q, want NET TOTAL", da.Label)
	}
}

func TestSelectBestAmount_OnlyOneTotalLike(t *testing.T) {
	amounts := []DetectedAmount{
		{Label: "TOTAL", Amount: 1000, IsLikelyTotal: true},
		{Label: "VAT AMOUNT", Amount: 120, IsLikelyTotal: false},
		{Label: "DISCOUNT", Amount: 50, IsLikelyTotal: false},
	}
	da, ok := SelectBestAmount(amounts)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if da.Label != "TOTAL" {
		t.Errorf("label=%q, want TOTAL", da.Label)
	}
}

func TestSelectBestAmount_Empty(t *testing.T) {
	_, ok := SelectBestAmount(nil)
	if ok {
		t.Error("expected ok=false for empty amounts")
	}
}

func TestNeedsAmountSelection(t *testing.T) {
	tests := []struct {
		name    string
		amounts []DetectedAmount
		want    bool
	}{
		{
			name:    "empty",
			amounts: nil,
			want:    false,
		},
		{
			name: "single amount",
			amounts: []DetectedAmount{
				{Label: "TOTAL", Amount: 1000, IsLikelyTotal: true},
			},
			want: false,
		},
		{
			name: "two totals",
			amounts: []DetectedAmount{
				{Label: "NET TOTAL", Amount: 1500, IsLikelyTotal: true},
				{Label: "GRAND TOTAL", Amount: 2000, IsLikelyTotal: true},
			},
			want: true,
		},
		{
			name: "one total + one non-total",
			amounts: []DetectedAmount{
				{Label: "TOTAL", Amount: 1000, IsLikelyTotal: true},
				{Label: "VAT AMOUNT", Amount: 120, IsLikelyTotal: false},
			},
			want: false,
		},
		{
			name: "three amounts including non-totals",
			amounts: []DetectedAmount{
				{Label: "TOTAL", Amount: 1000, IsLikelyTotal: true},
				{Label: "VAT AMOUNT", Amount: 120, IsLikelyTotal: false},
				{Label: "DISCOUNT", Amount: 50, IsLikelyTotal: false},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsAmountSelection(tt.amounts)
			if got != tt.want {
				t.Errorf("NeedsAmountSelection() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractAllLabeledAmounts(t *testing.T) {
	lines := []string{
		"STORE NAME",
		"Date: 2024-01-15",
		"Item 1   100.00",
		"Item 2   200.00",
		"SUBTOTAL   300.00",
		"VAT AMOUNT   36.00",
		"DISCOUNT   10.00",
		"TOTAL   326.00",
		"CASH   400.00",
		"CHANGE   74.00",
	}

	amounts := extractAllLabeledAmounts(lines, nil)
	if len(amounts) == 0 {
		t.Fatal("expected at least one detected amount")
	}

	// Check TOTAL is detected as likely total.
	found := false
	for _, da := range amounts {
		if da.Label == "TOTAL" && da.Amount == 326.00 && da.IsLikelyTotal {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected TOTAL: 326.00 as likely total")
	}

	// Check SUBTOTAL is detected as NOT likely total.
	for _, da := range amounts {
		if da.Label == "SUBTOTAL" && da.IsLikelyTotal {
			t.Error("SUBTOTAL should not be marked as likely total")
		}
	}
}
