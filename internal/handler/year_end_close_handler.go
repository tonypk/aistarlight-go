package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

type YearEndCloseHandler struct {
	svc *service.YearEndCloseService
}

func NewYearEndCloseHandler(svc *service.YearEndCloseService) *YearEndCloseHandler {
	return &YearEndCloseHandler{svc: svc}
}

type yearEndCloseRequest struct {
	Year int `json:"year"`
}

func (h *YearEndCloseHandler) Close(c *gin.Context) {
	var req yearEndCloseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Try query param
		yearStr := c.Query("year")
		if yearStr != "" {
			y, err := strconv.Atoi(yearStr)
			if err == nil {
				req.Year = y
			}
		}
	}
	if req.Year == 0 {
		req.Year = time.Now().Year() - 1
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	result, err := h.svc.Close(c.Request.Context(), companyID, req.Year, userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, result)
}
