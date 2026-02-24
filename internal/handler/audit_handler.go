package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// AuditHandler handles audit trail endpoints.
type AuditHandler struct {
	svc *service.AuditService
}

// NewAuditHandler creates an audit handler.
func NewAuditHandler(svc *service.AuditService) *AuditHandler {
	return &AuditHandler{svc: svc}
}

// List handles GET /api/v1/audit.
func (h *AuditHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	entries, total, err := h.svc.ListByCompany(c.Request.Context(), companyID, p.Limit, p.Offset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, entries, int(total), p.Page, p.Limit)
}

// ByReport handles GET /api/v1/audit/report/:reportId.
func (h *AuditHandler) ByReport(c *gin.Context) {
	reportID, err := uuid.Parse(c.Param("reportId"))
	if err != nil {
		response.BadRequest(c, "invalid report id")
		return
	}

	entries, err := h.svc.ListByEntity(c.Request.Context(), "report", reportID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, entries)
}
