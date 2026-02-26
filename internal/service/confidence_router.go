package service

// Confidence thresholds for classification routing.
// Note: these are informational only — AI never auto-modifies user data.
// The frontend uses these to prioritize which transactions to show first.
const (
	ConfidenceHighThreshold = 0.90 // >= 0.90: high confidence, can be batch-confirmed by user
	ConfidenceLowThreshold  = 0.70 // < 0.70: needs review, shown first in UI
)

// ClassifyConfidenceLevel categorizes a confidence score for UI display.
func ClassifyConfidenceLevel(confidence float64) string {
	switch {
	case confidence >= ConfidenceHighThreshold:
		return "high"
	case confidence >= ConfidenceLowThreshold:
		return "medium"
	default:
		return "low"
	}
}

// ClassificationSummary provides a breakdown of classification results by confidence level.
type ClassificationSummary struct {
	Classified    int `json:"classified"`
	HighConf      int `json:"high_confidence"`
	MediumConf    int `json:"medium_confidence"`
	NeedsReview   int `json:"needs_review"`
	Total         int `json:"total"`
}

// BuildClassificationSummary creates a summary from classification results.
func BuildClassificationSummary(results []ClassificationResult, totalTxns int) ClassificationSummary {
	summary := ClassificationSummary{
		Classified: len(results),
		Total:      totalTxns,
	}
	for _, r := range results {
		switch ClassifyConfidenceLevel(r.Confidence) {
		case "high":
			summary.HighConf++
		case "medium":
			summary.MediumConf++
		default:
			summary.NeedsReview++
		}
	}
	return summary
}
