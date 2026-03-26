package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

var (
	ErrPolicyNotFound  = errors.New("expense policy not found")
	ErrInvalidCategory = errors.New("invalid expense category")
)

type ExpensePolicyService struct {
	q *sqlc.Queries
}

func NewExpensePolicyService(q *sqlc.Queries) *ExpensePolicyService {
	return &ExpensePolicyService{q: q}
}

type CreatePolicyInput struct {
	CompanyID            uuid.UUID
	Name                 string
	Category             string
	MaxAmount            *decimal.Decimal
	RequiresReceiptAbove *decimal.Decimal
	AutoApproveBelow     *decimal.Decimal
	AIAutoApprove        bool
	Description          *string
}

type UpdatePolicyInput struct {
	Name                 string
	Category             string
	MaxAmount            *decimal.Decimal
	RequiresReceiptAbove *decimal.Decimal
	AutoApproveBelow     *decimal.Decimal
	AIAutoApprove        bool
	Description          *string
}

// Create validates the category and creates a new expense policy.
func (s *ExpensePolicyService) Create(ctx context.Context, input CreatePolicyInput) (*domain.ExpensePolicy, error) {
	if !domain.ValidCategories[input.Category] {
		return nil, fmt.Errorf("%w: %s", ErrInvalidCategory, input.Category)
	}

	row, err := s.q.CreateExpensePolicy(ctx, sqlc.CreateExpensePolicyParams{
		ID:                   uuid.New(),
		CompanyID:            input.CompanyID,
		Name:                 input.Name,
		Category:             input.Category,
		MaxAmount:            decimalToNumeric(input.MaxAmount),
		RequiresReceiptAbove: decimalToNumeric(input.RequiresReceiptAbove),
		AutoApproveBelow:     decimalToNumeric(input.AutoApproveBelow),
		AiAutoApprove:        input.AIAutoApprove,
		Description:          input.Description,
		IsActive:             true,
	})
	if err != nil {
		return nil, fmt.Errorf("create expense policy: %w", err)
	}

	return toExpensePolicy(row), nil
}

// GetByID retrieves an expense policy by ID scoped to the company.
func (s *ExpensePolicyService) GetByID(ctx context.Context, id, companyID uuid.UUID) (*domain.ExpensePolicy, error) {
	row, err := s.q.GetExpensePolicyByID(ctx, sqlc.GetExpensePolicyByIDParams{
		ID:        id,
		CompanyID: companyID,
	})
	if err != nil {
		return nil, ErrPolicyNotFound
	}

	return toExpensePolicy(row), nil
}

// List returns all active expense policies for a company.
func (s *ExpensePolicyService) List(ctx context.Context, companyID uuid.UUID) ([]domain.ExpensePolicy, error) {
	rows, err := s.q.ListExpensePolicies(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("list expense policies: %w", err)
	}

	result := make([]domain.ExpensePolicy, len(rows))
	for i, row := range rows {
		result[i] = *toExpensePolicy(row)
	}
	return result, nil
}

// Update modifies an existing expense policy's fields.
func (s *ExpensePolicyService) Update(ctx context.Context, id, companyID uuid.UUID, input UpdatePolicyInput) error {
	if !domain.ValidCategories[input.Category] {
		return fmt.Errorf("%w: %s", ErrInvalidCategory, input.Category)
	}

	err := s.q.UpdateExpensePolicy(ctx, sqlc.UpdateExpensePolicyParams{
		ID:                   id,
		CompanyID:            companyID,
		Name:                 input.Name,
		Category:             input.Category,
		MaxAmount:            decimalToNumeric(input.MaxAmount),
		RequiresReceiptAbove: decimalToNumeric(input.RequiresReceiptAbove),
		AutoApproveBelow:     decimalToNumeric(input.AutoApproveBelow),
		AiAutoApprove:        input.AIAutoApprove,
		Description:          input.Description,
	})
	if err != nil {
		return fmt.Errorf("update expense policy: %w", err)
	}
	return nil
}

// Deactivate soft-deletes an expense policy by setting is_active = false.
func (s *ExpensePolicyService) Deactivate(ctx context.Context, id, companyID uuid.UUID) error {
	err := s.q.DeactivateExpensePolicy(ctx, sqlc.DeactivateExpensePolicyParams{
		ID:        id,
		CompanyID: companyID,
	})
	if err != nil {
		return fmt.Errorf("deactivate expense policy: %w", err)
	}
	return nil
}

// ListByCategory returns active expense policies for a company filtered by category.
func (s *ExpensePolicyService) ListByCategory(ctx context.Context, companyID uuid.UUID, category string) ([]domain.ExpensePolicy, error) {
	rows, err := s.q.ListExpensePoliciesByCategory(ctx, sqlc.ListExpensePoliciesByCategoryParams{
		CompanyID: companyID,
		Category:  category,
	})
	if err != nil {
		return nil, fmt.Errorf("list expense policies by category: %w", err)
	}

	result := make([]domain.ExpensePolicy, len(rows))
	for i, row := range rows {
		result[i] = *toExpensePolicy(row)
	}
	return result, nil
}

// -- Conversion helpers --

func toExpensePolicy(p sqlc.ExpensePolicy) *domain.ExpensePolicy {
	return &domain.ExpensePolicy{
		ID:                   p.ID,
		CompanyID:            p.CompanyID,
		Name:                 p.Name,
		Category:             p.Category,
		MaxAmount:            numericToPtrDecimal(p.MaxAmount),
		RequiresReceiptAbove: numericToPtrDecimal(p.RequiresReceiptAbove),
		AutoApproveBelow:     numericToPtrDecimal(p.AutoApproveBelow),
		AIAutoApprove:        p.AiAutoApprove,
		Description:          p.Description,
		IsActive:             p.IsActive,
		CreatedAt:            p.CreatedAt,
		UpdatedAt:            p.UpdatedAt,
	}
}

// decimalToNumeric converts *decimal.Decimal to pgtype.Numeric (NULL when nil).
func decimalToNumeric(d *decimal.Decimal) pgtype.Numeric {
	if d == nil {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

// numericToPtrDecimal converts pgtype.Numeric to *decimal.Decimal (nil when not valid).
func numericToPtrDecimal(n pgtype.Numeric) *decimal.Decimal {
	if !n.Valid {
		return nil
	}
	d := decimal.NewFromBigInt(n.Int, n.Exp)
	return &d
}
