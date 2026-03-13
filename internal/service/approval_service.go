package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// ApprovalService evaluates approval rules and manages receipt approval workflow.
type ApprovalService struct {
	q *sqlc.Queries
}

// NewApprovalService creates an ApprovalService.
func NewApprovalService(q *sqlc.Queries) *ApprovalService {
	return &ApprovalService{q: q}
}

// ApprovalCheckResult describes whether a receipt needs approval and why.
type ApprovalCheckResult struct {
	NeedsApproval bool   `json:"needs_approval"`
	TriggerReason string `json:"trigger_reason"` // amount_threshold, new_vendor, risk_flag, none
}

// EvaluateApproval checks whether a receipt batch requires approval based on company settings.
func (s *ApprovalService) EvaluateApproval(ctx context.Context, companyID uuid.UUID, amount float64, vendorName string, isDuplicate bool) (ApprovalCheckResult, error) {
	settings, err := s.q.GetApprovalSettings(ctx, companyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ApprovalCheckResult{NeedsApproval: false, TriggerReason: "none"}, nil
		}
		return ApprovalCheckResult{}, fmt.Errorf("get approval settings: %w", err)
	}

	if !settings.IsEnabled {
		return ApprovalCheckResult{NeedsApproval: false, TriggerReason: "none"}, nil
	}

	// Check risk flags (duplicate).
	if isDuplicate && settings.RiskFlagsRequireApproval {
		return ApprovalCheckResult{NeedsApproval: true, TriggerReason: "risk_flag"}, nil
	}

	// Check amount threshold.
	if settings.AmountThreshold.Valid {
		threshold, _ := numericToFloat64(settings.AmountThreshold)
		if amount > threshold {
			return ApprovalCheckResult{NeedsApproval: true, TriggerReason: "amount_threshold"}, nil
		}
	}

	// Check new vendor (first N receipts).
	if settings.NewVendorReceipts != nil && *settings.NewVendorReceipts > 0 && vendorName != "" {
		count, cErr := s.q.CountVendorReceiptBatches(ctx, sqlc.CountVendorReceiptBatchesParams{
			CompanyID: companyID,
			Column2:   &vendorName,
		})
		if cErr != nil {
			return ApprovalCheckResult{}, fmt.Errorf("count vendor batches: %w", cErr)
		}
		if count < int64(*settings.NewVendorReceipts) {
			return ApprovalCheckResult{NeedsApproval: true, TriggerReason: "new_vendor"}, nil
		}
	}

	return ApprovalCheckResult{NeedsApproval: false, TriggerReason: "none"}, nil
}

// CreateApproval creates a new pending approval for a receipt batch.
func (s *ApprovalService) CreateApproval(ctx context.Context, batchID, companyID, requestedBy uuid.UUID, triggerReason string) (*sqlc.ReceiptApproval, error) {
	approval, err := s.q.CreateReceiptApproval(ctx, sqlc.CreateReceiptApprovalParams{
		BatchID:       batchID,
		CompanyID:     companyID,
		TriggerReason: triggerReason,
		RequestedBy:   pgtype.UUID{Bytes: requestedBy, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("create approval: %w", err)
	}

	// Update batch approval status.
	pending := "pending"
	if err := s.q.UpdateBatchApprovalStatus(ctx, sqlc.UpdateBatchApprovalStatusParams{
		ID:             batchID,
		ApprovalStatus: &pending,
	}); err != nil {
		return nil, fmt.Errorf("update batch approval status: %w", err)
	}

	return &approval, nil
}

// Approve approves a pending receipt approval.
func (s *ApprovalService) Approve(ctx context.Context, approvalID, approverID uuid.UUID, notes *string) (*sqlc.ReceiptApproval, error) {
	approval, err := s.q.UpdateReceiptApprovalStatus(ctx, sqlc.UpdateReceiptApprovalStatusParams{
		ID:         approvalID,
		Status:     "approved",
		ApprovedBy: pgtype.UUID{Bytes: approverID, Valid: true},
		Notes:      notes,
	})
	if err != nil {
		return nil, fmt.Errorf("approve: %w", err)
	}

	approved := "approved"
	_ = s.q.UpdateBatchApprovalStatus(ctx, sqlc.UpdateBatchApprovalStatusParams{
		ID:             approval.BatchID,
		ApprovalStatus: &approved,
	})

	return &approval, nil
}

// Reject rejects a pending receipt approval.
func (s *ApprovalService) Reject(ctx context.Context, approvalID, approverID uuid.UUID, notes *string) (*sqlc.ReceiptApproval, error) {
	approval, err := s.q.UpdateReceiptApprovalStatus(ctx, sqlc.UpdateReceiptApprovalStatusParams{
		ID:         approvalID,
		Status:     "rejected",
		ApprovedBy: pgtype.UUID{Bytes: approverID, Valid: true},
		Notes:      notes,
	})
	if err != nil {
		return nil, fmt.Errorf("reject: %w", err)
	}

	rejected := "rejected"
	_ = s.q.UpdateBatchApprovalStatus(ctx, sqlc.UpdateBatchApprovalStatusParams{
		ID:             approval.BatchID,
		ApprovalStatus: &rejected,
	})

	return &approval, nil
}

// GetSettings returns the approval settings for a company.
func (s *ApprovalService) GetSettings(ctx context.Context, companyID uuid.UUID) (*sqlc.CompanyApprovalSetting, error) {
	settings, err := s.q.GetApprovalSettings(ctx, companyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &settings, nil
}

// numericToFloat64 converts pgtype.Numeric to float64.
func numericToFloat64(n pgtype.Numeric) (float64, bool) {
	if !n.Valid {
		return 0, false
	}
	if n.Int == nil {
		return 0, false
	}
	f := new(big.Float).SetInt(n.Int)
	if n.Exp < 0 {
		divisor := new(big.Float).SetFloat64(math.Pow10(-int(n.Exp)))
		f.Quo(f, divisor)
	} else if n.Exp > 0 {
		multiplier := new(big.Float).SetFloat64(math.Pow10(int(n.Exp)))
		f.Mul(f, multiplier)
	}
	result, _ := f.Float64()
	return result, true
}
