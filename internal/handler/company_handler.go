package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

type CompanyHandler struct {
	company *service.CompanyService
}

func NewCompanyHandler(company *service.CompanyService) *CompanyHandler {
	return &CompanyHandler{company: company}
}

type createCompanyRequest struct {
	CompanyName       string  `json:"company_name" binding:"required"`
	TINNumber         *string `json:"tin_number"`
	RDOCode           *string `json:"rdo_code"`
	VATClassification string  `json:"vat_classification"`
	FiscalYearEnd     string  `json:"fiscal_year_end"`
	Industry          *string `json:"industry"`
	Address           *string `json:"address"`
}

// CreateUnderOrg creates a company under a specific organization.
func (h *CompanyHandler) CreateUnderOrg(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.BadRequest(c, "invalid org ID")
		return
	}

	var req createCompanyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	company, err := h.company.Create(c.Request.Context(), service.CreateCompanyInput{
		OrgID:             &orgID,
		CompanyName:       req.CompanyName,
		TINNumber:         req.TINNumber,
		RDOCode:           req.RDOCode,
		VATClassification: req.VATClassification,
		FiscalYearEnd:     req.FiscalYearEnd,
		Industry:          req.Industry,
		Address:           req.Address,
		CreatedByUserID:   userID,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, company)
}

// ListByOrg lists companies under a specific organization.
func (h *CompanyHandler) ListByOrg(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("orgId"))
	if err != nil {
		response.BadRequest(c, "invalid org ID")
		return
	}

	p := pagination.Parse(c)
	companies, total, err := h.company.ListByOrg(c.Request.Context(), orgID, p)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, companies, total, p.Page, p.Limit)
}

func (h *CompanyHandler) Get(c *gin.Context) {
	companyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid company ID")
		return
	}

	company, err := h.company.GetByID(c.Request.Context(), companyID)
	if err != nil {
		response.NotFound(c, "company not found")
		return
	}

	response.OK(c, company)
}

func (h *CompanyHandler) Update(c *gin.Context) {
	companyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid company ID")
		return
	}

	var req createCompanyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	company, err := h.company.Update(c.Request.Context(), companyID, service.CreateCompanyInput{
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

	response.OK(c, company)
}

func (h *CompanyHandler) ListMembers(c *gin.Context) {
	companyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid company ID")
		return
	}

	members, err := h.company.ListMembers(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, members)
}

type addCompanyMemberRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
	Role   string    `json:"role" binding:"required"`
}

func (h *CompanyHandler) AddMember(c *gin.Context) {
	companyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid company ID")
		return
	}

	var req addCompanyMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	err = h.company.AddMember(c.Request.Context(), companyID, req.UserID, domain.CompanyRole(req.Role))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, gin.H{"message": "member added"})
}
