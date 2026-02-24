package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// MemoryHandler handles user preference endpoints.
type MemoryHandler struct {
	svc *service.MemoryService
}

// NewMemoryHandler creates a memory handler.
func NewMemoryHandler(svc *service.MemoryService) *MemoryHandler {
	return &MemoryHandler{svc: svc}
}

// GetPreference handles GET /api/v1/memory/preferences/:reportType.
func (h *MemoryHandler) GetPreference(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	reportType := c.Param("reportType")

	pref, err := h.svc.GetPreference(c.Request.Context(), companyID, reportType)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, pref)
}

type upsertPreferenceRequest struct {
	ColumnMappings map[string]interface{} `json:"column_mappings"`
	FormatRules    map[string]interface{} `json:"format_rules"`
	AutoFillRules  map[string]interface{} `json:"auto_fill_rules"`
}

// UpsertPreference handles PUT /api/v1/memory/preferences/:reportType.
func (h *MemoryHandler) UpsertPreference(c *gin.Context) {
	var req upsertPreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	reportType := c.Param("reportType")

	pref, err := h.svc.UpsertPreference(
		c.Request.Context(),
		companyID, reportType,
		req.ColumnMappings, req.FormatRules, req.AutoFillRules,
	)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, pref)
}
