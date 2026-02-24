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
	q *sqlc.Queries
}

func NewReportService(q *sqlc.Queries) *ReportService {
	return &ReportService{q: q}
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

// UpdateStatus transitions a report to a new status.
func (s *ReportService) UpdateStatus(ctx context.Context, id uuid.UUID, newStatus domain.ReportStatus, userID uuid.UUID) (*domain.Report, error) {
	report, err := s.q.GetReportByID(ctx, id)
	if err != nil {
		return nil, ErrReportNotFound
	}

	current := domain.ReportStatus(report.Status)
	if !domain.IsValidTransition(current, newStatus) {
		return nil, fmt.Errorf("invalid transition from %s to %s", current, newStatus)
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

	return s.GetByID(ctx, id)
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

	return report
}

func int32PtrToIntPtr(v *int32) *int {
	if v == nil {
		return nil
	}
	i := int(*v)
	return &i
}
