package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

type AccountingPeriodHandler struct {
	svc *service.AccountingPeriodService
}

func NewAccountingPeriodHandler(svc *service.AccountingPeriodService) *AccountingPeriodHandler {
	return &AccountingPeriodHandler{svc: svc}
}

type createPeriodRequest struct {
	Name       string `json:"name" binding:"required"`
	PeriodType string `json:"period_type" binding:"required"`
	StartDate  string `json:"start_date" binding:"required"`
	EndDate    string `json:"end_date" binding:"required"`
}

func (h *AccountingPeriodHandler) Create(c *gin.Context) {
	var req createPeriodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		response.BadRequest(c, "invalid start_date (use YYYY-MM-DD)")
		return
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		response.BadRequest(c, "invalid end_date (use YYYY-MM-DD)")
		return
	}

	companyID := middleware.GetCompanyID(c)
	period, err := h.svc.Create(c.Request.Context(), service.CreatePeriodInput{
		CompanyID:  companyID,
		Name:       req.Name,
		PeriodType: domain.PeriodType(req.PeriodType),
		StartDate:  startDate,
		EndDate:    endDate,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, period)
}

func (h *AccountingPeriodHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	periods, err := h.svc.List(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, periods)
}

func (h *AccountingPeriodHandler) Close(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid period ID")
		return
	}

	userID := middleware.GetUserID(c)
	if err := h.svc.Close(c.Request.Context(), id, userID); err != nil {
		if err == service.ErrPeriodHasDraftJE || err == service.ErrPeriodNotOpen {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "period closed"})
}

func (h *AccountingPeriodHandler) Reopen(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid period ID")
		return
	}

	if err := h.svc.Reopen(c.Request.Context(), id); err != nil {
		if err == service.ErrPeriodNotClosed {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "period reopened"})
}

type generatePeriodsRequest struct {
	Year int `json:"year" binding:"required"`
}

func (h *AccountingPeriodHandler) Generate(c *gin.Context) {
	var req generatePeriodsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Also try query param
		yearStr := c.Query("year")
		if yearStr != "" {
			y, err := strconv.Atoi(yearStr)
			if err == nil {
				req.Year = y
			}
		}
		if req.Year == 0 {
			response.BadRequest(c, "year is required")
			return
		}
	}

	companyID := middleware.GetCompanyID(c)
	periods, err := h.svc.GenerateMonthly(c.Request.Context(), companyID, req.Year)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, periods)
}
