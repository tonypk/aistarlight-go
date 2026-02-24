package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// JournalBridgeHandler handles journal generation bridge endpoints.
type JournalBridgeHandler struct {
	gen *service.JournalGenerator
}

// NewJournalBridgeHandler creates a JournalBridgeHandler.
func NewJournalBridgeHandler(gen *service.JournalGenerator) *JournalBridgeHandler {
	return &JournalBridgeHandler{gen: gen}
}

type generateJournalRequest struct {
	TransactionIDs []string `json:"transaction_ids"`
	SessionID      string   `json:"session_id"`
}

// Generate handles POST /api/v1/journals/generate
func (h *JournalBridgeHandler) Generate(c *gin.Context) {
	var req generateJournalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	// If session_id provided, generate for all unlinked transactions in session
	if req.SessionID != "" {
		sessionID, err := uuid.Parse(req.SessionID)
		if err != nil {
			response.BadRequest(c, "invalid session_id")
			return
		}

		entries, err := h.gen.GenerateFromSession(c.Request.Context(), companyID, sessionID, userID)
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}

		response.Created(c, gin.H{
			"journal_entries": entries,
			"count":           len(entries),
		})
		return
	}

	// If transaction_ids provided, generate for specific transactions
	if len(req.TransactionIDs) > 0 {
		txnIDs := make([]uuid.UUID, len(req.TransactionIDs))
		for i, idStr := range req.TransactionIDs {
			id, err := uuid.Parse(idStr)
			if err != nil {
				response.BadRequest(c, "invalid transaction_id: "+idStr)
				return
			}
			txnIDs[i] = id
		}

		entries, err := h.gen.BatchGenerate(c.Request.Context(), companyID, txnIDs, userID)
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}

		response.Created(c, gin.H{
			"journal_entries": entries,
			"count":           len(entries),
		})
		return
	}

	response.BadRequest(c, "either session_id or transaction_ids required")
}
