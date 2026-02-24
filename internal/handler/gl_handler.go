package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

type GLHandler struct {
	svc *service.GLService
}

func NewGLHandler(svc *service.GLService) *GLHandler {
	return &GLHandler{svc: svc}
}

func (h *GLHandler) TrialBalance(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	startDate, endDate, err := parseDateRange(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	tb, err := h.svc.TrialBalance(c.Request.Context(), companyID, startDate, endDate)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, tb)
}

func (h *GLHandler) AccountLedger(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid account ID")
		return
	}

	startDate, endDate, err := parseDateRange(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	ledger, err := h.svc.AccountLedger(c.Request.Context(), companyID, accountID, startDate, endDate)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, ledger)
}

func parseDateRange(c *gin.Context) (time.Time, time.Time, error) {
	startStr := c.DefaultQuery("start_date", time.Now().AddDate(0, 0, -time.Now().YearDay()+1).Format("2006-01-02"))
	endStr := c.DefaultQuery("end_date", time.Now().Format("2006-01-02"))

	startDate, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	endDate, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return startDate, endDate, nil
}
