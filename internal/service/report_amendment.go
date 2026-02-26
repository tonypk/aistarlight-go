package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// AmendReport creates an amended version of a filed report.
// The original report remains unchanged; a new draft report is created with
// amendment_number incremented and original_report_id pointing to the root.
func (s *ReportService) AmendReport(ctx context.Context, reportID, userID uuid.UUID) (*domain.Report, error) {
	report, err := s.q.GetReportByID(ctx, reportID)
	if err != nil {
		return nil, ErrReportNotFound
	}

	if domain.ReportStatus(report.Status) != domain.StatusFiled {
		return nil, fmt.Errorf("only filed reports can be amended (current status: %s)", report.Status)
	}

	// Determine the root report ID (original chain head)
	rootID := reportID
	if report.OriginalReportID.Valid {
		rootID = uuid.UUID(report.OriginalReportID.Bytes)
	}

	// Get current max amendment number
	maxAmendment, err := s.q.GetMaxAmendmentNumber(ctx, rootID)
	if err != nil {
		maxAmendment = 0
	}

	newID := uuid.New()
	newAmendment := int32(maxAmendment + 1)

	dbReport, err := s.q.CreateReportWithAmendment(ctx, sqlc.CreateReportWithAmendmentParams{
		ID:               newID,
		CompanyID:        report.CompanyID,
		ReportType:       report.ReportType,
		Period:           report.Period,
		Status:           string(domain.StatusDraft),
		InputData:        report.InputData,
		CalculatedData:   report.CalculatedData,
		CreatedBy:        pgtype.UUID{Bytes: userID, Valid: true},
		AmendmentNumber:  newAmendment,
		OriginalReportID: pgtype.UUID{Bytes: rootID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("create amended report: %w", err)
	}

	return toReport(dbReport), nil
}

// GetAmendmentChain returns all versions of a report (original + amendments).
func (s *ReportService) GetAmendmentChain(ctx context.Context, reportID uuid.UUID) ([]domain.Report, error) {
	// First determine the root
	report, err := s.q.GetReportByID(ctx, reportID)
	if err != nil {
		return nil, ErrReportNotFound
	}

	rootID := reportID
	if report.OriginalReportID.Valid {
		rootID = uuid.UUID(report.OriginalReportID.Bytes)
	}

	rows, err := s.q.ListAmendmentChain(ctx, rootID)
	if err != nil {
		return nil, fmt.Errorf("list amendment chain: %w", err)
	}

	result := make([]domain.Report, len(rows))
	for i, r := range rows {
		result[i] = *toReport(r)
	}
	return result, nil
}
