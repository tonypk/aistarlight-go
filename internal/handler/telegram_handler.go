package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// TelegramHandler handles Telegram-related API endpoints.
type TelegramHandler struct {
	q       *sqlc.Queries
	botName string
}

// NewTelegramHandler creates a TelegramHandler.
func NewTelegramHandler(q *sqlc.Queries, botName string) *TelegramHandler {
	return &TelegramHandler{q: q, botName: botName}
}

// GenerateLinkToken creates a one-time link token and returns the deep link URL.
func (h *TelegramHandler) GenerateLinkToken(c *gin.Context) {
	userID := middleware.GetUserID(c)
	companyID := middleware.GetCompanyID(c)

	token, err := generateToken(8)
	if err != nil {
		response.InternalError(c, "failed to generate token")
		return
	}

	lt, err := h.q.CreateLinkToken(c.Request.Context(), sqlc.CreateLinkTokenParams{
		UserID:    userID,
		CompanyID: companyID,
		Token:     token,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	if err != nil {
		response.InternalError(c, "failed to create link token")
		return
	}

	deepLink := fmt.Sprintf("https://t.me/%s?start=lt_%s", h.botName, lt.Token)

	response.OK(c, gin.H{
		"token":      lt.Token,
		"deep_link":  deepLink,
		"expires_at": lt.ExpiresAt.Format(time.RFC3339),
	})
}

// GetBindingStatus returns the current user's Telegram binding status.
func (h *TelegramHandler) GetBindingStatus(c *gin.Context) {
	userID := middleware.GetUserID(c)

	tu, err := h.q.GetTelegramUserByUserID(c.Request.Context(), userID)
	if err != nil {
		// Not linked
		response.OK(c, gin.H{
			"linked":   false,
			"bot_name": h.botName,
		})
		return
	}

	response.OK(c, gin.H{
		"linked":     true,
		"username":   tu.Username,
		"bot_name":   h.botName,
		"linked_at":  tu.CreatedAt.Format(time.RFC3339),
	})
}

func generateToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
