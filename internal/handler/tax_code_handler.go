package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

type TaxCodeHandler struct {
	svc *service.TaxCodeService
}

func NewTaxCodeHandler(svc *service.TaxCodeService) *TaxCodeHandler {
	return &TaxCodeHandler{svc: svc}
}

func (h *TaxCodeHandler) Create(c *gin.Context) {
	var input service.CreateTaxCodeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	tc, err := h.svc.Create(c.Request.Context(), companyID, input)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, tc)
}

func (h *TaxCodeHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	codes, err := h.svc.List(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, codes)
}

func (h *TaxCodeHandler) GetByCode(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	code := c.Param("code")

	tc, err := h.svc.GetByCode(c.Request.Context(), companyID, code)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, tc)
}

func (h *TaxCodeHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid tax code ID")
		return
	}

	companyID := middleware.GetCompanyID(c)
	if err := h.svc.Delete(c.Request.Context(), id, companyID); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "deleted"})
}
