package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// VendorPolicyHandler handles vendor posting policy endpoints.
type VendorPolicyHandler struct {
	vendorMemory *service.VendorMemoryService
}

// NewVendorPolicyHandler creates a VendorPolicyHandler.
func NewVendorPolicyHandler(vendorMemory *service.VendorMemoryService) *VendorPolicyHandler {
	return &VendorPolicyHandler{vendorMemory: vendorMemory}
}

// List handles GET /vendor-policies.
func (h *VendorPolicyHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	policies, err := h.vendorMemory.ListPolicies(c.Request.Context(), companyID, int32(p.Limit), int32(p.Offset))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, policies, len(policies), p.Page, p.Limit)
}

// Get handles GET /vendor-policies/:id.
func (h *VendorPolicyHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid policy id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	policy, err := h.vendorMemory.GetPolicy(c.Request.Context(), id, companyID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, policy)
}

type updateVendorDefaultsRequest struct {
	Category    string `json:"category"`
	AccountCode string `json:"account_code"`
	TaxCode     string `json:"tax_code"`
	Department  string `json:"department"`
	Project     string `json:"project"`
}

// Update handles PUT /vendor-policies/:id.
func (h *VendorPolicyHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid policy id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	var req updateVendorDefaultsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Fetch existing policy to get the normalized vendor name.
	policy, err := h.vendorMemory.GetPolicy(c.Request.Context(), id, companyID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	if err := h.vendorMemory.UpdateDefaults(
		c.Request.Context(),
		companyID,
		policy.VendorNormalized,
		req.Category,
		req.AccountCode,
		req.TaxCode,
		req.Department,
		req.Project,
	); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"updated": true})
}

// Delete handles DELETE /vendor-policies/:id.
func (h *VendorPolicyHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid policy id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	if err := h.vendorMemory.DeletePolicy(c.Request.Context(), id, companyID); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"deleted": true})
}

// Suggestions handles GET /vendor-policies/suggestions.
func (h *VendorPolicyHandler) Suggestions(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	suggestions, err := h.vendorMemory.SuggestRules(c.Request.Context(), companyID, 0)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, suggestions)
}

// Promote handles POST /vendor-policies/:id/promote.
func (h *VendorPolicyHandler) Promote(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid policy id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	// Fetch existing policy to get the normalized vendor name.
	policy, err := h.vendorMemory.GetPolicy(c.Request.Context(), id, companyID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	if err := h.vendorMemory.PromoteRule(c.Request.Context(), companyID, policy.VendorNormalized); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, gin.H{"promoted": true})
}
