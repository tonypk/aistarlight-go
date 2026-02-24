package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

var (
	ErrCompanyLimitReached = errors.New("organization has reached max companies")
)

type CompanyService struct {
	q *sqlc.Queries
}

func NewCompanyService(q *sqlc.Queries) *CompanyService {
	return &CompanyService{q: q}
}

type CreateCompanyInput struct {
	OrgID             *uuid.UUID
	CompanyName       string
	TINNumber         *string
	RDOCode           *string
	VATClassification string
	FiscalYearEnd     string
	Industry          *string
	Address           *string
	CreatedByUserID   uuid.UUID
}

func (s *CompanyService) Create(ctx context.Context, input CreateCompanyInput) (*domain.Company, error) {
	// Check org limit if under an org
	if input.OrgID != nil {
		org, err := s.q.GetOrganizationByID(ctx, *input.OrgID)
		if err != nil {
			return nil, ErrOrgNotFound
		}

		count, _ := s.q.CountCompaniesByOrg(ctx, pgtype.UUID{Bytes: *input.OrgID, Valid: true})
		if count >= int64(org.MaxCompanies) {
			return nil, ErrCompanyLimitReached
		}
	}

	vatClass := input.VATClassification
	if vatClass == "" {
		vatClass = "vat_registered"
	}
	fyEnd := input.FiscalYearEnd
	if fyEnd == "" {
		fyEnd = "12-31"
	}

	companyID := uuid.New()
	var orgID pgtype.UUID
	if input.OrgID != nil {
		orgID = pgtype.UUID{Bytes: *input.OrgID, Valid: true}
	}

	dbCompany, err := s.q.CreateCompany(ctx, sqlc.CreateCompanyParams{
		ID:                companyID,
		OrganizationID:    orgID,
		CompanyName:       input.CompanyName,
		TinNumber:         input.TINNumber,
		RdoCode:           input.RDOCode,
		VatClassification: vatClass,
		FiscalYearEnd:     fyEnd,
		Industry:          input.Industry,
		Address:           input.Address,
		Plan:              "free",
		Settings:          []byte("{}"),
		IsActive:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("create company: %w", err)
	}

	// If not under an org, add creator as company_admin
	if input.OrgID == nil {
		err = s.q.AddCompanyMember(ctx, sqlc.AddCompanyMemberParams{
			CompanyID: companyID,
			UserID:    input.CreatedByUserID,
			Role:      string(domain.CompanyRoleAdmin),
		})
		if err != nil {
			return nil, fmt.Errorf("add company member: %w", err)
		}
	}

	return toCompany(dbCompany), nil
}

func (s *CompanyService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Company, error) {
	dbCompany, err := s.q.GetCompanyByID(ctx, id)
	if err != nil {
		return nil, ErrCompanyNotFound
	}
	return toCompany(dbCompany), nil
}

func (s *CompanyService) Update(ctx context.Context, id uuid.UUID, input CreateCompanyInput) (*domain.Company, error) {
	existing, err := s.q.GetCompanyByID(ctx, id)
	if err != nil {
		return nil, ErrCompanyNotFound
	}

	name := input.CompanyName
	if name == "" {
		name = existing.CompanyName
	}
	tin := input.TINNumber
	if tin == nil {
		tin = existing.TinNumber
	}
	rdo := input.RDOCode
	if rdo == nil {
		rdo = existing.RdoCode
	}
	vat := input.VATClassification
	if vat == "" {
		vat = existing.VatClassification
	}
	fy := input.FiscalYearEnd
	if fy == "" {
		fy = existing.FiscalYearEnd
	}

	err = s.q.UpdateCompany(ctx, sqlc.UpdateCompanyParams{
		ID:                id,
		CompanyName:       name,
		TinNumber:         tin,
		RdoCode:           rdo,
		VatClassification: vat,
		FiscalYearEnd:     fy,
		Industry:          input.Industry,
		Address:           input.Address,
		Plan:              existing.Plan,
		Settings:          existing.Settings,
		IsActive:          existing.IsActive,
	})
	if err != nil {
		return nil, fmt.Errorf("update company: %w", err)
	}

	return s.GetByID(ctx, id)
}

func (s *CompanyService) ListByOrg(ctx context.Context, orgID uuid.UUID, p pagination.Params) ([]domain.Company, int, error) {
	rows, err := s.q.ListCompaniesByOrg(ctx, sqlc.ListCompaniesByOrgParams{
		OrganizationID: pgtype.UUID{Bytes: orgID, Valid: true},
		Limit:          int32(p.Limit),
		Offset:         int32(p.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list companies: %w", err)
	}

	count, _ := s.q.CountCompaniesByOrg(ctx, pgtype.UUID{Bytes: orgID, Valid: true})

	result := make([]domain.Company, len(rows))
	for i, r := range rows {
		result[i] = *toCompany(r)
	}
	return result, int(count), nil
}

func (s *CompanyService) ListByUser(ctx context.Context, userID uuid.UUID, p pagination.Params) ([]domain.Company, int, error) {
	rows, err := s.q.ListCompaniesByUser(ctx, sqlc.ListCompaniesByUserParams{
		UserID: userID,
		Limit:  int32(p.Limit),
		Offset: int32(p.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list companies: %w", err)
	}

	count, _ := s.q.CountCompaniesByUser(ctx, userID)

	result := make([]domain.Company, len(rows))
	for i, r := range rows {
		result[i] = *toCompany(r)
	}
	return result, int(count), nil
}

func (s *CompanyService) AddMember(ctx context.Context, companyID, userID uuid.UUID, role domain.CompanyRole) error {
	return s.q.AddCompanyMember(ctx, sqlc.AddCompanyMemberParams{
		CompanyID: companyID,
		UserID:    userID,
		Role:      string(role),
	})
}

func (s *CompanyService) RemoveMember(ctx context.Context, companyID, userID uuid.UUID) error {
	return s.q.RemoveCompanyMember(ctx, sqlc.RemoveCompanyMemberParams{
		CompanyID: companyID,
		UserID:    userID,
	})
}

func (s *CompanyService) UpdateMemberRole(ctx context.Context, companyID, userID uuid.UUID, role domain.CompanyRole) error {
	return s.q.UpdateCompanyMemberRole(ctx, sqlc.UpdateCompanyMemberRoleParams{
		CompanyID: companyID,
		UserID:    userID,
		Role:      string(role),
	})
}

func (s *CompanyService) ListMembers(ctx context.Context, companyID uuid.UUID) ([]domain.CompanyMember, error) {
	rows, err := s.q.ListCompanyMembers(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}

	members := make([]domain.CompanyMember, len(rows))
	for i, r := range rows {
		members[i] = domain.CompanyMember{
			CompanyID: r.CompanyID,
			UserID:    r.UserID,
			Role:      domain.CompanyRole(r.Role),
			JoinedAt:  r.JoinedAt,
			Email:     r.Email,
			FullName:  r.FullName,
		}
	}
	return members, nil
}

// GetEffectiveRole implements middleware.AccessChecker.
func (s *CompanyService) GetEffectiveRole(ctx context.Context, userID, companyID uuid.UUID) (domain.CompanyRole, error) {
	roleVal, err := s.q.GetEffectiveRole(ctx, sqlc.GetEffectiveRoleParams{
		UserID:    userID,
		CompanyID: companyID,
	})
	if err != nil || roleVal == nil {
		return "", ErrNoAccess
	}
	role, ok := roleVal.(string)
	if !ok {
		return "", ErrNoAccess
	}
	return domain.CompanyRole(role), nil
}

func toCompany(c sqlc.Company) *domain.Company {
	company := &domain.Company{
		ID:                c.ID,
		CompanyName:       c.CompanyName,
		TINNumber:         c.TinNumber,
		RDOCode:           c.RdoCode,
		VATClassification: c.VatClassification,
		FiscalYearEnd:     c.FiscalYearEnd,
		Industry:          c.Industry,
		Address:           c.Address,
		Plan:              c.Plan,
		Settings:          domain.JSON(c.Settings),
		IsActive:          c.IsActive,
		CreatedAt:         c.CreatedAt,
		UpdatedAt:         c.UpdatedAt,
	}
	if c.OrganizationID.Valid {
		id := uuid.UUID(c.OrganizationID.Bytes)
		company.OrganizationID = &id
	}
	return company
}
