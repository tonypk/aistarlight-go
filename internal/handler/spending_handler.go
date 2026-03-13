package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// SpendingHandler handles spending analytics endpoints.
type SpendingHandler struct {
	analytics *service.SpendingAnalyticsService
}

// NewSpendingHandler creates a SpendingHandler.
func NewSpendingHandler(analytics *service.SpendingAnalyticsService) *SpendingHandler {
	return &SpendingHandler{analytics: analytics}
}

// Dashboard handles GET /api/v1/spending/dashboard.
func (h *SpendingHandler) Dashboard(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	fromStr := c.Query("from")
	toStr := c.Query("to")

	var from, to time.Time
	var err error

	if fromStr != "" {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			response.BadRequest(c, "invalid 'from' date format, use YYYY-MM-DD")
			return
		}
	} else {
		// Default: start of current year.
		now := time.Now()
		from = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	}

	if toStr != "" {
		to, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			response.BadRequest(c, "invalid 'to' date format, use YYYY-MM-DD")
			return
		}
	} else {
		to = time.Now()
	}

	dashboard, err := h.analytics.GetDashboard(c.Request.Context(), companyID, from, to)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, dashboard)
}
