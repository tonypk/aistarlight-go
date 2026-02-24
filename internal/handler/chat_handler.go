package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// ChatHandler handles AI chat endpoints.
type ChatHandler struct {
	svc *service.ChatService
}

// NewChatHandler creates a chat handler.
func NewChatHandler(svc *service.ChatService) *ChatHandler {
	return &ChatHandler{svc: svc}
}

type chatMessageRequest struct {
	Message string `json:"message" binding:"required"`
}

// Message handles POST /api/v1/chat/message (non-streaming).
func (h *ChatHandler) Message(c *gin.Context) {
	var req chatMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	// Load recent history
	history, _ := h.svc.ListHistory(c.Request.Context(), companyID, 20)

	// Process message
	chatResp, err := h.svc.ProcessMessage(c.Request.Context(), req.Message, history, companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// Save user message
	_ = h.svc.SaveMessage(c.Request.Context(), companyID, userID, "user", req.Message, nil)

	// Save assistant response
	_ = h.svc.SaveMessage(c.Request.Context(), companyID, userID, "assistant", chatResp.Response, chatResp.ToolCalls)

	response.OK(c, gin.H{
		"role":       "assistant",
		"content":    chatResp.Response,
		"tool_calls": chatResp.ToolCalls,
	})
}

// Stream handles POST /api/v1/chat/stream (SSE streaming).
func (h *ChatHandler) Stream(c *gin.Context) {
	var req chatMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	// Load recent history
	history, _ := h.svc.ListHistory(c.Request.Context(), companyID, 20)

	// Save user message first
	_ = h.svc.SaveMessage(c.Request.Context(), companyID, userID, "user", req.Message, nil)

	// Process message with streaming
	tokenCh, toolResults, err := h.svc.ProcessMessageStream(c.Request.Context(), req.Message, history, companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Stream tokens and accumulate full content
	var fullContent string
	c.Stream(func(w io.Writer) bool {
		token, ok := <-tokenCh
		if !ok {
			return false
		}
		fullContent += token
		data, _ := json.Marshal(gin.H{"token": token})
		fmt.Fprintf(w, "data: %s\n\n", data)
		c.Writer.Flush()
		return true
	})

	// Send done signal with accumulated content
	doneData, _ := json.Marshal(gin.H{"done": true, "content": fullContent})
	fmt.Fprintf(c.Writer, "data: %s\n\n", doneData)
	c.Writer.Flush()

	// Save assistant response with accumulated content
	var toolCalls []service.ToolCallResult
	if toolResults != nil {
		toolCalls = *toolResults
	}
	_ = h.svc.SaveMessage(c.Request.Context(), companyID, userID, "assistant", fullContent, toolCalls)
}

// History handles GET /api/v1/chat/history.
func (h *ChatHandler) History(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	p := defaultPagination()
	messages, err := h.svc.ListHistory(c.Request.Context(), companyID, p.Limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    messages,
		"meta": gin.H{
			"total": len(messages),
			"page":  1,
			"limit": p.Limit,
		},
	})
}
