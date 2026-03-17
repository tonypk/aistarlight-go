package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tonypk/aistarlight-go/internal/handler/response"
	sqlc "github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// WebhookHandler receives inbound webhooks from integrated systems.
type WebhookHandler struct {
	q      *sqlc.Queries
	logger *slog.Logger
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(q *sqlc.Queries, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{q: q, logger: logger}
}

// ReceiveAIGoNHR handles POST /api/v1/webhooks/aigonhr.
// Verifies HMAC signature and inserts into integration_event_inbox.
func (h *WebhookHandler) ReceiveAIGoNHR(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)) // 1MB limit
	if err != nil {
		response.BadRequest(c, "failed to read body")
		return
	}

	eventType := c.GetHeader("X-Event-Type")
	eventID := c.GetHeader("X-Event-ID")
	signature := c.GetHeader("X-Webhook-Signature")

	if eventType == "" || eventID == "" {
		response.BadRequest(c, "missing X-Event-Type or X-Event-ID header")
		return
	}

	// Extract company from Bearer token to find integration source
	apiKey := ""
	if auth := c.GetHeader("Authorization"); len(auth) > 7 {
		apiKey = auth[7:] // strip "Bearer "
	}
	if apiKey == "" {
		response.Unauthorized(c, "missing authorization")
		return
	}

	// Look up integration source by hashed API key
	// For now, iterate sources — in production, use a key-prefix lookup
	sources, err := h.q.ListIntegrationSourcesBySystem(c.Request.Context(), "aigonhr")
	if err != nil {
		h.logger.Error("failed to list integration sources", "error", err)
		response.InternalError(c, "internal error")
		return
	}

	var matchedSource *sqlc.IntegrationSource
	for _, src := range sources {
		if src.ApiKeyHash == apiKey && src.Status == "active" {
			matchedSource = &src
			break
		}
	}
	if matchedSource == nil {
		response.Unauthorized(c, "invalid or inactive integration source")
		return
	}

	// Verify HMAC signature
	mac := hmac.New(sha256.New, []byte(matchedSource.WebhookSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		response.Unauthorized(c, "invalid webhook signature")
		return
	}

	// Insert into event inbox (idempotent via unique constraint)
	inserted, err := h.q.InsertEventInbox(c.Request.Context(), sqlc.InsertEventInboxParams{
		CompanyID:    matchedSource.CompanyID,
		SourceSystem: "aigonhr",
		EventID:      eventID,
		EventType:    eventType,
		Payload:      body,
	})
	if err != nil {
		// ON CONFLICT DO NOTHING — if no row returned, it's a duplicate
		h.logger.Info("duplicate event ignored", "event_id", eventID)
		c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "duplicate": true})
		return
	}

	h.logger.Info("webhook event received",
		"event_id", eventID,
		"event_type", eventType,
		"inbox_id", inserted.ID,
		"company_id", matchedSource.CompanyID,
	)

	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "inbox_id": inserted.ID})
}
