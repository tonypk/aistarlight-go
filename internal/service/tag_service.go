package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// TagService handles tag CRUD and transaction-tag assignments.
type TagService struct {
	q *sqlc.Queries
}

// NewTagService creates a TagService.
func NewTagService(q *sqlc.Queries) *TagService {
	return &TagService{q: q}
}

// TagResponse is the API response for a tag.
type TagResponse struct {
	ID        string `json:"id"`
	CompanyID string `json:"company_id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	IsProject bool   `json:"is_project"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func tagToResponse(t sqlc.Tag) TagResponse {
	return TagResponse{
		ID:        t.ID.String(),
		CompanyID: t.CompanyID.String(),
		Name:      t.Name,
		Color:     t.Color,
		IsProject: t.IsProject,
		CreatedAt: t.CreatedAt.Format(time.RFC3339),
		UpdatedAt: t.UpdatedAt.Format(time.RFC3339),
	}
}

// Create creates a new tag for a company.
func (s *TagService) Create(ctx context.Context, companyID uuid.UUID, name, color string, isProject bool) (*TagResponse, error) {
	if name == "" {
		return nil, fmt.Errorf("tag name is required")
	}
	if color == "" {
		color = "#4f46e5"
	}

	tag, err := s.q.CreateTag(ctx, sqlc.CreateTagParams{
		CompanyID: companyID,
		Name:      name,
		Color:     color,
		IsProject: isProject,
	})
	if err != nil {
		return nil, fmt.Errorf("create tag: %w", err)
	}

	resp := tagToResponse(tag)
	return &resp, nil
}

// List returns paginated tags for a company with optional search and is_project filter.
func (s *TagService) List(ctx context.Context, companyID uuid.UUID, search string, isProject *bool, limit, offset int) ([]TagResponse, int64, error) {
	tags, err := s.q.ListTagsByCompany(ctx, sqlc.ListTagsByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(limit),
		Offset:    int32(offset),
		Column4:   search,
		IsProject: isProject,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list tags: %w", err)
	}

	total, err := s.q.CountTagsByCompany(ctx, sqlc.CountTagsByCompanyParams{
		CompanyID: companyID,
		Column2:   search,
		IsProject: isProject,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count tags: %w", err)
	}

	result := make([]TagResponse, len(tags))
	for i, t := range tags {
		result[i] = tagToResponse(t)
	}
	return result, total, nil
}

// Update updates a tag's name, color, and is_project flag.
func (s *TagService) Update(ctx context.Context, id, companyID uuid.UUID, name, color string, isProject bool) (*TagResponse, error) {
	if name == "" {
		return nil, fmt.Errorf("tag name is required")
	}

	tag, err := s.q.UpdateTag(ctx, sqlc.UpdateTagParams{
		ID:        id,
		CompanyID: companyID,
		Name:      name,
		Color:     color,
		IsProject: isProject,
	})
	if err != nil {
		return nil, fmt.Errorf("update tag: %w", err)
	}

	resp := tagToResponse(tag)
	return &resp, nil
}

// Delete deletes a tag. Cascade will remove transaction_tags entries.
func (s *TagService) Delete(ctx context.Context, id, companyID uuid.UUID) error {
	return s.q.DeleteTag(ctx, sqlc.DeleteTagParams{
		ID:        id,
		CompanyID: companyID,
	})
}

// SetTransactionTags replaces all tags on a transaction with the given tag IDs.
func (s *TagService) SetTransactionTags(ctx context.Context, transactionID uuid.UUID, tagIDs []uuid.UUID) error {
	if err := s.q.DeleteAllTransactionTags(ctx, transactionID); err != nil {
		return fmt.Errorf("clear transaction tags: %w", err)
	}

	for _, tagID := range tagIDs {
		if err := s.q.AddTransactionTag(ctx, sqlc.AddTransactionTagParams{
			TransactionID: transactionID,
			TagID:         tagID,
		}); err != nil {
			return fmt.Errorf("add transaction tag: %w", err)
		}
	}
	return nil
}

// GetTransactionTags returns all tags for a transaction.
func (s *TagService) GetTransactionTags(ctx context.Context, transactionID uuid.UUID) ([]TagResponse, error) {
	tags, err := s.q.ListTagsForTransaction(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("get transaction tags: %w", err)
	}

	result := make([]TagResponse, len(tags))
	for i, t := range tags {
		result[i] = tagToResponse(t)
	}
	return result, nil
}

// ListProjectTags returns all project tags for a company (used by bot).
func (s *TagService) ListProjectTags(ctx context.Context, companyID uuid.UUID) ([]string, error) {
	tags, err := s.q.ListProjectTagsByCompany(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("list project tags: %w", err)
	}

	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.Name
	}
	return names, nil
}

// SeedProjectTags idempotently creates project tags from a list of names.
func (s *TagService) SeedProjectTags(ctx context.Context, companyID uuid.UUID, names []string) error {
	for _, name := range names {
		_, err := s.q.UpsertProjectTag(ctx, sqlc.UpsertProjectTagParams{
			CompanyID: companyID,
			Name:      name,
			Column3:   "",
		})
		if err != nil {
			slog.Warn("seed project tag failed", "name", name, "error", err)
			continue
		}
	}
	return nil
}

// LinkProjectTagToTransaction finds the project tag by name and links it to a transaction.
func (s *TagService) LinkProjectTagToTransaction(ctx context.Context, companyID uuid.UUID, projectName string, transactionID uuid.UUID) error {
	if projectName == "" {
		return nil
	}

	// Upsert to ensure the project tag exists, then link it.
	tag, err := s.q.UpsertProjectTag(ctx, sqlc.UpsertProjectTagParams{
		CompanyID: companyID,
		Name:      projectName,
		Column3:   "",
	})
	if err != nil {
		return fmt.Errorf("upsert project tag %q: %w", projectName, err)
	}

	return s.q.AddTransactionTag(ctx, sqlc.AddTransactionTagParams{
		TransactionID: transactionID,
		TagID:         tag.ID,
	})
}
