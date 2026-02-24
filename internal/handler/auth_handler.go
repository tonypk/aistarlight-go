package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

type AuthHandler struct {
	auth    *service.AuthService
	company *service.CompanyService
}

func NewAuthHandler(auth *service.AuthService, company *service.CompanyService) *AuthHandler {
	return &AuthHandler{auth: auth, company: company}
}

type registerRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	FullName    string `json:"full_name" binding:"required"`
	CompanyName string `json:"company_name" binding:"required"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	user, err := h.auth.Register(c.Request.Context(), service.RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		FullName:    req.FullName,
		CompanyName: req.CompanyName,
	})
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			response.Conflict(c, "email already registered")
			return
		}
		response.InternalError(c, "registration failed")
		return
	}

	response.Created(c, user)
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	tokens, companyID, err := h.auth.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCreds) || errors.Is(err, service.ErrUserInactive) {
			response.Unauthorized(c, "invalid email or password")
			return
		}
		response.InternalError(c, "login failed")
		return
	}

	response.OK(c, gin.H{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"token_type":    tokens.TokenType,
		"tenant_id":     companyID, // backward compat
		"company_id":    companyID,
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	tokens, err := h.auth.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.Unauthorized(c, "invalid or expired refresh token")
		return
	}

	response.OK(c, tokens)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	_ = h.auth.Logout(c.Request.Context(), req.RefreshToken)
	response.OK(c, gin.H{"message": "logged out"})
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID := middleware.GetUserID(c)
	companyID := middleware.GetCompanyID(c)

	email, _ := c.Get(string(middleware.EmailKey))
	role, _ := c.Get(string(middleware.RoleKey))

	response.OK(c, gin.H{
		"user_id":    userID,
		"company_id": companyID,
		"tenant_id":  companyID, // backward compat
		"email":      email,
		"role":       role,
	})
}

func (h *AuthHandler) GenerateAPIKey(c *gin.Context) {
	userID := middleware.GetUserID(c)

	key, err := h.auth.GenerateAPIKey(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "failed to generate API key")
		return
	}

	response.OK(c, gin.H{"api_key": key})
}

// ListCompanies lists all companies the current user has access to.
// This replaces the old /auth/companies endpoint.
func (h *AuthHandler) ListCompanies(c *gin.Context) {
	userID := middleware.GetUserID(c)

	companies, _, err := h.company.ListByUser(c.Request.Context(), userID, defaultPagination())
	if err != nil {
		response.InternalError(c, "failed to list companies")
		return
	}

	// Return in the old format with tenant_id for compat
	type companyItem struct {
		ID          uuid.UUID `json:"id"`
		TenantID    uuid.UUID `json:"tenant_id"` // backward compat
		CompanyName string    `json:"company_name"`
		Role        string    `json:"role"`
	}

	items := make([]companyItem, len(companies))
	for i, co := range companies {
		items[i] = companyItem{
			ID:          co.ID,
			TenantID:    co.ID,
			CompanyName: co.CompanyName,
		}
	}

	response.OK(c, items)
}

// InviteMember handles POST /api/v1/auth/invite.
// Adds a user as a member of the current company.
func (h *AuthHandler) InviteMember(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Role  string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	// Look up user by email
	user, err := h.auth.GetByEmail(c.Request.Context(), req.Email)
	if err != nil {
		response.NotFound(c, "user not found, they must register first")
		return
	}

	// Add as company member
	err = h.company.AddMember(c.Request.Context(), companyID, user.ID, domain.CompanyRole(req.Role))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, gin.H{
		"message":    "member invited successfully",
		"user_id":    user.ID,
		"company_id": companyID,
		"role":       req.Role,
	})
}

type switchCompanyRequest struct {
	CompanyID *uuid.UUID `json:"company_id"`
	TenantID  *uuid.UUID `json:"tenant_id"` // backward compat
}

func (h *AuthHandler) SwitchCompany(c *gin.Context) {
	var req switchCompanyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := req.CompanyID
	if companyID == nil {
		companyID = req.TenantID // backward compat
	}
	if companyID == nil {
		response.BadRequest(c, "company_id is required")
		return
	}

	userID := middleware.GetUserID(c)
	tokens, err := h.auth.SwitchCompany(c.Request.Context(), userID, *companyID)
	if err != nil {
		if errors.Is(err, service.ErrNoAccess) {
			response.Forbidden(c, "no access to this company")
			return
		}
		if errors.Is(err, service.ErrCompanyNotFound) {
			response.NotFound(c, "company not found")
			return
		}
		response.InternalError(c, "failed to switch company")
		return
	}

	response.OK(c, gin.H{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"token_type":    tokens.TokenType,
		"tenant_id":     companyID,
		"company_id":    companyID,
	})
}
