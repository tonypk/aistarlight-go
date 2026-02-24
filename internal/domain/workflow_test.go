package domain

import "testing"

func TestIsValidTransition(t *testing.T) {
	tests := []struct {
		from, to ReportStatus
		want     bool
	}{
		// Happy path
		{StatusDraft, StatusReview, true},
		{StatusReview, StatusApproved, true},
		{StatusReview, StatusRejected, true},
		{StatusReview, StatusDraft, true},
		{StatusApproved, StatusFiled, true},
		{StatusApproved, StatusReview, true},
		{StatusRejected, StatusDraft, true},
		{StatusFiled, StatusArchived, true},
		// Invalid transitions
		{StatusDraft, StatusApproved, false},
		{StatusDraft, StatusFiled, false},
		{StatusDraft, StatusArchived, false},
		{StatusReview, StatusFiled, false},
		{StatusApproved, StatusDraft, false},
		{StatusApproved, StatusArchived, false},
		{StatusRejected, StatusApproved, false},
		{StatusFiled, StatusDraft, false},
		{StatusArchived, StatusDraft, false},
		{StatusArchived, StatusReview, false},
		// Unknown status
		{"unknown", StatusDraft, false},
		{StatusDraft, "unknown", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"→"+string(tt.to), func(t *testing.T) {
			got := IsValidTransition(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("IsValidTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestIsEditable(t *testing.T) {
	tests := []struct {
		status ReportStatus
		want   bool
	}{
		{StatusDraft, true},
		{StatusReview, true},
		{StatusRejected, true},
		{StatusApproved, false},
		{StatusFiled, false},
		{StatusArchived, false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := IsEditable(tt.status)
			if got != tt.want {
				t.Errorf("IsEditable(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestWorkflowFullLifecycle(t *testing.T) {
	// Test a complete report lifecycle: draft → review → approved → filed → archived
	lifecycle := []ReportStatus{StatusDraft, StatusReview, StatusApproved, StatusFiled, StatusArchived}

	for i := 0; i < len(lifecycle)-1; i++ {
		if !IsValidTransition(lifecycle[i], lifecycle[i+1]) {
			t.Errorf("Expected valid transition from %s to %s", lifecycle[i], lifecycle[i+1])
		}
	}
}

func TestWorkflowRejectionLoop(t *testing.T) {
	// Test rejection loop: draft → review → rejected → draft → review → approved
	steps := []ReportStatus{StatusDraft, StatusReview, StatusRejected, StatusDraft, StatusReview, StatusApproved}

	for i := 0; i < len(steps)-1; i++ {
		if !IsValidTransition(steps[i], steps[i+1]) {
			t.Errorf("Expected valid transition from %s to %s at step %d", steps[i], steps[i+1], i)
		}
	}
}
