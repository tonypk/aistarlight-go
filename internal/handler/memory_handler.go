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

// ListPreferences handles GET /api/v1/memory/preferences.
func (h *MemoryHandler) ListPreferences(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	prefs, err := h.svc.ListPreferences(c.Request.Context(), companyID)
	if err != nil {
		response.OK(c, []interface{}{})
		return
	}

	response.OK(c, prefs)
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

// DeletePreference handles DELETE /api/v1/memory/preferences/:reportType.
func (h *MemoryHandler) DeletePreference(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	reportType := c.Param("reportType")

	if err := h.svc.DeletePreference(c.Request.Context(), companyID, reportType); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"deleted": true})
}

// ListCorrections handles GET /api/v1/memory/corrections.
func (h *MemoryHandler) ListCorrections(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	corrections, err := h.svc.ListCorrections(c.Request.Context(), companyID)
	if err != nil {
		response.OK(c, []interface{}{})
		return
	}

	response.OK(c, corrections)
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
