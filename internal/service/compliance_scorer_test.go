package service

import "testing"

func TestCalculateComplianceScore_AllPassed(t *testing.T) {
	checks := []CheckResult{
		{CheckID: "1", Passed: true, Severity: "critical"},
		{CheckID: "2", Passed: true, Severity: "high"},
		{CheckID: "3", Passed: true, Severity: "medium"},
	}
	score := CalculateComplianceScore(checks, nil)
	if score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}
}

func TestCalculateComplianceScore_CriticalFail(t *testing.T) {
	checks := []CheckResult{
		{CheckID: "1", Passed: false, Severity: "critical"},
	}
	score := CalculateComplianceScore(checks, nil)
	// 100 - 30 = 70
	if score != 70 {
		t.Errorf("Expected 70, got %d", score)
	}
}

func TestCalculateComplianceScore_MultipleFails(t *testing.T) {
	checks := []CheckResult{
		{CheckID: "1", Passed: false, Severity: "critical"}, // -30
		{CheckID: "2", Passed: false, Severity: "high"},     // -15
		{CheckID: "3", Passed: false, Severity: "medium"},   // -5
		{CheckID: "4", Passed: false, Severity: "low"},      // -2
		{CheckID: "5", Passed: true, Severity: "critical"},  // no penalty
	}
	score := CalculateComplianceScore(checks, nil)
	// 100 - 30 - 15 - 5 - 2 = 48
	if score != 48 {
		t.Errorf("Expected 48, got %d", score)
	}
}

func TestCalculateComplianceScore_WithRAGFindings(t *testing.T) {
	checks := []CheckResult{
		{CheckID: "1", Passed: true, Severity: "critical"},
	}
	findings := []RAGFinding{
		{Finding: "Missing receipt", Severity: "high"},   // -10
		{Finding: "Late filing", Severity: "medium"},     // -5
		{Finding: "Minor issue", Severity: "low"},        // -2
	}
	score := CalculateComplianceScore(checks, findings)
	// 100 - 10 - 5 - 2 = 83
	if score != 83 {
		t.Errorf("Expected 83, got %d", score)
	}
}

func TestCalculateComplianceScore_FloorAtZero(t *testing.T) {
	checks := []CheckResult{
		{CheckID: "1", Passed: false, Severity: "critical"}, // -30
		{CheckID: "2", Passed: false, Severity: "critical"}, // -30
		{CheckID: "3", Passed: false, Severity: "critical"}, // -30
		{CheckID: "4", Passed: false, Severity: "critical"}, // -30
	}
	score := CalculateComplianceScore(checks, nil)
	// 100 - 120 = clamped to 0
	if score != 0 {
		t.Errorf("Expected 0 (floor), got %d", score)
	}
}

func TestCalculateComplianceScore_Empty(t *testing.T) {
	score := CalculateComplianceScore(nil, nil)
	if score != 100 {
		t.Errorf("Expected 100 for empty input, got %d", score)
	}
}

func TestCalculateComplianceScore_MixedChecksAndFindings(t *testing.T) {
	checks := []CheckResult{
		{CheckID: "1", Passed: false, Severity: "high"},     // -15
		{CheckID: "2", Passed: false, Severity: "medium"},   // -5
	}
	findings := []RAGFinding{
		{Finding: "Issue A", Severity: "high"},   // -10
		{Finding: "Issue B", Severity: "medium"}, // -5
	}
	score := CalculateComplianceScore(checks, findings)
	// 100 - 15 - 5 - 10 - 5 = 65
	if score != 65 {
		t.Errorf("Expected 65, got %d", score)
	}
}
