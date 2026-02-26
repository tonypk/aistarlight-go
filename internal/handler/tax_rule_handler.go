package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// TaxRuleHandler handles tax rule and penalty endpoints.
type TaxRuleHandler struct {
	taxRuleSvc *service.TaxRuleService
}

// NewTaxRuleHandler creates a tax rule handler.
func NewTaxRuleHandler(taxRuleSvc *service.TaxRuleService) *TaxRuleHandler {
	return &TaxRuleHandler{taxRuleSvc: taxRuleSvc}
}

// ListRules handles GET /api/v1/tax-rules?type=rate.
func (h *TaxRuleHandler) ListRules(c *gin.Context) {
	ruleType := c.DefaultQuery("type", "rate")

	rules, err := h.taxRuleSvc.ListActiveRules(c.Request.Context(), ruleType)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, rules)
}

type penaltyRequest struct {
	FormType string  `json:"form_type" binding:"required"`
	Period   string  `json:"period" binding:"required"`
	DaysLate int     `json:"days_late" binding:"required,min=1"`
	TaxDue   float64 `json:"tax_due" binding:"required"`
}

// CalculatePenalty handles POST /api/v1/compliance/calculate-penalty.
func (h *TaxRuleHandler) CalculatePenalty(c *gin.Context) {
	var req penaltyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.taxRuleSvc.CalculatePenalty(c.Request.Context(), service.PenaltyInput{
		FormType: req.FormType,
		Period:   req.Period,
		DaysLate: req.DaysLate,
		TaxDue:   decimal.NewFromFloat(req.TaxDue),
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}
