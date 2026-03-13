package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// ComplianceService orchestrates compliance validation for reports.
type ComplianceService struct {
	q         *sqlc.Queries
	knowledge *KnowledgeService
}

// NewComplianceService creates a ComplianceService.
func NewComplianceService(q *sqlc.Queries, knowledge *KnowledgeService) *ComplianceService {
	return &ComplianceService{q: q, knowledge: knowledge}
}

// ValidationOutput holds the result of a compliance validation.
type ValidationOutput struct {
	ID           uuid.UUID    `json:"id"`
	ReportID     uuid.UUID    `json:"report_id"`
	OverallScore int          `json:"overall_score"`
	CheckResults []CheckResult `json:"check_results"`
	RAGFindings  []RAGFinding `json:"rag_findings"`
	ValidatedAt  string       `json:"validated_at"`
}

// ValidateReport runs full compliance validation: rules + RAG + score + persist.
func (s *ComplianceService) ValidateReport(ctx context.Context, reportID, companyID uuid.UUID) (*ValidationOutput, error) {
	// 1. Load report
	report, err := s.q.GetReportByID(ctx, reportID)
	if err != nil {
		return nil, fmt.Errorf("report not found: %w", err)
	}

	var calcData map[string]interface{}
	if len(report.CalculatedData) > 0 {
		_ = json.Unmarshal(report.CalculatedData, &calcData)
	}
	if calcData == nil {
		calcData = make(map[string]interface{})
	}
	calcData["period"] = report.Period
	calcData["report_type"] = report.ReportType

	// 2. Load prior period data
	priorData := s.getPriorPeriodData(ctx, companyID, report.ReportType, report.Period)

	// 3. Load existing reports for duplicate check (exclude current report)
	allReports := s.getExistingReports(ctx, companyID, report.ReportType)
	existingReports := make([]map[string]interface{}, 0, len(allReports))
	for _, r := range allReports {
		if toString(r["id"]) != reportID.String() {
			existingReports = append(existingReports, r)
		}
	}

	// 4. Run deterministic rules
	checks := RunAllChecks(calcData, report.ReportType, priorData, existingReports)

	// 5. Run RAG validation
	ragFindings := s.runRAGValidation(ctx, calcData, report.ReportType)

	// 6. Calculate score
	score := CalculateComplianceScore(checks, ragFindings)

	// 7. Persist
	checksJSON, _ := json.Marshal(checks)
	ragJSON, _ := json.Marshal(ragFindings)

	vr, err := s.q.CreateValidationResult(ctx, sqlc.CreateValidationResultParams{
		ID:           uuid.New(),
		ReportID:     reportID,
		CompanyID:    companyID,
		OverallScore: int32(score),
		CheckResults: checksJSON,
		RagFindings:  ragJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("save validation: %w", err)
	}

	// 8. Update report compliance score
	scoreInt := int32(score)
	_ = s.q.UpdateReport(ctx, sqlc.UpdateReportParams{
		ID:              reportID,
		Status:          report.Status,
		ComplianceScore: &scoreInt,
		Version:         report.Version,
	})

	return &ValidationOutput{
		ID:           vr.ID,
		ReportID:     reportID,
		OverallScore: score,
		CheckResults: checks,
		RAGFindings:  ragFindings,
		ValidatedAt:  vr.ValidatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// GetLatestValidation returns the most recent validation for a report.
func (s *ComplianceService) GetLatestValidation(ctx context.Context, reportID uuid.UUID) (*ValidationOutput, error) {
	vr, err := s.q.GetLatestValidationByReport(ctx, reportID)
	if err != nil {
		return nil, fmt.Errorf("validation not found: %w", err)
	}

	var checks []CheckResult
	var findings []RAGFinding
	_ = json.Unmarshal(vr.CheckResults, &checks)
	_ = json.Unmarshal(vr.RagFindings, &findings)

	return &ValidationOutput{
		ID:           vr.ID,
		ReportID:     vr.ReportID,
		OverallScore: int(vr.OverallScore),
		CheckResults: checks,
		RAGFindings:  findings,
		ValidatedAt:  vr.ValidatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// ListValidations returns all validations for a report.
func (s *ComplianceService) ListValidations(ctx context.Context, reportID uuid.UUID) ([]ValidationOutput, error) {
	rows, err := s.q.ListValidationsByReport(ctx, reportID)
	if err != nil {
		return nil, err
	}

	results := make([]ValidationOutput, len(rows))
	for i, vr := range rows {
		var checks []CheckResult
		var findings []RAGFinding
		_ = json.Unmarshal(vr.CheckResults, &checks)
		_ = json.Unmarshal(vr.RagFindings, &findings)

		results[i] = ValidationOutput{
			ID:           vr.ID,
			ReportID:     vr.ReportID,
			OverallScore: int(vr.OverallScore),
			CheckResults: checks,
			RAGFindings:  findings,
			ValidatedAt:  vr.ValidatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return results, nil
}

func (s *ComplianceService) getPriorPeriodData(ctx context.Context, companyID uuid.UUID, reportType, currentPeriod string) map[string]interface{} {
	reports, err := s.q.ListReportsByCompanyAndType(ctx, sqlc.ListReportsByCompanyAndTypeParams{
		CompanyID:  companyID,
		ReportType: reportType,
		Limit:      10,
		Offset:     0,
	})
	if err != nil || len(reports) == 0 {
		return nil
	}

	for _, r := range reports {
		if r.Period < currentPeriod && len(r.CalculatedData) > 0 {
			var data map[string]interface{}
			_ = json.Unmarshal(r.CalculatedData, &data)
			return data
		}
	}
	return nil
}

func (s *ComplianceService) getExistingReports(ctx context.Context, companyID uuid.UUID, reportType string) []map[string]interface{} {
	reports, err := s.q.ListReportsByCompanyAndType(ctx, sqlc.ListReportsByCompanyAndTypeParams{
		CompanyID:  companyID,
		ReportType: reportType,
		Limit:      50,
		Offset:     0,
	})
	if err != nil {
		return nil
	}

	result := make([]map[string]interface{}, len(reports))
	for i, r := range reports {
		result[i] = map[string]interface{}{
			"id":          r.ID.String(),
			"report_type": r.ReportType,
			"period":      r.Period,
			"status":      r.Status,
		}
	}
	return result
}

func (s *ComplianceService) runRAGValidation(ctx context.Context, data map[string]interface{}, reportType string) []RAGFinding {
	if s.knowledge == nil {
		return nil
	}

	query := fmt.Sprintf("BIR %s compliance requirements filing rules", reportType)
	answer, err := s.knowledge.AnswerQuestion(ctx, query, nil, nil)
	if err != nil {
		slog.Warn("RAG validation failed", "error", err)
		return nil
	}

	// Parse structured findings from RAG response
	var findings []RAGFinding
	if err := json.Unmarshal([]byte(answer), &findings); err != nil {
		// Not structured JSON — skip
		return nil
	}

	// Validate and sanitize
	valid := make([]RAGFinding, 0, len(findings))
	for _, f := range findings {
		if f.Finding == "" {
			continue
		}
		if len(f.Finding) > 500 {
			f.Finding = f.Finding[:500]
		}
		if len(f.RegulationReference) > 200 {
			f.RegulationReference = f.RegulationReference[:200]
		}
		if f.Severity != "high" && f.Severity != "medium" && f.Severity != "low" {
			f.Severity = "low"
		}
		valid = append(valid, f)
	}
	return valid
}
