package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// ComplianceHandler handles compliance validation endpoints.
type ComplianceHandler struct {
	compliance *service.ComplianceService
	calendar   func(int, int) []service.FilingEntry
}

// NewComplianceHandler creates a compliance handler.
func NewComplianceHandler(compliance *service.ComplianceService) *ComplianceHandler {
	return &ComplianceHandler{
		compliance: compliance,
		calendar:   service.GenerateFilingCalendar,
	}
}

// Validate handles POST /api/v1/compliance/validate/:reportId.
func (h *ComplianceHandler) Validate(c *gin.Context) {
	reportID, err := uuid.Parse(c.Param("reportId"))
	if err != nil {
		response.BadRequest(c, "invalid report id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	result, err := h.compliance.ValidateReport(c.Request.Context(), reportID, companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

// GetLatest handles GET /api/v1/compliance/reports/:reportId/latest.
func (h *ComplianceHandler) GetLatest(c *gin.Context) {
	reportID, err := uuid.Parse(c.Param("reportId"))
	if err != nil {
		response.BadRequest(c, "invalid report id")
		return
	}

	result, err := h.compliance.GetLatestValidation(c.Request.Context(), reportID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, result)
}

// ListValidations handles GET /api/v1/compliance/reports/:reportId/history.
func (h *ComplianceHandler) ListValidations(c *gin.Context) {
	reportID, err := uuid.Parse(c.Param("reportId"))
	if err != nil {
		response.BadRequest(c, "invalid report id")
		return
	}

	results, err := h.compliance.ListValidations(c.Request.Context(), reportID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, results)
}

type filingCalendarRequest struct {
	Year        int `form:"year"`
	MonthsAhead int `form:"months_ahead"`
}

// FilingCalendar handles GET /api/v1/compliance/filing-calendar.
func (h *ComplianceHandler) FilingCalendar(c *gin.Context) {
	var req filingCalendarRequest
	_ = c.ShouldBindQuery(&req)

	if req.Year == 0 {
		req.Year = 2026
	}
	if req.MonthsAhead == 0 {
		req.MonthsAhead = 3
	}

	entries := h.calendar(req.Year, req.MonthsAhead)
	response.OK(c, entries)
}
