package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// DashboardHandler handles dashboard and analytics endpoints.
type DashboardHandler struct {
	svc *service.DashboardService
}

// NewDashboardHandler creates a dashboard handler.
func NewDashboardHandler(svc *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// Stats handles GET /api/v1/dashboard/stats.
func (h *DashboardHandler) Stats(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	stats, err := h.svc.GetStats(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, stats)
}

// Calendar handles GET /api/v1/dashboard/calendar.
func (h *DashboardHandler) Calendar(c *gin.Context) {
	year, _ := strconv.Atoi(c.DefaultQuery("year", strconv.Itoa(time.Now().Year())))
	months, _ := strconv.Atoi(c.DefaultQuery("months_ahead", "3"))

	entries := service.GenerateFilingCalendar(year, months)
	response.OK(c, entries)
}

// Compare handles GET /api/v1/dashboard/compare.
func (h *DashboardHandler) Compare(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	periodA := c.Query("period_a")
	periodB := c.Query("period_b")
	reportType := c.DefaultQuery("report_type", "BIR_2550M")

	if periodA == "" || periodB == "" {
		response.BadRequest(c, "period_a and period_b are required")
		return
	}

	result, err := h.svc.ComparePeriods(c.Request.Context(), companyID, periodA, periodB, reportType)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

// CompanySettings handles GET /api/v1/dashboard/company.
func (h *DashboardHandler) CompanySettings(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	company, err := h.svc.GetCompanyForDashboard(c.Request.Context(), companyID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, company)
}
