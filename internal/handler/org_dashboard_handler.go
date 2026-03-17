package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// OrgDashboardHandler handles org-level dashboard and batch endpoints.
type OrgDashboardHandler struct {
	svc *service.OrgDashboardService
}

// NewOrgDashboardHandler creates an OrgDashboardHandler.
func NewOrgDashboardHandler(svc *service.OrgDashboardService) *OrgDashboardHandler {
	return &OrgDashboardHandler{svc: svc}
}

// Dashboard handles GET /api/v1/orgs/:orgId/dashboard.
func (h *OrgDashboardHandler) Dashboard(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.BadRequest(c, "invalid org ID")
		return
	}

	data, err := h.svc.GetDashboard(c.Request.Context(), orgID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, data)
}

// BatchComplianceCheck handles POST /api/v1/orgs/:orgId/batch/compliance-check.
func (h *OrgDashboardHandler) BatchComplianceCheck(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.BadRequest(c, "invalid org ID")
		return
	}

	userID := middleware.GetUserID(c)

	result, err := h.svc.BatchComplianceCheck(c.Request.Context(), orgID, userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}
