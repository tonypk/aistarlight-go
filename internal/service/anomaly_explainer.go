package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
)

// AnomalyExplanation holds AI-generated context for an anomaly.
type AnomalyExplanation struct {
	Explanation  string `json:"explanation"`
	Resolution   string `json:"resolution"`
	BIRReference string `json:"bir_reference"`
}

const anomalyExplainerBatchSize = 20

// ExplainAnomalies sends detected anomalies to the LLM for natural-language explanations.
// Returns nil (graceful degradation) if the AI client is nil or the call fails.
func ExplainAnomalies(ctx context.Context, ai *openai.Client, anomalies []DetectedAnomaly) ([]AnomalyExplanation, error) {
	if ai == nil || len(anomalies) == 0 {
		return nil, nil
	}

	var allExplanations []AnomalyExplanation

	// Process in batches
	for i := 0; i < len(anomalies); i += anomalyExplainerBatchSize {
		end := i + anomalyExplainerBatchSize
		if end > len(anomalies) {
			end = len(anomalies)
		}
		batch := anomalies[i:end]

		explanations, err := explainBatch(ctx, ai, batch)
		if err != nil {
			slog.Warn("anomaly explanation batch failed, skipping", "batch_start", i, "error", err)
			// Fill with empty explanations for this batch
			for range batch {
				allExplanations = append(allExplanations, AnomalyExplanation{})
			}
			continue
		}

		allExplanations = append(allExplanations, explanations...)
	}

	return allExplanations, nil
}

func explainBatch(ctx context.Context, ai *openai.Client, batch []DetectedAnomaly) ([]AnomalyExplanation, error) {
	var sb strings.Builder
	for i, a := range batch {
		fmt.Fprintf(&sb, "[%d] Type: %s | Severity: %s | Description: %s\n", i+1, a.AnomalyType, a.Severity, a.Description)
		if a.Details != nil {
			detailsJSON, _ := json.Marshal(a.Details)
			fmt.Fprintf(&sb, "    Details: %s\n", string(detailsJSON))
		}
	}

	prompt := fmt.Sprintf(`You are a Philippine tax compliance expert. For each anomaly below, provide:
1. A clear, non-technical explanation of what the anomaly means
2. A specific resolution action the accountant should take
3. The relevant BIR regulation or revenue memorandum

Anomalies:
%s

Respond in JSON format:
{"explanations": [{"explanation": "...", "resolution": "...", "bir_reference": "..."}, ...]}

Rules:
- Keep explanations under 200 characters
- Keep resolutions actionable and specific
- Include BIR regulation numbers when applicable (e.g., "RR 16-2005", "RMC 16-2005")
- If no specific BIR reference applies, use "General BIR compliance"`, sb.String())

	messages := []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: "You are a Philippine BIR tax compliance expert assistant."},
		{Role: oai.ChatMessageRoleUser, Content: prompt},
	}

	resp, err := ai.ChatCompletion(ctx, messages,
		openai.WithTemperature(0.2),
		openai.WithMaxTokens(2000),
		openai.WithJSONResponse(),
	)
	if err != nil {
		return nil, fmt.Errorf("AI explanation call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty AI response")
	}

	var result struct {
		Explanations []AnomalyExplanation `json:"explanations"`
	}
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		return nil, fmt.Errorf("parse AI response: %w", err)
	}

	// Pad if AI returned fewer explanations than anomalies
	for len(result.Explanations) < len(batch) {
		result.Explanations = append(result.Explanations, AnomalyExplanation{})
	}

	return result.Explanations[:len(batch)], nil
}
