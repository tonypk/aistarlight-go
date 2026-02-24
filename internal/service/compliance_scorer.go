package service

// SeverityPenalties defines deduction per failed rule check.
var severityPenalties = map[string]int{
	"critical": 30,
	"high":     15,
	"medium":   5,
	"low":      2,
}

// RAGSeverityPenalties defines deduction per RAG finding.
var ragSeverityPenalties = map[string]int{
	"high":   10,
	"medium": 5,
	"low":    2,
}

// RAGFinding represents an AI-detected compliance finding.
type RAGFinding struct {
	Finding             string `json:"finding"`
	Severity            string `json:"severity"`
	RegulationReference string `json:"regulation_reference"`
}

// CalculateComplianceScore computes a 0-100 score from check results and RAG findings.
func CalculateComplianceScore(checks []CheckResult, ragFindings []RAGFinding) int {
	score := 100

	for _, c := range checks {
		if !c.Passed {
			if penalty, ok := severityPenalties[c.Severity]; ok {
				score -= penalty
			}
		}
	}

	for _, f := range ragFindings {
		if penalty, ok := ragSeverityPenalties[f.Severity]; ok {
			score -= penalty
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}
