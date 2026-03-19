package handler

import (
	"errors"

	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
)

type AuthHandler struct {
	auth    *service.AuthService
	company *service.CompanyService
	q       *sqlc.Queries
	botName string
}

func NewAuthHandler(auth *service.AuthService, company *service.CompanyService, q *sqlc.Queries, botName string) *AuthHandler {
	return &AuthHandler{auth: auth, company: company, q: q, botName: botName}
}

type registerRequest struct {
	Email        string `json:"email" binding:"required,email"`
	Password     string `json:"password" binding:"required,min=8"`
	FullName     string `json:"full_name" binding:"required"`
	CompanyName  string `json:"company_name" binding:"required"`
	Jurisdiction string `json:"jurisdiction"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	jurisdiction := req.Jurisdiction
	if jurisdiction != "SG" && jurisdiction != "PH" && jurisdiction != "LK" {
		jurisdiction = "PH"
	}

	user, err := h.auth.Register(c.Request.Context(), service.RegisterInput{
		Email:        req.Email,
		Password:     req.Password,
		FullName:     req.FullName,
		CompanyName:  req.CompanyName,
		Jurisdiction: jurisdiction,
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

type ssoLoginRequest struct {
	SSOToken string `json:"sso_token" binding:"required"`
}

// SSOLogin handles POST /api/v1/auth/sso — validates a cross-app JWT from AIGoNHR.
func (h *AuthHandler) SSOLogin(c *gin.Context) {
	var req ssoLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	tokens, companyID, jurisdiction, err := h.auth.SSOLogin(c.Request.Context(), req.SSOToken)
	if err != nil {
		if errors.Is(err, service.ErrSSONotConfigured) {
			response.InternalError(c, "SSO integration not configured")
			return
		}
		if errors.Is(err, service.ErrSSONoLink) {
			response.NotFound(c, "no active integration link for this HR company")
			return
		}
		response.Unauthorized(c, "invalid or expired SSO token")
		return
	}

	response.OK(c, gin.H{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"token_type":    tokens.TokenType,
		"company_id":    companyID,
		"tenant_id":     companyID,
		"jurisdiction":  jurisdiction,
	})
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

	tokens, companyID, jurisdiction, err := h.auth.Login(c.Request.Context(), req.Email, req.Password)
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
		"jurisdiction":  jurisdiction,
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
	jurisdiction := middleware.GetJurisdiction(c)

	email, _ := c.Get(string(middleware.EmailKey))
	role, _ := c.Get(string(middleware.RoleKey))

	response.OK(c, gin.H{
		"user_id":      userID,
		"company_id":   companyID,
		"tenant_id":    companyID, // backward compat
		"email":        email,
		"role":         role,
		"jurisdiction": jurisdiction,
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

type createMemberRequest struct {
	Email            string `json:"email" binding:"required,email"`
	Password         string `json:"password" binding:"required,min=8"`
	FullName         string `json:"full_name" binding:"required"`
	TelegramUsername string `json:"telegram_username"`
	Role             string `json:"role"`
}

func (h *AuthHandler) CreateMember(c *gin.Context) {
	var req createMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	role := req.Role
	if role == "" {
		role = "member"
	}

	companyID := middleware.GetCompanyID(c)

	result, err := h.auth.CreateMember(c.Request.Context(), service.CreateMemberInput{
		Email:            req.Email,
		Password:         req.Password,
		FullName:         req.FullName,
		TelegramUsername: req.TelegramUsername,
		CompanyID:        companyID,
		Role:             role,
	})
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			response.Conflict(c, "email already registered")
			return
		}
		if errors.Is(err, service.ErrTelegramUsernameTaken) {
			response.Conflict(c, "telegram username already linked to another account")
			return
		}
		response.InternalError(c, "failed to create member")
		return
	}

	resp := gin.H{
		"user":       result.User,
		"api_key":    result.APIKey,
		"company_id": companyID,
	}

	// Generate Telegram deep link if bot is configured
	if h.botName != "" && h.q != nil {
		token, err := generateToken(8)
		if err == nil {
			lt, err := h.q.CreateLinkToken(c.Request.Context(), sqlc.CreateLinkTokenParams{
				UserID:    result.User.ID,
				CompanyID: companyID,
				Token:     token,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			})
			if err == nil {
				resp["deep_link"] = fmt.Sprintf("https://t.me/%s?start=lt_%s", h.botName, lt.Token)
			}
		}
	}

	response.Created(c, resp)
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
	tokens, jurisdiction, err := h.auth.SwitchCompany(c.Request.Context(), userID, *companyID)
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
		"jurisdiction":  jurisdiction,
	})
}
