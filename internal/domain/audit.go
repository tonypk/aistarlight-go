package domain

import (
	"time"

	"github.com/google/uuid"
)

type AuditLog struct {
	ID         uuid.UUID  `json:"id"`
	CompanyID  uuid.UUID  `json:"company_id"`
	UserID     *uuid.UUID `json:"user_id,omitempty"`
	EntityType string     `json:"entity_type"`
	EntityID   *uuid.UUID `json:"entity_id,omitempty"`
	Action     string     `json:"action"`
	Changes    JSON       `json:"changes,omitempty"`
	Comment    *string    `json:"comment,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}
