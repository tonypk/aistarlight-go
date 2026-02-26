package handler

import (
	"github.com/gin-gonic/gin"
	oai "github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
)

// HealthHandler serves AI readiness probes.
type HealthHandler struct {
	ai *oai.Client
}

// NewHealthHandler creates a HealthHandler.
func NewHealthHandler(ai *oai.Client) *HealthHandler {
	return &HealthHandler{ai: ai}
}

// AIHealth returns the availability status of each AI-powered feature.
func (h *HealthHandler) AIHealth(c *gin.Context) {
	enabled := h.ai != nil

	features := map[string]bool{
		"chat":           enabled,
		"classification": enabled,
		"column_mapping": enabled,
		"anomaly_detection": enabled,
		"knowledge_rag":  enabled,
	}

	provider := ""
	model := ""
	if enabled {
		provider = "openai"
		model = "gpt-4o-mini"
	}

	response.OK(c, gin.H{
		"ai_enabled": enabled,
		"provider":   provider,
		"model":      model,
		"features":   features,
	})
}
