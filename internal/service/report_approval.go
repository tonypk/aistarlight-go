package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// ApprovalEntry represents a single approval/transition record.
type ApprovalEntry struct {
	ID         string  `json:"id"`
	ReportID   string  `json:"report_id"`
	UserID     string  `json:"user_id"`
	UserName   string  `json:"user_name"`
	UserEmail  string  `json:"user_email"`
	FromStatus string  `json:"from_status"`
	ToStatus   string  `json:"to_status"`
	Action     string  `json:"action"`
	Comment    *string `json:"comment"`
	CreatedAt  string  `json:"created_at"`
}

// ReportApprovalService manages the approval audit trail.
type ReportApprovalService struct {
	q *sqlc.Queries
}

// NewReportApprovalService creates a ReportApprovalService.
func NewReportApprovalService(q *sqlc.Queries) *ReportApprovalService {
	return &ReportApprovalService{q: q}
}

// RecordTransition logs a status transition with optional comment.
func (s *ReportApprovalService) RecordTransition(ctx context.Context, reportID, userID uuid.UUID, fromStatus, toStatus domain.ReportStatus, comment *string) error {
	action := deriveAction(fromStatus, toStatus)

	_, err := s.q.CreateReportApproval(ctx, sqlc.CreateReportApprovalParams{
		ID:         uuid.New(),
		ReportID:   reportID,
		UserID:     userID,
		FromStatus: string(fromStatus),
		ToStatus:   string(toStatus),
		Action:     action,
		Comment:    comment,
	})
	if err != nil {
		return fmt.Errorf("record approval: %w", err)
	}
	return nil
}

// ListApprovals returns the approval history for a report.
func (s *ReportApprovalService) ListApprovals(ctx context.Context, reportID uuid.UUID) ([]ApprovalEntry, error) {
	rows, err := s.q.ListReportApprovals(ctx, reportID)
	if err != nil {
		return nil, fmt.Errorf("list approvals: %w", err)
	}

	entries := make([]ApprovalEntry, len(rows))
	for i, r := range rows {
		userName := ""
		if r.UserName != nil {
			userName = *r.UserName
		}
		entries[i] = ApprovalEntry{
			ID:         r.ID.String(),
			ReportID:   r.ReportID.String(),
			UserID:     r.UserID.String(),
			UserName:   userName,
			UserEmail:  r.UserEmail,
			FromStatus: r.FromStatus,
			ToStatus:   r.ToStatus,
			Action:     r.Action,
			Comment:    r.Comment,
			CreatedAt:  r.CreatedAt.Format(time.RFC3339),
		}
	}
	return entries, nil
}

// deriveAction maps a status transition to a human-readable action name.
func deriveAction(from, to domain.ReportStatus) string {
	switch {
	case to == domain.StatusReview && from == domain.StatusDraft:
		return "submit"
	case to == domain.StatusApproved:
		return "approve"
	case to == domain.StatusRejected:
		return "reject"
	case to == domain.StatusDraft && from == domain.StatusReview:
		return "return"
	case to == domain.StatusFiled:
		return "file"
	case to == domain.StatusArchived:
		return "archive"
	default:
		return "transition"
	}
}
