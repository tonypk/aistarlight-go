package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// AuditService records immutable audit trail entries.
type AuditService struct {
	q *sqlc.Queries
}

// NewAuditService creates an AuditService.
func NewAuditService(q *sqlc.Queries) *AuditService {
	return &AuditService{q: q}
}

// AuditEntry represents an audit log output.
type AuditEntry struct {
	ID         uuid.UUID              `json:"id"`
	CompanyID  uuid.UUID              `json:"company_id"`
	UserID     *uuid.UUID             `json:"user_id,omitempty"`
	EntityType string                 `json:"entity_type"`
	EntityID   *uuid.UUID             `json:"entity_id,omitempty"`
	Action     string                 `json:"action"`
	Changes    map[string]interface{} `json:"changes,omitempty"`
	Comment    *string                `json:"comment,omitempty"`
	CreatedAt  string                 `json:"created_at"`
}

// LogAction writes an audit log entry.
func (s *AuditService) LogAction(ctx context.Context, companyID uuid.UUID, userID *uuid.UUID, entityType string, entityID *uuid.UUID, action string, changes map[string]interface{}, comment *string) error {
	changesJSON, _ := json.Marshal(changes)

	uid := pgtype.UUID{}
	if userID != nil {
		uid = pgtype.UUID{Bytes: *userID, Valid: true}
	}

	eid := pgtype.UUID{}
	if entityID != nil {
		eid = pgtype.UUID{Bytes: *entityID, Valid: true}
	}

	_, err := s.q.CreateAuditLog(ctx, sqlc.CreateAuditLogParams{
		ID:         uuid.New(),
		CompanyID:  companyID,
		UserID:     uid,
		EntityType: entityType,
		EntityID:   eid,
		Action:     action,
		Changes:    changesJSON,
		Comment:    comment,
	})
	return err
}

// ListByCompany returns paginated audit logs for a company.
func (s *AuditService) ListByCompany(ctx context.Context, companyID uuid.UUID, limit, offset int) ([]AuditEntry, int64, error) {
	rows, err := s.q.ListAuditLogsByCompany(ctx, sqlc.ListAuditLogsByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}

	total, err := s.q.CountAuditLogsByCompany(ctx, companyID)
	if err != nil {
		return nil, 0, err
	}

	return toAuditEntries(rows), total, nil
}

// ListByEntity returns audit logs for a specific entity.
func (s *AuditService) ListByEntity(ctx context.Context, entityType string, entityID uuid.UUID) ([]AuditEntry, error) {
	rows, err := s.q.ListAuditLogsByEntity(ctx, sqlc.ListAuditLogsByEntityParams{
		EntityType: entityType,
		EntityID:   pgtype.UUID{Bytes: entityID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("list audit logs by entity: %w", err)
	}

	return toAuditEntries(rows), nil
}

func toAuditEntries(rows []sqlc.AuditLog) []AuditEntry {
	entries := make([]AuditEntry, len(rows))
	for i, r := range rows {
		entries[i] = AuditEntry{
			ID:         r.ID,
			CompanyID:  r.CompanyID,
			EntityType: r.EntityType,
			Action:     r.Action,
			Comment:    r.Comment,
			CreatedAt:  r.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if r.UserID.Valid {
			uid := uuid.UUID(r.UserID.Bytes)
			entries[i].UserID = &uid
		}
		if r.EntityID.Valid {
			eid := uuid.UUID(r.EntityID.Bytes)
			entries[i].EntityID = &eid
		}
		if len(r.Changes) > 0 {
			var changes map[string]interface{}
			_ = json.Unmarshal(r.Changes, &changes)
			entries[i].Changes = changes
		}
	}
	return entries
}
