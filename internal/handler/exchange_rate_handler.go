package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

type ExchangeRateHandler struct {
	svc *service.ExchangeRateService
}

func NewExchangeRateHandler(svc *service.ExchangeRateService) *ExchangeRateHandler {
	return &ExchangeRateHandler{svc: svc}
}

func (h *ExchangeRateHandler) Create(c *gin.Context) {
	var input service.CreateExchangeRateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	rate, err := h.svc.Create(c.Request.Context(), companyID, input)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, rate)
}

func (h *ExchangeRateHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	rates, total, err := h.svc.List(c.Request.Context(), companyID, limit, offset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	page := 1
	if limit > 0 {
		page = (offset / limit) + 1
	}
	response.Paginated(c, rates, int(total), page, limit)
}

func (h *ExchangeRateHandler) GetLatest(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	from := c.Query("from")
	to := c.Query("to")
	if from == "" || to == "" {
		response.BadRequest(c, "from and to currency codes are required")
		return
	}

	asOfStr := c.DefaultQuery("as_of", time.Now().Format("2006-01-02"))
	asOf, err := time.Parse("2006-01-02", asOfStr)
	if err != nil {
		response.BadRequest(c, "invalid as_of date (use YYYY-MM-DD)")
		return
	}

	rate, err := h.svc.GetLatest(c.Request.Context(), companyID, from, to, asOf)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, rate)
}

func (h *ExchangeRateHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid exchange rate ID")
		return
	}

	companyID := middleware.GetCompanyID(c)
	if err := h.svc.Delete(c.Request.Context(), id, companyID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "deleted"})
}
