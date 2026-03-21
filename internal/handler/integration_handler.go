package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	sqlc "github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// IntegrationHandler handles integration source and event inbox endpoints.
type IntegrationHandler struct {
	q         *sqlc.Queries
	jwtSecret string
}

// NewIntegrationHandler creates a new integration handler.
func NewIntegrationHandler(q *sqlc.Queries, jwtSecret string) *IntegrationHandler {
	return &IntegrationHandler{q: q, jwtSecret: jwtSecret}
}

// financeToHRClaims defines the JWT claims for Finance→HR SSO tokens.
type financeToHRClaims struct {
	jwt.RegisteredClaims
	Email            string `json:"email"`
	FinanceCompanyID string `json:"finance_company_id"`
	FinanceUserID    string `json:"finance_user_id"`
}

// GetHRSSOToken generates a short-lived JWT for SSO into the HR system.
func (h *IntegrationHandler) GetHRSSOToken(c *gin.Context) {
	if h.jwtSecret == "" {
		response.InternalError(c, "SSO integration not configured")
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)
	// Get email from context (middleware stores it with key "email")
	emailVal, _ := c.Get("email")
	email, _ := emailVal.(string)

	// Check that an active HR integration source exists for this company
	source, err := h.q.GetIntegrationSource(c.Request.Context(), sqlc.GetIntegrationSourceParams{
		CompanyID:    companyID,
		SourceSystem: "aigonhr",
	})
	if err != nil {
		response.NotFound(c, "no HR integration configured for this company")
		return
	}
	if source.Status != "active" {
		response.NotFound(c, "HR integration is not active")
		return
	}

	now := time.Now()
	claims := &financeToHRClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "aistarlight",
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        fmt.Sprintf("sso-%s-%d", userID.String(), now.UnixMilli()),
		},
		Email:            email,
		FinanceCompanyID: companyID.String(),
		FinanceUserID:    userID.String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		response.InternalError(c, "failed to generate SSO token")
		return
	}

	targetURL := os.Getenv("HR_BASE_URL")
	if targetURL == "" {
		targetURL = "https://hr.halaos.com"
	}

	response.OK(c, gin.H{
		"sso_token":  tokenStr,
		"target_url": targetURL,
	})
}

// ListSources returns integration sources for the current company.
func (h *IntegrationHandler) ListSources(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	sources, err := h.q.ListIntegrationSources(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, "failed to list integration sources")
		return
	}
	response.OK(c, sources)
}

// GetSource returns a single integration source.
func (h *IntegrationHandler) GetSource(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}
	source, err := h.q.GetIntegrationSourceByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "integration source not found")
		return
	}
	response.OK(c, source)
}

// CreateSource registers a new integration source for the current company.
func (h *IntegrationHandler) CreateSource(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var req struct {
		SourceSystem    string `json:"source_system" binding:"required"`
		RemoteCompanyID string `json:"remote_company_id" binding:"required"`
		WebhookSecret   string `json:"webhook_secret"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// Auto-generate webhook secret if not provided
	webhookSecret := req.WebhookSecret
	if webhookSecret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			response.InternalError(c, "failed to generate webhook secret")
			return
		}
		webhookSecret = hex.EncodeToString(b)
	}

	source, err := h.q.CreateIntegrationSource(c.Request.Context(), sqlc.CreateIntegrationSourceParams{
		CompanyID:       companyID,
		SourceSystem:    req.SourceSystem,
		RemoteCompanyID: req.RemoteCompanyID,
		ApiKeyHash:      "",
		WebhookSecret:   webhookSecret,
		Status:          "active",
	})
	if err != nil {
		response.InternalError(c, "failed to create integration source")
		return
	}

	response.Created(c, source)
}

// DeleteSource removes an integration source.
func (h *IntegrationHandler) DeleteSource(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}

	if err := h.q.DeleteIntegrationSource(c.Request.Context(), sqlc.DeleteIntegrationSourceParams{
		ID:        id,
		CompanyID: companyID,
	}); err != nil {
		response.InternalError(c, "failed to delete integration source")
		return
	}

	response.OK(c, gin.H{"message": "integration source removed"})
}

// ListEvents returns integration inbox events for the current company.
func (h *IntegrationHandler) ListEvents(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	events, err := h.q.ListInboxByCompany(c.Request.Context(), sqlc.ListInboxByCompanyParams{
		CompanyID: companyID,
		Limit:     int32(p.Limit),
		Offset:    int32(p.Offset),
	})
	if err != nil {
		response.InternalError(c, "failed to list integration events")
		return
	}
	response.OK(c, events)
}

// ReplayEvent resets a failed event to 'received' for re-processing.
func (h *IntegrationHandler) ReplayEvent(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid event ID")
		return
	}

	err = h.q.ReplayInboxEvent(c.Request.Context(), sqlc.ReplayInboxEventParams{
		ID:        id,
		CompanyID: companyID,
	})
	if err != nil {
		response.NotFound(c, "event not found or not in failed state")
		return
	}
	response.OK(c, gin.H{"message": "event queued for replay"})
}

// ReplayAllFailed resets all failed events to 'received' for re-processing.
func (h *IntegrationHandler) ReplayAllFailed(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	count, err := h.q.ReplayAllFailedInboxEvents(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, "failed to replay events")
		return
	}
	response.OK(c, gin.H{"replayed": count})
}
