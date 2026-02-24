package domain

import (
	"time"

	"github.com/google/uuid"
)

type OrgRole string

const (
	OrgRoleOwner  OrgRole = "org_owner"
	OrgRoleAdmin  OrgRole = "org_admin"
	OrgRoleMember OrgRole = "org_member"
)

type OrgMember struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	Role           OrgRole   `json:"role"`
	JoinedAt       time.Time `json:"joined_at"`

	// Joined fields (populated by queries)
	Email    string  `json:"email,omitempty"`
	FullName *string `json:"full_name,omitempty"`
}
