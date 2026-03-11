package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// ReceiptBridgeHandler handles receipt-to-transaction bridge endpoints.
type ReceiptBridgeHandler struct {
	bridge *service.ReceiptBridge
}

// NewReceiptBridgeHandler creates a ReceiptBridgeHandler.
func NewReceiptBridgeHandler(bridge *service.ReceiptBridge) *ReceiptBridgeHandler {
	return &ReceiptBridgeHandler{bridge: bridge}
}

type convertReceiptRequest struct {
	SessionID string `json:"session_id" binding:"required"`
}

// Convert handles POST /api/v1/receipts/:id/convert
func (h *ReceiptBridgeHandler) Convert(c *gin.Context) {
	receiptID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid receipt batch ID")
		return
	}

	var req convertReceiptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		response.BadRequest(c, "invalid session_id")
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	transactions, err := h.bridge.ConvertReceiptToTransactions(
		c.Request.Context(), companyID, receiptID, sessionID, nil, userID,
	)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
	})
}
