package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// SettingsHandler handles company settings and team management endpoints.
type SettingsHandler struct {
	company *service.CompanyService
}

// NewSettingsHandler creates a settings handler.
func NewSettingsHandler(company *service.CompanyService) *SettingsHandler {
	return &SettingsHandler{company: company}
}

// GetCompanySettings handles GET /api/v1/settings/company.
func (h *SettingsHandler) GetCompanySettings(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	company, err := h.company.GetByID(c.Request.Context(), companyID)
	if err != nil {
		response.NotFound(c, "company not found")
		return
	}

	response.OK(c, company)
}

type updateCompanySettingsRequest struct {
	CompanyName       string  `json:"company_name"`
	TINNumber         *string `json:"tin_number"`
	RDOCode           *string `json:"rdo_code"`
	VATClassification string  `json:"vat_classification"`
	FiscalYearEnd     string  `json:"fiscal_year_end"`
	Industry          *string `json:"industry"`
	Address           *string `json:"address"`
}

// UpdateCompanySettings handles PUT /api/v1/settings/company.
func (h *SettingsHandler) UpdateCompanySettings(c *gin.Context) {
	var req updateCompanySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	updated, err := h.company.Update(c.Request.Context(), companyID, service.CreateCompanyInput{
		CompanyName:       req.CompanyName,
		TINNumber:         req.TINNumber,
		RDOCode:           req.RDOCode,
		VATClassification: req.VATClassification,
		FiscalYearEnd:     req.FiscalYearEnd,
		Industry:          req.Industry,
		Address:           req.Address,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, updated)
}

// ListTeam handles GET /api/v1/settings/team.
func (h *SettingsHandler) ListTeam(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	members, err := h.company.ListMembers(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, members)
}

type updateMemberRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// UpdateMemberRole handles PATCH /api/v1/settings/team/:userId/role.
func (h *SettingsHandler) UpdateMemberRole(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	var req updateMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	if err := h.company.UpdateMemberRole(c.Request.Context(), companyID, userID, domain.CompanyRole(req.Role)); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"updated": true})
}
