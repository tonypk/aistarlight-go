package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/agent"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
)

// AgentHandler handles AI agent endpoints.
type AgentHandler struct {
	runtime *agent.Runtime
}

// NewAgentHandler creates an agent handler.
func NewAgentHandler(runtime *agent.Runtime) *AgentHandler {
	return &AgentHandler{runtime: runtime}
}

type agentStreamRequest struct {
	Content      string                 `json:"content" binding:"required"`
	ThreadID     *string                `json:"thread_id"`
	WorkflowType string                 `json:"workflow_type"`
	EntityType   string                 `json:"entity_type"`
	EntityID     *string                `json:"entity_id"`
	Context      map[string]interface{} `json:"context"`
}

// ListAgents handles GET /api/v1/agents.
func (h *AgentHandler) ListAgents(c *gin.Context) {
	workflowType := c.Query("workflow_type")
	var agents []agent.AgentInfo
	if workflowType != "" {
		agents = h.runtime.Registry().ListForWorkflow(workflowType)
	} else {
		agents = h.runtime.Registry().ListAll()
	}
	response.OK(c, agents)
}

// GetAgent handles GET /api/v1/agents/:agentId.
func (h *AgentHandler) GetAgent(c *gin.Context) {
	agentID := c.Param("agentId")
	def, ok := h.runtime.Registry().Get(agentID)
	if !ok {
		response.NotFound(c, "agent not found")
		return
	}
	response.OK(c, def.Info())
}

// Stream handles POST /api/v1/agents/:agentId/stream (SSE).
func (h *AgentHandler) Stream(c *gin.Context) {
	agentID := c.Param("agentId")
	var req agentStreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)
	jurisdiction := middleware.GetJurisdiction(c)

	agentReq := agent.AgentRequest{
		AgentID:      agentID,
		Content:      req.Content,
		CompanyID:    companyID,
		UserID:       userID,
		Jurisdiction: jurisdiction,
		WorkflowType: req.WorkflowType,
		EntityType:   req.EntityType,
		Context:      req.Context,
	}

	if req.ThreadID != nil {
		if tid, err := uuid.Parse(*req.ThreadID); err == nil {
			agentReq.ThreadID = &tid
		}
	}
	if req.EntityID != nil {
		if eid, err := uuid.Parse(*req.EntityID); err == nil {
			agentReq.EntityID = &eid
		}
	}

	eventCh, err := h.runtime.ProcessStream(c.Request.Context(), agentReq)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Stream events
	c.Stream(func(w io.Writer) bool {
		evt, ok := <-eventCh
		if !ok {
			return false
		}
		data, _ := json.Marshal(evt)
		fmt.Fprintf(w, "data: %s\n\n", data)
		c.Writer.Flush()
		return !evt.Done
	})
}

// ListThreads handles GET /api/v1/agents/:agentId/threads.
func (h *AgentHandler) ListThreads(c *gin.Context) {
	agentID := c.Param("agentId")
	companyID := middleware.GetCompanyID(c)

	threads, err := h.runtime.ListThreads(c.Request.Context(), companyID, agentID, 50, 0)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    threads,
	})
}

// ThreadMessages handles GET /api/v1/agents/:agentId/threads/:threadId/messages.
func (h *AgentHandler) ThreadMessages(c *gin.Context) {
	threadIDStr := c.Param("threadId")
	threadID, err := uuid.Parse(threadIDStr)
	if err != nil {
		response.BadRequest(c, "invalid thread_id")
		return
	}

	messages, err := h.runtime.ThreadMessages(c.Request.Context(), threadID, 100)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    messages,
	})
}
