package domain

import (
	"time"

	"github.com/google/uuid"
)

type CompanyRole string

const (
	CompanyRoleAdmin      CompanyRole = "company_admin"
	CompanyRoleAccountant CompanyRole = "accountant"
	CompanyRoleMember     CompanyRole = "member"
	CompanyRoleViewer     CompanyRole = "viewer"
)

type CompanyMember struct {
	CompanyID uuid.UUID   `json:"company_id"`
	UserID    uuid.UUID   `json:"user_id"`
	Role      CompanyRole `json:"role"`
	JoinedAt  time.Time   `json:"joined_at"`

	// Joined fields
	Email    string  `json:"email,omitempty"`
	FullName *string `json:"full_name,omitempty"`
}
