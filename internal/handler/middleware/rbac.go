package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
)

type AccessChecker interface {
	// GetEffectiveRole returns the user's effective role for a company.
	// It checks both direct company_members and inherited org_members roles.
	GetEffectiveRole(ctx context.Context, userID, companyID uuid.UUID) (domain.CompanyRole, error)
}

// RequireCompanyRole checks that the user has at least the specified role for the current company.
// Role hierarchy: company_admin > accountant > viewer
func RequireCompanyRole(checker AccessChecker, minRole domain.CompanyRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := GetUserID(c)
		companyID := GetCompanyID(c)

		if userID == uuid.Nil || companyID == uuid.Nil {
			response.Unauthorized(c, "authentication required")
			c.Abort()
			return
		}

		role, err := checker.GetEffectiveRole(c.Request.Context(), userID, companyID)
		if err != nil {
			response.Forbidden(c, "access denied")
			c.Abort()
			return
		}

		if roleLevel(role) < roleLevel(minRole) {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireOrgRole checks org-level role for org management endpoints.
func RequireOrgRole(checker OrgAccessChecker, minRole domain.OrgRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := GetUserID(c)
		orgIDStr := c.Param("orgId")
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			response.BadRequest(c, "invalid organization ID")
			c.Abort()
			return
		}

		role, err := checker.GetOrgRole(c.Request.Context(), userID, orgID)
		if err != nil {
			response.Forbidden(c, "access denied")
			c.Abort()
			return
		}

		if orgRoleLevel(role) < orgRoleLevel(minRole) {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		c.Set("org_id", orgID)
		c.Next()
	}
}

type OrgAccessChecker interface {
	GetOrgRole(ctx context.Context, userID, orgID uuid.UUID) (domain.OrgRole, error)
}

func roleLevel(r domain.CompanyRole) int {
	switch r {
	case domain.CompanyRoleAdmin:
		return 3
	case domain.CompanyRoleAccountant:
		return 2
	case domain.CompanyRoleViewer:
		return 1
	default:
		return 0
	}
}

func orgRoleLevel(r domain.OrgRole) int {
	switch r {
	case domain.OrgRoleOwner:
		return 3
	case domain.OrgRoleAdmin:
		return 2
	case domain.OrgRoleMember:
		return 1
	default:
		return 0
	}
}
