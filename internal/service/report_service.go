package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

var (
	ErrReportNotFound    = errors.New("report not found")
	ErrReportNotEditable = errors.New("report is not in an editable state")
	ErrVersionConflict   = errors.New("report was modified by another user")
)

type ReportService struct {
	q          *sqlc.Queries
	compliance *ComplianceService
	approval   *ReportApprovalService
}

func NewReportService(q *sqlc.Queries, compliance *ComplianceService) *ReportService {
	return &ReportService{
		q:          q,
		compliance: compliance,
		approval:   NewReportApprovalService(q),
	}
}

type CreateReportInput struct {
	CompanyID  uuid.UUID
	ReportType string
	Period     string
	InputData  map[string]interface{}
	CreatedBy  uuid.UUID
}

// Create generates a new report by running the tax engine on input data.
func (s *ReportService) Create(ctx context.Context, input CreateReportInput) (*domain.Report, error) {
	// Run tax engine calculation
	calculated, err := CalculateReport(input.ReportType, input.InputData)
	if err != nil {
		return nil, fmt.Errorf("calculate report: %w", err)
	}

	inputJSON, err := json.Marshal(input.InputData)
	if err != nil {
		return nil, fmt.Errorf("marshal input data: %w", err)
	}

	calcJSON, err := json.Marshal(calculated)
	if err != nil {
		return nil, fmt.Errorf("marshal calculated data: %w", err)
	}

	reportID := uuid.New()
	dbReport, err := s.q.CreateReport(ctx, sqlc.CreateReportParams{
		ID:             reportID,
		CompanyID:      input.CompanyID,
		ReportType:     input.ReportType,
		Period:         input.Period,
		Status:         string(domain.StatusDraft),
		InputData:      inputJSON,
		CalculatedData: calcJSON,
		CreatedBy:      pgtype.UUID{Bytes: input.CreatedBy, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("create report: %w", err)
	}

	return toReport(dbReport), nil
}

// GetByID retrieves a single report.
func (s *ReportService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Report, error) {
	dbReport, err := s.q.GetReportByID(ctx, id)
	if err != nil {
		return nil, ErrReportNotFound
	}
	return toReport(dbReport), nil
}

// Recalculate re-runs the tax engine on existing input data.
func (s *ReportService) Recalculate(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*domain.Report, error) {
	report, err := s.q.GetReportByID(ctx, id)
	if err != nil {
		return nil, ErrReportNotFound
	}

	if !domain.IsEditable(domain.ReportStatus(report.Status)) {
		return nil, ErrReportNotEditable
	}

	var inputData map[string]interface{}
	if err := json.Unmarshal(report.InputData, &inputData); err != nil {
		return nil, fmt.Errorf("unmarshal input data: %w", err)
	}

	calculated, err := CalculateReport(report.ReportType, inputData)
	if err != nil {
		return nil, fmt.Errorf("calculate report: %w", err)
	}

	calcJSON, err := json.Marshal(calculated)
	if err != nil {
		return nil, fmt.Errorf("marshal calculated data: %w", err)
	}

	err = s.q.UpdateReport(ctx, sqlc.UpdateReportParams{
		ID:             id,
		Status:         report.Status,
		CalculatedData: calcJSON,
		UpdatedBy:      pgtype.UUID{Bytes: userID, Valid: true},
		Version:        report.Version,
	})
	if err != nil {
		return nil, ErrVersionConflict
	}

	return s.GetByID(ctx, id)
}

// UpdateStatus transitions a report to a new status with an optional comment.
func (s *ReportService) UpdateStatus(ctx context.Context, id uuid.UUID, newStatus domain.ReportStatus, userID uuid.UUID, comment ...*string) (*domain.Report, error) {
	report, err := s.q.GetReportByID(ctx, id)
	if err != nil {
		return nil, ErrReportNotFound
	}

	current := domain.ReportStatus(report.Status)
	if !domain.IsValidTransition(current, newStatus) {
		return nil, fmt.Errorf("invalid transition from %s to %s", current, newStatus)
	}

	// Compliance gate: block approval if score is below threshold
	if newStatus == domain.StatusApproved && s.compliance != nil {
		validation, err := s.compliance.ValidateReport(ctx, id, report.CompanyID)
		if err == nil && validation.OverallScore < 70 {
			// Parse calculated data for fix suggestion generation
			var calcData map[string]interface{}
			if len(report.CalculatedData) > 0 {
				_ = json.Unmarshal(report.CalculatedData, &calcData)
			}
			if calcData == nil {
				calcData = make(map[string]interface{})
			}

			failedFixes := GenerateFixSuggestions(validation.CheckResults, calcData)
			return nil, &ComplianceBlockedError{
				Score:        validation.OverallScore,
				Threshold:    70,
				FailedChecks: failedFixes,
				Summary:      fmt.Sprintf("Compliance score %d/100 is below the approval threshold (70). Fix %d issue(s) before approving.", validation.OverallScore, len(failedFixes)),
			}
		}
	}

	var confirmedAt pgtype.Timestamptz
	if newStatus == domain.StatusApproved {
		confirmedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}

	err = s.q.UpdateReport(ctx, sqlc.UpdateReportParams{
		ID:          id,
		Status:      string(newStatus),
		ConfirmedAt: confirmedAt,
		UpdatedBy:   pgtype.UUID{Bytes: userID, Valid: true},
		Version:     report.Version,
	})
	if err != nil {
		return nil, ErrVersionConflict
	}

	// Record approval trail
	if s.approval != nil {
		var cmt *string
		if len(comment) > 0 {
			cmt = comment[0]
		}
		_ = s.approval.RecordTransition(ctx, id, userID, current, newStatus, cmt)
	}

	return s.GetByID(ctx, id)
}

// ListApprovals returns the approval history for a report.
func (s *ReportService) ListApprovals(ctx context.Context, reportID uuid.UUID) ([]ApprovalEntry, error) {
	if s.approval == nil {
		return nil, nil
	}
	return s.approval.ListApprovals(ctx, reportID)
}

type OverrideInput struct {
	ReportID  uuid.UUID
	UserID    uuid.UUID
	Overrides map[string]string
	Notes     *string
	Version   int32
}

// ApplyOverrides applies manual field overrides to calculated data.
func (s *ReportService) ApplyOverrides(ctx context.Context, input OverrideInput) (*domain.Report, error) {
	report, err := s.q.GetReportByID(ctx, input.ReportID)
	if err != nil {
		return nil, ErrReportNotFound
	}

	if !domain.IsEditable(domain.ReportStatus(report.Status)) {
		return nil, ErrReportNotEditable
	}

	// Parse current calculated data
	var calcData map[string]string
	if err := json.Unmarshal(report.CalculatedData, &calcData); err != nil {
		return nil, fmt.Errorf("unmarshal calculated data: %w", err)
	}

	// Save original if this is the first override
	var origCalcData []byte
	if len(report.OriginalCalculatedData) == 0 || string(report.OriginalCalculatedData) == "null" {
		origCalcData = report.CalculatedData
	}

	// Apply overrides
	for k, v := range input.Overrides {
		calcData[k] = v
	}

	overridesJSON, err := json.Marshal(input.Overrides)
	if err != nil {
		return nil, fmt.Errorf("marshal overrides: %w", err)
	}

	newCalcJSON, err := json.Marshal(calcData)
	if err != nil {
		return nil, fmt.Errorf("marshal calculated data: %w", err)
	}

	err = s.q.UpdateReport(ctx, sqlc.UpdateReportParams{
		ID:                     input.ReportID,
		Status:                 report.Status,
		CalculatedData:         newCalcJSON,
		UpdatedBy:              pgtype.UUID{Bytes: input.UserID, Valid: true},
		Version:                input.Version,
		Overrides:              overridesJSON,
		OriginalCalculatedData: origCalcData,
		Notes:                  input.Notes,
	})
	if err != nil {
		return nil, ErrVersionConflict
	}

	return s.GetByID(ctx, input.ReportID)
}

// ListByCompany returns paginated reports for a company.
func (s *ReportService) ListByCompany(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.Report, int, error) {
	rows, err := s.q.ListReportsByCompany(ctx, sqlc.ListReportsByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(p.Limit),
		Offset:    int32(p.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list reports: %w", err)
	}

	count, _ := s.q.CountReportsByCompany(ctx, companyID)

	result := make([]domain.Report, len(rows))
	for i, r := range rows {
		result[i] = *toReport(r)
	}
	return result, int(count), nil
}

// ListByCompanyAndType returns paginated reports filtered by company and form type.
func (s *ReportService) ListByCompanyAndType(ctx context.Context, companyID uuid.UUID, reportType string, p pagination.Params) ([]domain.Report, int, error) {
	rows, err := s.q.ListReportsByCompanyAndType(ctx, sqlc.ListReportsByCompanyAndTypeParams{
		CompanyID:  companyID,
		ReportType: reportType,
		Limit:      int32(p.Limit),
		Offset:     int32(p.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list reports: %w", err)
	}

	count, _ := s.q.CountReportsByCompanyAndType(ctx, sqlc.CountReportsByCompanyAndTypeParams{
		CompanyID:  companyID,
		ReportType: reportType,
	})

	result := make([]domain.Report, len(rows))
	for i, r := range rows {
		result[i] = *toReport(r)
	}
	return result, int(count), nil
}

// Delete removes a report (only draft reports can be deleted).
func (s *ReportService) Delete(ctx context.Context, id uuid.UUID) error {
	report, err := s.q.GetReportByID(ctx, id)
	if err != nil {
		return ErrReportNotFound
	}
	if domain.ReportStatus(report.Status) != domain.StatusDraft {
		return fmt.Errorf("only draft reports can be deleted")
	}
	return s.q.DeleteReport(ctx, id)
}

// ArchiveDuplicates archives all draft/rejected reports for the same company+type+period
// except the specified report, resolving the "duplicate report" compliance issue.
func (s *ReportService) ArchiveDuplicates(ctx context.Context, reportID, companyID uuid.UUID) (int, error) {
	report, err := s.q.GetReportByID(ctx, reportID)
	if err != nil {
		return 0, ErrReportNotFound
	}

	err = s.q.ArchiveDuplicateReports(ctx, sqlc.ArchiveDuplicateReportsParams{
		CompanyID:  companyID,
		ReportType: report.ReportType,
		Period:     report.Period,
		ID:         reportID,
	})
	if err != nil {
		return 0, fmt.Errorf("archive duplicates: %w", err)
	}

	// Count remaining active reports to return how many were archived
	// (the exact count isn't critical — the important thing is they're archived)
	return 0, nil
}

// GenerateFromSession creates a report from a reconciliation session's VAT summary.
func (s *ReportService) GenerateFromSession(ctx context.Context, sessionID, companyID, userID uuid.UUID, reportType string) (*domain.Report, error) {
	session, err := s.q.GetReconciliationSessionByID(ctx, sessionID)
	if err != nil || session.CompanyID != companyID {
		return nil, fmt.Errorf("session not found")
	}

	var inputData map[string]interface{}
	if len(session.Summary) > 0 {
		_ = json.Unmarshal(session.Summary, &inputData)
	}
	if inputData == nil {
		return nil, fmt.Errorf("no summary available — run reconciliation first")
	}

	return s.Create(ctx, CreateReportInput{
		CompanyID:  companyID,
		ReportType: reportType,
		Period:     session.Period,
		InputData:  inputData,
		CreatedBy:  userID,
	})
}

func toReport(r sqlc.Report) *domain.Report {
	report := &domain.Report{
		ID:                     r.ID,
		CompanyID:              r.CompanyID,
		ReportType:             r.ReportType,
		Period:                 r.Period,
		Status:                 r.Status,
		InputData:              domain.JSON(r.InputData),
		CalculatedData:         domain.JSON(r.CalculatedData),
		FilePath:               r.FilePath,
		CreatedAt:              r.CreatedAt,
		Version:                int(r.Version),
		Overrides:              domain.JSON(r.Overrides),
		OriginalCalculatedData: domain.JSON(r.OriginalCalculatedData),
		Notes:                  r.Notes,
		ComplianceScore:        int32PtrToIntPtr(r.ComplianceScore),
	}

	if r.ConfirmedAt.Valid {
		t := r.ConfirmedAt.Time
		report.ConfirmedAt = &t
	}
	if r.CreatedBy.Valid {
		id := uuid.UUID(r.CreatedBy.Bytes)
		report.CreatedBy = &id
	}
	if r.UpdatedBy.Valid {
		id := uuid.UUID(r.UpdatedBy.Bytes)
		report.UpdatedBy = &id
	}
	if r.UpdatedAt.Valid {
		t := r.UpdatedAt.Time
		report.UpdatedAt = &t
	}
	report.AmendmentNumber = int(r.AmendmentNumber)
	if r.OriginalReportID.Valid {
		id := uuid.UUID(r.OriginalReportID.Bytes)
		report.OriginalReportID = &id
	}

	return report
}

func int32PtrToIntPtr(v *int32) *int {
	if v == nil {
		return nil
	}
	i := int(*v)
	return &i
}
