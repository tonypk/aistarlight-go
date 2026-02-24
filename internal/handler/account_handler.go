package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

type AccountHandler struct {
	svc *service.AccountService
}

func NewAccountHandler(svc *service.AccountService) *AccountHandler {
	return &AccountHandler{svc: svc}
}

type createAccountRequest struct {
	AccountNumber string `json:"account_number" binding:"required"`
	Name          string `json:"name" binding:"required"`
	AccountType   string `json:"account_type" binding:"required"`
	SubType       string `json:"sub_type"`
	ParentID      string `json:"parent_id"`
	Description   string `json:"description"`
	NormalBalance string `json:"normal_balance"`
}

func (h *AccountHandler) Create(c *gin.Context) {
	var req createAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	normalBal := domain.NormalBalance(req.NormalBalance)
	if normalBal == "" {
		normalBal = domain.NormalBalanceFor(domain.AccountType(req.AccountType))
	}

	input := service.CreateAccountInput{
		CompanyID:     companyID,
		AccountNumber: req.AccountNumber,
		Name:          req.Name,
		AccountType:   domain.AccountType(req.AccountType),
		NormalBalance: normalBal,
	}
	if req.SubType != "" {
		input.SubType = &req.SubType
	}
	if req.Description != "" {
		input.Description = &req.Description
	}
	if req.ParentID != "" {
		pid, err := uuid.Parse(req.ParentID)
		if err == nil {
			input.ParentID = &pid
		}
	}

	acct, err := h.svc.Create(c.Request.Context(), input)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, acct)
}

func (h *AccountHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid account ID")
		return
	}

	acct, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	if acct.CompanyID != companyID {
		response.Forbidden(c, "no access")
		return
	}
	response.OK(c, acct)
}

func (h *AccountHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	accounts, total, err := h.svc.List(c.Request.Context(), companyID, p)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Paginated(c, accounts, int(total), p.Page, p.Limit)
}

type updateAccountRequest struct {
	Name        *string `json:"name"`
	SubType     *string `json:"sub_type"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"is_active"`
}

func (h *AccountHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid account ID")
		return
	}

	var req updateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.svc.Update(c.Request.Context(), id, req.Name, req.SubType, req.Description, req.IsActive, nil); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "updated"})
}

func (h *AccountHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid account ID")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		if err == service.ErrSystemAccount {
			response.Err(c, http.StatusForbidden, err.Error())
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "deleted"})
}

func (h *AccountHandler) Seed(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	count, err := h.svc.Seed(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"created": count})
}

func (h *AccountHandler) Balance(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid account ID")
		return
	}

	asOf := c.DefaultQuery("as_of", "2099-12-31")
	date := pgtype.Date{}
	if err := date.Scan(asOf); err != nil {
		response.BadRequest(c, "invalid date format")
		return
	}

	bal, err := h.svc.GetBalance(c.Request.Context(), id, date)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, bal)
}
