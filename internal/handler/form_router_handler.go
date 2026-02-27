package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// FormRouterHandler handles form recommendation endpoints.
type FormRouterHandler struct {
	router *service.FormRouter
}

// NewFormRouterHandler creates a form router handler.
func NewFormRouterHandler(router *service.FormRouter) *FormRouterHandler {
	return &FormRouterHandler{router: router}
}

// Recommend handles GET /api/v1/forms/recommended.
func (h *FormRouterHandler) Recommend(c *gin.Context) {
	profile := service.CompanyProfile{
		VATRegistered: c.DefaultQuery("vat", "true") == "true",
		HasEmployees:  c.DefaultQuery("has_employees", "true") == "true",
		EntityType:    c.DefaultQuery("entity_type", "corporation"),
		Industry:      c.Query("industry"),
		RevenueScale:  c.DefaultQuery("revenue_scale", "small"),
	}

	jurisdiction := middleware.GetJurisdiction(c)
	recs := h.router.RecommendForms(profile, jurisdiction)
	response.OK(c, recs)
}
