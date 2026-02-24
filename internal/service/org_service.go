package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

var (
	ErrOrgNotFound   = errors.New("organization not found")
	ErrSlugTaken     = errors.New("organization slug already taken")
	ErrOrgLimitUsers = errors.New("organization has reached max users")
)

type OrgService struct {
	q *sqlc.Queries
}

func NewOrgService(q *sqlc.Queries) *OrgService {
	return &OrgService{q: q}
}

type CreateOrgInput struct {
	Name    string
	Slug    string
	OwnerID uuid.UUID
}

func (s *OrgService) Create(ctx context.Context, input CreateOrgInput) (*domain.Organization, error) {
	slug := slugify(input.Slug)
	if slug == "" {
		slug = slugify(input.Name)
	}

	// Check slug uniqueness
	existing, _ := s.q.GetOrganizationBySlug(ctx, slug)
	if existing.ID != uuid.Nil {
		return nil, ErrSlugTaken
	}

	orgID := uuid.New()
	dbOrg, err := s.q.CreateOrganization(ctx, sqlc.CreateOrganizationParams{
		ID:           orgID,
		Name:         input.Name,
		Slug:         slug,
		Plan:         "free",
		MaxCompanies: 5,
		MaxUsers:     10,
		Settings:     []byte("{}"),
		IsActive:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}

	// Add creator as org_owner
	err = s.q.AddOrgMember(ctx, sqlc.AddOrgMemberParams{
		OrganizationID: orgID,
		UserID:         input.OwnerID,
		Role:           string(domain.OrgRoleOwner),
	})
	if err != nil {
		return nil, fmt.Errorf("add owner: %w", err)
	}

	return toOrg(dbOrg), nil
}

func (s *OrgService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Organization, error) {
	dbOrg, err := s.q.GetOrganizationByID(ctx, id)
	if err != nil {
		return nil, ErrOrgNotFound
	}
	return toOrg(dbOrg), nil
}

func (s *OrgService) Update(ctx context.Context, id uuid.UUID, name, slug string) (*domain.Organization, error) {
	org, err := s.q.GetOrganizationByID(ctx, id)
	if err != nil {
		return nil, ErrOrgNotFound
	}

	newSlug := slugify(slug)
	if newSlug != org.Slug {
		existing, _ := s.q.GetOrganizationBySlug(ctx, newSlug)
		if existing.ID != uuid.Nil && existing.ID != id {
			return nil, ErrSlugTaken
		}
	}

	err = s.q.UpdateOrganization(ctx, sqlc.UpdateOrganizationParams{
		ID:           id,
		Name:         name,
		Slug:         newSlug,
		Plan:         org.Plan,
		MaxCompanies: org.MaxCompanies,
		MaxUsers:     org.MaxUsers,
		Settings:     org.Settings,
		IsActive:     org.IsActive,
	})
	if err != nil {
		return nil, fmt.Errorf("update org: %w", err)
	}

	return s.GetByID(ctx, id)
}

func (s *OrgService) ListByUser(ctx context.Context, userID uuid.UUID, p pagination.Params) ([]domain.Organization, int, error) {
	orgs, err := s.q.ListOrganizationsByUser(ctx, sqlc.ListOrganizationsByUserParams{
		UserID: userID,
		Limit:  int32(p.Limit),
		Offset: int32(p.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list orgs: %w", err)
	}

	count, err := s.q.CountOrganizationsByUser(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("count orgs: %w", err)
	}

	result := make([]domain.Organization, len(orgs))
	for i, o := range orgs {
		result[i] = *toOrg(o)
	}

	return result, int(count), nil
}

func (s *OrgService) AddMember(ctx context.Context, orgID, userID uuid.UUID, role domain.OrgRole) error {
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return ErrOrgNotFound
	}

	members, err := s.q.ListOrgMembers(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list members: %w", err)
	}

	if int32(len(members)) >= org.MaxUsers {
		return ErrOrgLimitUsers
	}

	return s.q.AddOrgMember(ctx, sqlc.AddOrgMemberParams{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           string(role),
	})
}

func (s *OrgService) RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error {
	return s.q.RemoveOrgMember(ctx, sqlc.RemoveOrgMemberParams{
		OrganizationID: orgID,
		UserID:         userID,
	})
}

func (s *OrgService) UpdateMemberRole(ctx context.Context, orgID, userID uuid.UUID, role domain.OrgRole) error {
	return s.q.UpdateOrgMemberRole(ctx, sqlc.UpdateOrgMemberRoleParams{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           string(role),
	})
}

func (s *OrgService) ListMembers(ctx context.Context, orgID uuid.UUID) ([]domain.OrgMember, error) {
	rows, err := s.q.ListOrgMembers(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}

	members := make([]domain.OrgMember, len(rows))
	for i, r := range rows {
		members[i] = domain.OrgMember{
			OrganizationID: r.OrganizationID,
			UserID:         r.UserID,
			Role:           domain.OrgRole(r.Role),
			JoinedAt:       r.JoinedAt,
			Email:          r.Email,
			FullName:       r.FullName,
		}
	}
	return members, nil
}

// GetOrgRole implements middleware.OrgAccessChecker.
func (s *OrgService) GetOrgRole(ctx context.Context, userID, orgID uuid.UUID) (domain.OrgRole, error) {
	role, err := s.q.GetOrgMemberRole(ctx, sqlc.GetOrgMemberRoleParams{
		OrganizationID: orgID,
		UserID:         userID,
	})
	if err != nil {
		return "", fmt.Errorf("not a member")
	}
	return domain.OrgRole(role), nil
}

func toOrg(o sqlc.Organization) *domain.Organization {
	return &domain.Organization{
		ID:           o.ID,
		Name:         o.Name,
		Slug:         o.Slug,
		Plan:         o.Plan,
		MaxCompanies: int(o.MaxCompanies),
		MaxUsers:     int(o.MaxUsers),
		Settings:     domain.JSON(o.Settings),
		IsActive:     o.IsActive,
		CreatedAt:    o.CreatedAt,
		UpdatedAt:    o.UpdatedAt,
	}
}

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 100 {
		s = s[:100]
	}
	return s
}
