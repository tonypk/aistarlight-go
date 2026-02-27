package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	oai "github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// ComplianceHandler handles compliance validation endpoints.
type ComplianceHandler struct {
	compliance   *service.ComplianceService
	reportSvc    *service.ReportService
	ai           *oai.Client
	calendar     func(int, int, string) []service.FilingEntry
	ruleResolver *service.RuleResolver
}

// NewComplianceHandler creates a compliance handler.
func NewComplianceHandler(compliance *service.ComplianceService, reportSvc *service.ReportService, ai *oai.Client, resolver *service.RuleResolver) *ComplianceHandler {
	return &ComplianceHandler{
		compliance:   compliance,
		reportSvc:    reportSvc,
		ai:           ai,
		calendar:     service.GenerateFilingCalendar,
		ruleResolver: resolver,
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

// SuggestFixes handles GET /api/v1/compliance/reports/:reportId/suggest-fixes.
func (h *ComplianceHandler) SuggestFixes(c *gin.Context) {
	reportID, err := uuid.Parse(c.Param("reportId"))
	if err != nil {
		response.BadRequest(c, "invalid report id")
		return
	}

	result, err := h.compliance.GenerateAutoFixSuggestions(c.Request.Context(), reportID, h.ai, middleware.GetJurisdiction(c))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

// AutoFix handles POST /api/v1/compliance/reports/:reportId/auto-fix.
func (h *ComplianceHandler) AutoFix(c *gin.Context) {
	reportID, err := uuid.Parse(c.Param("reportId"))
	if err != nil {
		response.BadRequest(c, "invalid report id")
		return
	}

	var req struct {
		Suggestions []service.AutoFixSuggestion `json:"suggestions" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	userID := middleware.GetUserID(c)

	result, err := h.compliance.ApplyAutoFix(c.Request.Context(), reportID, userID, req.Suggestions, h.reportSvc)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

type filingCalendarRequest struct {
	Year        int `form:"year"`
	MonthsAhead int `form:"months_ahead"`
}

// Checklists handles GET /api/v1/compliance/checklists?form_type=BIR_2550M.
func (h *ComplianceHandler) Checklists(c *gin.Context) {
	formType := c.Query("form_type")
	if formType == "" {
		// Return all checklists
		items, err := h.ruleResolver.ListAll(c.Request.Context(), middleware.GetJurisdiction(c))
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}
		response.OK(c, items)
		return
	}

	items, err := h.ruleResolver.ListByFormType(c.Request.Context(), formType)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, items)
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

	jurisdiction := middleware.GetJurisdiction(c)
	entries := h.calendar(req.Year, req.MonthsAhead, jurisdiction)
	response.OK(c, entries)
}
