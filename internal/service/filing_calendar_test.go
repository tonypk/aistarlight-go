package service

import "testing"

func TestGenerateFilingCalendar_NotEmpty(t *testing.T) {
	entries := GenerateFilingCalendar(2026, 12)
	if len(entries) == 0 {
		t.Fatal("GenerateFilingCalendar returned no entries for 12 months ahead")
	}

	// Check entries are sorted by deadline
	for i := 1; i < len(entries); i++ {
		if entries[i].Deadline < entries[i-1].Deadline {
			t.Errorf("Entries not sorted: %s > %s at index %d", entries[i-1].Deadline, entries[i].Deadline, i)
		}
	}
}

func TestGenerateFilingCalendar_HasAllForms(t *testing.T) {
	entries := GenerateFilingCalendar(2026, 12)

	formSeen := make(map[string]bool)
	for _, e := range entries {
		formSeen[e.Form] = true
	}

	expectedForms := []string{"BIR 2550M", "BIR 1601-C", "BIR 0619-E", "BIR 2550Q"}
	for _, form := range expectedForms {
		if !formSeen[form] {
			t.Errorf("Expected form %s in calendar entries", form)
		}
	}
}

func TestGenerateFilingCalendar_StatusValues(t *testing.T) {
	entries := GenerateFilingCalendar(2026, 12)
	validStatuses := map[string]bool{
		"overdue":   true,
		"upcoming":  true,
		"scheduled": true,
	}

	for _, e := range entries {
		if !validStatuses[e.Status] {
			t.Errorf("Invalid status %q for entry %s %s", e.Status, e.Form, e.Period)
		}
	}
}

func TestQuarterEndMonth(t *testing.T) {
	tests := []struct {
		month int
		want  int
	}{
		{1, 3}, {2, 3}, {3, 3},
		{4, 6}, {5, 6}, {6, 6},
		{7, 9}, {8, 9}, {9, 9},
		{10, 12}, {11, 12}, {12, 12},
	}

	for _, tt := range tests {
		got := quarterEndMonth(tt.month)
		if got != tt.want {
			t.Errorf("quarterEndMonth(%d) = %d, want %d", tt.month, got, tt.want)
		}
	}
}

func TestMakeEntry_StatusLogic(t *testing.T) {
	// This indirectly tests makeEntry via GenerateFilingCalendar
	entries := GenerateFilingCalendar(2026, 12)
	for _, e := range entries {
		if e.DaysRemaining < 0 && e.Status != "overdue" {
			t.Errorf("Entry with negative days (%d) should be overdue, got %q: %s %s",
				e.DaysRemaining, e.Status, e.Form, e.Period)
		}
		if e.DaysRemaining >= 0 && e.DaysRemaining <= 7 && e.Status != "upcoming" {
			t.Errorf("Entry with %d days should be upcoming, got %q: %s %s",
				e.DaysRemaining, e.Status, e.Form, e.Period)
		}
	}
}
