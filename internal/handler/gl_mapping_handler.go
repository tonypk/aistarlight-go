package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// GLMappingHandler handles GL mapping rule CRUD.
type GLMappingHandler struct {
	svc *service.GLMappingService
}

// NewGLMappingHandler creates a new handler.
func NewGLMappingHandler(svc *service.GLMappingService) *GLMappingHandler {
	return &GLMappingHandler{svc: svc}
}

func (h *GLMappingHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	jurisdiction := c.DefaultQuery("jurisdiction", "")

	rules, err := h.svc.List(c.Request.Context(), companyID, jurisdiction)
	if err != nil {
		response.InternalError(c, "failed to list GL mappings")
		return
	}
	response.OK(c, rules)
}

func (h *GLMappingHandler) Get(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}

	rule, err := h.svc.Get(c.Request.Context(), companyID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, rule)
}

type createGLMappingRequest struct {
	Jurisdiction    string `json:"jurisdiction" binding:"required"`
	SourceDimension string `json:"source_dimension" binding:"required"`
	SourceValue     string `json:"source_value" binding:"required"`
	TargetAccountID string `json:"target_account_id" binding:"required"`
	DebitCredit     string `json:"debit_credit" binding:"required"`
	Priority        int32  `json:"priority"`
	EffectiveFrom   string `json:"effective_from"`
}

func (h *GLMappingHandler) Create(c *gin.Context) {
	var req createGLMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	accountID, err := uuid.Parse(req.TargetAccountID)
	if err != nil {
		response.BadRequest(c, "invalid target_account_id")
		return
	}

	effectiveFrom := time.Now()
	if req.EffectiveFrom != "" {
		effectiveFrom, err = time.Parse("2006-01-02", req.EffectiveFrom)
		if err != nil {
			response.BadRequest(c, "invalid effective_from date")
			return
		}
	}

	if req.DebitCredit != "debit" && req.DebitCredit != "credit" {
		response.BadRequest(c, "debit_credit must be 'debit' or 'credit'")
		return
	}

	rule, err := h.svc.Create(c.Request.Context(), service.CreateGLMappingInput{
		CompanyID:       companyID,
		Jurisdiction:    req.Jurisdiction,
		SourceDimension: req.SourceDimension,
		SourceValue:     req.SourceValue,
		TargetAccountID: accountID,
		DebitCredit:     req.DebitCredit,
		Priority:        req.Priority,
		EffectiveFrom:   effectiveFrom,
	})
	if err != nil {
		response.InternalError(c, "failed to create GL mapping")
		return
	}
	response.Created(c, rule)
}

type updateGLMappingRequest struct {
	TargetAccountID string  `json:"target_account_id" binding:"required"`
	DebitCredit     string  `json:"debit_credit" binding:"required"`
	Priority        int32   `json:"priority"`
	EffectiveTo     *string `json:"effective_to"`
	IsActive        *bool   `json:"is_active"`
}

func (h *GLMappingHandler) Update(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}

	var req updateGLMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	accountID, err := uuid.Parse(req.TargetAccountID)
	if err != nil {
		response.BadRequest(c, "invalid target_account_id")
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var effectiveTo *time.Time
	if req.EffectiveTo != nil {
		t, err := time.Parse("2006-01-02", *req.EffectiveTo)
		if err != nil {
			response.BadRequest(c, "invalid effective_to date")
			return
		}
		effectiveTo = &t
	}

	rule, err := h.svc.Update(c.Request.Context(), service.UpdateGLMappingInput{
		ID:              id,
		CompanyID:       companyID,
		TargetAccountID: accountID,
		DebitCredit:     req.DebitCredit,
		Priority:        req.Priority,
		EffectiveTo:     effectiveTo,
		IsActive:        isActive,
	})
	if err != nil {
		response.InternalError(c, "failed to update GL mapping")
		return
	}
	response.OK(c, rule)
}

func (h *GLMappingHandler) Delete(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), companyID, id); err != nil {
		response.InternalError(c, "failed to delete GL mapping")
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

// SeedDefaults creates default accounts and GL mappings for a jurisdiction.
func (h *GLMappingHandler) SeedDefaults(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	jurisdiction := c.DefaultQuery("jurisdiction", "PH")

	result, err := h.svc.SeedDefaults(c.Request.Context(), companyID, jurisdiction)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, result)
}
