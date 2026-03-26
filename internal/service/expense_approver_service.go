package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

var (
	ErrApproverNotFound   = errors.New("expense approver not found")
	ErrNoEligibleApprover = errors.New("no eligible approver found")
)

type ExpenseApproverService struct {
	q *sqlc.Queries
}

func NewExpenseApproverService(q *sqlc.Queries) *ExpenseApproverService {
	return &ExpenseApproverService{q: q}
}

type CreateApproverInput struct {
	CompanyID      uuid.UUID
	DepartmentName string
	ApproverUserID uuid.UUID
	MaxAmount      *decimal.Decimal
	Priority       int
}

type UpdateApproverInput struct {
	DepartmentName string
	MaxAmount      *decimal.Decimal
	Priority       int
}

// Create registers a new expense approver for a department.
func (s *ExpenseApproverService) Create(ctx context.Context, input CreateApproverInput) (*domain.ExpenseApprover, error) {
	row, err := s.q.CreateExpenseApprover(ctx, sqlc.CreateExpenseApproverParams{
		ID:             uuid.New(),
		CompanyID:      input.CompanyID,
		DepartmentName: input.DepartmentName,
		ApproverUserID: input.ApproverUserID,
		MaxAmount:      decimalToNumeric(input.MaxAmount),
		Priority:       int32(input.Priority),
		IsActive:       true,
	})
	if err != nil {
		return nil, fmt.Errorf("create expense approver: %w", err)
	}

	return toExpenseApprover(row), nil
}

// GetByID retrieves an expense approver by ID scoped to the company.
func (s *ExpenseApproverService) GetByID(ctx context.Context, id, companyID uuid.UUID) (*domain.ExpenseApprover, error) {
	row, err := s.q.GetExpenseApproverByID(ctx, sqlc.GetExpenseApproverByIDParams{
		ID:        id,
		CompanyID: companyID,
	})
	if err != nil {
		return nil, ErrApproverNotFound
	}

	return &domain.ExpenseApprover{
		ID:             row.ID,
		CompanyID:      row.CompanyID,
		DepartmentName: row.DepartmentName,
		ApproverUserID: row.ApproverUserID,
		MaxAmount:      numericToPtrDecimal(row.MaxAmount),
		Priority:       int(row.Priority),
		IsActive:       row.IsActive,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		ApproverEmail:  row.ApproverEmail,
		ApproverName:   row.ApproverName,
	}, nil
}

// List returns all active expense approvers for a company.
func (s *ExpenseApproverService) List(ctx context.Context, companyID uuid.UUID) ([]domain.ExpenseApprover, error) {
	rows, err := s.q.ListExpenseApprovers(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("list expense approvers: %w", err)
	}

	result := make([]domain.ExpenseApprover, len(rows))
	for i, row := range rows {
		result[i] = domain.ExpenseApprover{
			ID:             row.ID,
			CompanyID:      row.CompanyID,
			DepartmentName: row.DepartmentName,
			ApproverUserID: row.ApproverUserID,
			MaxAmount:      numericToPtrDecimal(row.MaxAmount),
			Priority:       int(row.Priority),
			IsActive:       row.IsActive,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
			ApproverEmail:  row.ApproverEmail,
			ApproverName:   row.ApproverName,
		}
	}
	return result, nil
}

// Update modifies an existing expense approver's configuration.
func (s *ExpenseApproverService) Update(ctx context.Context, id, companyID uuid.UUID, input UpdateApproverInput) error {
	err := s.q.UpdateExpenseApprover(ctx, sqlc.UpdateExpenseApproverParams{
		ID:             id,
		CompanyID:      companyID,
		DepartmentName: input.DepartmentName,
		MaxAmount:      decimalToNumeric(input.MaxAmount),
		Priority:       int32(input.Priority),
	})
	if err != nil {
		return fmt.Errorf("update expense approver: %w", err)
	}
	return nil
}

// Deactivate soft-deletes an expense approver by setting is_active = false.
func (s *ExpenseApproverService) Deactivate(ctx context.Context, id, companyID uuid.UUID) error {
	err := s.q.DeactivateExpenseApprover(ctx, sqlc.DeactivateExpenseApproverParams{
		ID:        id,
		CompanyID: companyID,
	})
	if err != nil {
		return fmt.Errorf("deactivate expense approver: %w", err)
	}
	return nil
}

// ResolveApprover finds the most appropriate approver for an expense report submission.
// It first looks for configured department approvers (excluding self), then falls back
// to company admins (also excluding self). Returns nil, nil if no approver is found,
// which signals the report should wait in pending_approval status.
func (s *ExpenseApproverService) ResolveApprover(ctx context.Context, companyID, submitterUserID uuid.UUID, department string, amount decimal.Decimal) (*uuid.UUID, error) {
	// Convert amount to pgtype.Numeric for the query
	amountNum := decimalToNumeric(&amount)

	// Step 1: find configured department approvers who can handle this amount
	eligible, err := s.q.FindEligibleApprovers(ctx, sqlc.FindEligibleApproversParams{
		CompanyID:      companyID,
		DepartmentName: department,
		ApproverUserID: submitterUserID, // excludes self
		MaxAmount:      amountNum,
	})
	if err != nil {
		return nil, fmt.Errorf("find eligible approvers: %w", err)
	}

	if len(eligible) > 0 {
		// Already sorted by priority ASC — take the first
		id := eligible[0].ApproverUserID
		return &id, nil
	}

	// Step 2: fall back to company admins (also excluding self)
	admins, err := s.q.FindAdminApprovers(ctx, sqlc.FindAdminApproversParams{
		CompanyID: companyID,
		UserID:    submitterUserID, // excludes self
	})
	if err != nil {
		return nil, fmt.Errorf("find admin approvers: %w", err)
	}

	if len(admins) > 0 {
		id := admins[0].ApproverUserID
		return &id, nil
	}

	// No approver found — report will hold in pending_approval
	return nil, nil
}

// -- Conversion helper --

func toExpenseApprover(a sqlc.ExpenseApprover) *domain.ExpenseApprover {
	return &domain.ExpenseApprover{
		ID:             a.ID,
		CompanyID:      a.CompanyID,
		DepartmentName: a.DepartmentName,
		ApproverUserID: a.ApproverUserID,
		MaxAmount:      numericToPtrDecimal(a.MaxAmount),
		Priority:       int(a.Priority),
		IsActive:       a.IsActive,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}
}
