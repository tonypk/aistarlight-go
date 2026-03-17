package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	sqlc "github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// IntegrationHandler handles integration source and event inbox endpoints.
type IntegrationHandler struct {
	q *sqlc.Queries
}

// NewIntegrationHandler creates a new integration handler.
func NewIntegrationHandler(q *sqlc.Queries) *IntegrationHandler {
	return &IntegrationHandler{q: q}
}

// ListSources returns integration sources for the current company.
func (h *IntegrationHandler) ListSources(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	sources, err := h.q.ListIntegrationSources(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, "failed to list integration sources")
		return
	}
	response.OK(c, sources)
}

// GetSource returns a single integration source.
func (h *IntegrationHandler) GetSource(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}
	source, err := h.q.GetIntegrationSourceByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "integration source not found")
		return
	}
	response.OK(c, source)
}

// ListEvents returns integration inbox events for the current company.
func (h *IntegrationHandler) ListEvents(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	events, err := h.q.ListInboxByCompany(c.Request.Context(), sqlc.ListInboxByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(p.Limit),
		Offset:    int32(p.Offset),
	})
	if err != nil {
		response.InternalError(c, "failed to list integration events")
		return
	}
	response.OK(c, events)
}
