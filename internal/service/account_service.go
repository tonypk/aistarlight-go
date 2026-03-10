package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/coa"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

var (
	ErrAccountNotFound     = errors.New("account not found")
	ErrAccountNumberExists = errors.New("account number already exists")
	ErrSystemAccount       = errors.New("cannot delete system account")
)

type AccountService struct {
	q *sqlc.Queries
}

func NewAccountService(q *sqlc.Queries) *AccountService {
	return &AccountService{q: q}
}

type CreateAccountInput struct {
	CompanyID     uuid.UUID
	AccountNumber string
	Name          string
	AccountType   domain.AccountType
	SubType       *string
	ParentID      *uuid.UUID
	Description   *string
	NormalBalance domain.NormalBalance
	QBOAccountID  *string
}

func (s *AccountService) Create(ctx context.Context, input CreateAccountInput) (*domain.Account, error) {
	// Check for duplicate account number
	_, err := s.q.GetAccountByNumber(ctx, sqlc.GetAccountByNumberParams{
		CompanyID:     input.CompanyID,
		AccountNumber: input.AccountNumber,
	})
	if err == nil {
		return nil, ErrAccountNumberExists
	}

	parentID := pgtype.UUID{}
	if input.ParentID != nil {
		parentID = pgtype.UUID{Bytes: *input.ParentID, Valid: true}
	}

	dbAcct, err := s.q.CreateAccount(ctx, sqlc.CreateAccountParams{
		ID:            uuid.New(),
		CompanyID:     input.CompanyID,
		AccountNumber: input.AccountNumber,
		Name:          input.Name,
		AccountType:   string(input.AccountType),
		SubType:       input.SubType,
		ParentID:      parentID,
		Description:   input.Description,
		IsActive:      true,
		IsSystem:      false,
		NormalBalance: string(input.NormalBalance),
		QboAccountID:  input.QBOAccountID,
	})
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	return toAccount(dbAcct), nil
}

func (s *AccountService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	dbAcct, err := s.q.GetAccountByID(ctx, id)
	if err != nil {
		return nil, ErrAccountNotFound
	}
	return toAccount(dbAcct), nil
}

func (s *AccountService) List(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.Account, int64, error) {
	accounts, err := s.q.ListAccountsByCompany(ctx, sqlc.ListAccountsByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(p.Limit),
		Offset:    int32(p.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list accounts: %w", err)
	}

	count, err := s.q.CountAccountsByCompany(ctx, companyID)
	if err != nil {
		return nil, 0, fmt.Errorf("count accounts: %w", err)
	}

	result := make([]domain.Account, len(accounts))
	for i, a := range accounts {
		result[i] = *toAccount(a)
	}
	return result, count, nil
}

func (s *AccountService) Update(ctx context.Context, id uuid.UUID, name *string, subType *string, desc *string, isActive *bool, qboID *string) error {
	n := ""
	if name != nil {
		n = *name
	}
	active := true
	if isActive != nil {
		active = *isActive
	}
	err := s.q.UpdateAccount(ctx, sqlc.UpdateAccountParams{
		ID:           id,
		Name:         n,
		SubType:      subType,
		Description:  desc,
		IsActive:     active,
		QboAccountID: qboID,
	})
	if err != nil {
		return fmt.Errorf("update account: %w", err)
	}
	return nil
}

func (s *AccountService) Delete(ctx context.Context, id uuid.UUID) error {
	acct, err := s.q.GetAccountByID(ctx, id)
	if err != nil {
		return ErrAccountNotFound
	}
	if acct.IsSystem {
		return ErrSystemAccount
	}
	return s.q.DeleteAccount(ctx, id)
}

// Seed creates the standard COA for a company based on jurisdiction.
func (s *AccountService) Seed(ctx context.Context, companyID uuid.UUID, jurisdiction string) (int, error) {
	var templates []coa.AccountTemplate
	switch jurisdiction {
	case "SG":
		templates = coa.SGStandardCOA()
	case "LK":
		templates = coa.LKStandardCOA()
	default:
		templates = coa.PHStandardCOA()
	}
	created := 0

	// Collect template account numbers for orphan detection
	templateNumbers := make([]string, len(templates))
	for i, t := range templates {
		templateNumbers[i] = t.Number
	}

	for _, t := range templates {
		// Check if already exists
		existing, err := s.q.GetAccountByNumber(ctx, sqlc.GetAccountByNumberParams{
			CompanyID:     companyID,
			AccountNumber: t.Number,
		})
		if err == nil {
			// Update existing system account if name/description changed (e.g. PH→SG re-seed)
			if existing.IsSystem && (existing.Name != t.Name || ptrStr(existing.Description) != t.Description) {
				_ = s.q.UpdateAccount(ctx, sqlc.UpdateAccountParams{
					ID:          existing.ID,
					Name:        t.Name,
					SubType:     &t.SubType,
					Description: &t.Description,
					IsActive:    true,
				})
				created++
			}
			continue
		}

		_, err = s.q.CreateAccount(ctx, sqlc.CreateAccountParams{
			ID:            uuid.New(),
			CompanyID:     companyID,
			AccountNumber: t.Number,
			Name:          t.Name,
			AccountType:   t.Type,
			SubType:       &t.SubType,
			Description:   &t.Description,
			IsActive:      true,
			IsSystem:      true,
			NormalBalance: t.Normal,
		})
		if err != nil {
			return created, fmt.Errorf("seed account %s: %w", t.Number, err)
		}
		created++
	}

	// Deactivate orphaned system accounts from previous jurisdiction seed
	_ = s.q.DeactivateSystemAccountsNotIn(ctx, sqlc.DeactivateSystemAccountsNotInParams{
		CompanyID: companyID,
		Column2:   templateNumbers,
	})

	return created, nil
}

// GetBalance returns the balance of an account up to a given date.
func (s *AccountService) GetBalance(ctx context.Context, accountID uuid.UUID, asOfDate pgtype.Date) (*sqlc.AccountBalanceRow, error) {
	row, err := s.q.AccountBalance(ctx, sqlc.AccountBalanceParams{
		ID:        accountID,
		EntryDate: asOfDate,
	})
	if err != nil {
		return nil, fmt.Errorf("account balance: %w", err)
	}
	return &row, nil
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toAccount(a sqlc.Account) *domain.Account {
	acct := &domain.Account{
		ID:            a.ID,
		CompanyID:     a.CompanyID,
		AccountNumber: a.AccountNumber,
		Name:          a.Name,
		AccountType:   domain.AccountType(a.AccountType),
		SubType:       a.SubType,
		Description:   a.Description,
		IsActive:      a.IsActive,
		IsSystem:      a.IsSystem,
		NormalBalance: domain.NormalBalance(a.NormalBalance),
		QBOAccountID:  a.QboAccountID,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
	if a.ParentID.Valid {
		id := uuid.UUID(a.ParentID.Bytes)
		acct.ParentID = &id
	}
	return acct
}
