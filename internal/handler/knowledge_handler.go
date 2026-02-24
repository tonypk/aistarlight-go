package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// KnowledgeHandler handles knowledge base endpoints.
type KnowledgeHandler struct {
	knowledge *service.KnowledgeService
}

// NewKnowledgeHandler creates a knowledge handler.
func NewKnowledgeHandler(knowledge *service.KnowledgeService) *KnowledgeHandler {
	return &KnowledgeHandler{knowledge: knowledge}
}

// List handles GET /api/v1/knowledge.
func (h *KnowledgeHandler) List(c *gin.Context) {
	p := pagination.Parse(c)
	category := c.Query("category")

	var cat *string
	if category != "" {
		cat = &category
	}

	results, err := h.knowledge.RetrieveRelevant(c.Request.Context(), "", cat, p.Limit)
	if err != nil {
		// Return empty list instead of error when no knowledge found
		response.Paginated(c, []service.KnowledgeResult{}, 0, p.Page, p.Limit)
		return
	}

	response.Paginated(c, results, len(results), p.Page, p.Limit)
}

// Stats handles GET /api/v1/knowledge/stats.
func (h *KnowledgeHandler) Stats(c *gin.Context) {
	stats, err := h.knowledge.GetStats(c.Request.Context())
	if err != nil {
		response.OK(c, gin.H{
			"total":           0,
			"with_embeddings": 0,
			"categories":      map[string]int64{},
		})
		return
	}

	response.OK(c, stats)
}
