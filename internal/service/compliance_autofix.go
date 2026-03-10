package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
)

// AutoFixSuggestion represents a single field-level fix suggestion from the AI.
type AutoFixSuggestion struct {
	Field          string `json:"field"`
	CurrentValue   string `json:"current_value"`
	SuggestedValue string `json:"suggested_value"`
	Reason         string `json:"reason"`
}

// AutoFixResult holds the complete set of AI-generated fix suggestions.
type AutoFixResult struct {
	ReportID    uuid.UUID           `json:"report_id"`
	Suggestions []AutoFixSuggestion `json:"suggestions"`
	Summary     string              `json:"summary"`
}

// GenerateAutoFixSuggestions calls LLM to generate field-level repair suggestions
// for a report that failed compliance checks.
func (s *ComplianceService) GenerateAutoFixSuggestions(ctx context.Context, reportID uuid.UUID, ai *openai.Client, jurisdictions ...string) (*AutoFixResult, error) {
	jurisdiction := "PH"
	if len(jurisdictions) > 0 && jurisdictions[0] != "" {
		jurisdiction = jurisdictions[0]
	}
	if ai == nil {
		return nil, fmt.Errorf("AI service not available")
	}

	report, err := s.q.GetReportByID(ctx, reportID)
	if err != nil {
		return nil, fmt.Errorf("report not found: %w", err)
	}

	// Get latest validation
	validation, err := s.GetLatestValidation(ctx, reportID)
	if err != nil {
		return nil, fmt.Errorf("no validation found — run compliance check first")
	}

	// Parse calculated data
	var calcData map[string]string
	if err := json.Unmarshal(report.CalculatedData, &calcData); err != nil {
		return nil, fmt.Errorf("parse calculated data: %w", err)
	}

	// Build failed checks description
	var failedChecks []map[string]string
	for _, c := range validation.CheckResults {
		if !c.Passed {
			failedChecks = append(failedChecks, map[string]string{
				"check_id": c.CheckID,
				"severity": c.Severity,
				"message":  c.Message,
			})
		}
	}

	if len(failedChecks) == 0 {
		return &AutoFixResult{
			ReportID: reportID,
			Summary:  "All compliance checks passed. No fixes needed.",
		}, nil
	}

	calcJSON, _ := json.Marshal(calcData)
	failedJSON, _ := json.Marshal(failedChecks)

	var sysPrompt, taxRules string
	switch jurisdiction {
	case "SG":
		sysPrompt = "You are a Singapore IRAS tax compliance expert. Provide precise, calculation-based fix suggestions following IRAS regulations."
		taxRules = "Use Singapore tax rules (9% GST, 17% corporate tax, CPF contributions, S45 withholding tax rates)."
	case "LK":
		sysPrompt = "You are a Sri Lanka IRD tax compliance expert. Provide precise, calculation-based fix suggestions following the Inland Revenue Act."
		taxRules = "Use Sri Lanka tax rules (18% VAT, 30% corporate tax, EPF 12%+8%, ETF 3%, WHT rates per IRA)."
	default:
		sysPrompt = "You are a Philippine BIR tax compliance expert. Provide precise, calculation-based fix suggestions."
		taxRules = "Use Philippine tax rules (12% VAT, 5% government VAT, etc.)."
	}

	prompt := fmt.Sprintf(`You are a tax compliance expert. A %s report has failed compliance checks.

Current report data:
%s

Failed checks:
%s

For each failed check, suggest specific field-level fixes. Only suggest changes that would fix the compliance issues.
Do NOT change fields that are correct. %s

Important: These are suggestions for the user to review and confirm. Never change user-submitted raw data without their approval.

Respond in JSON:
{"suggestions": [{"field": "field_name", "current_value": "current", "suggested_value": "new_value", "reason": "explanation"}], "summary": "brief summary of fixes"}`,
		report.ReportType, string(calcJSON), string(failedJSON), taxRules)

	messages := []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: sysPrompt},
		{Role: oai.ChatMessageRoleUser, Content: prompt},
	}

	resp, err := ai.ChatCompletion(ctx, messages,
		openai.WithTemperature(0.1),
		openai.WithMaxTokens(2000),
		openai.WithJSONResponse(),
	)
	if err != nil {
		return nil, fmt.Errorf("AI call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty AI response")
	}

	var result AutoFixResult
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		return nil, fmt.Errorf("parse AI response: %w", err)
	}

	result.ReportID = reportID
	return &result, nil
}

// ApplyAutoFix applies user-confirmed AI suggestions as overrides to a report.
// This requires explicit user action — AI never modifies data autonomously.
func (s *ComplianceService) ApplyAutoFix(ctx context.Context, reportID, userID uuid.UUID, suggestions []AutoFixSuggestion, reportSvc *ReportService) (*AutoFixResult, error) {
	report, err := s.q.GetReportByID(ctx, reportID)
	if err != nil {
		return nil, fmt.Errorf("report not found: %w", err)
	}

	overrides := make(map[string]string, len(suggestions))
	for _, s := range suggestions {
		overrides[s.Field] = s.SuggestedValue
	}

	_, err = reportSvc.ApplyOverrides(ctx, OverrideInput{
		ReportID:  reportID,
		UserID:    userID,
		Overrides: overrides,
		Version:   report.Version,
	})
	if err != nil {
		return nil, fmt.Errorf("apply overrides: %w", err)
	}

	return &AutoFixResult{
		ReportID:    reportID,
		Suggestions: suggestions,
		Summary:     fmt.Sprintf("Applied %d fix(es) as overrides", len(suggestions)),
	}, nil
}
