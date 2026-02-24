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
	response.OK(c, gin.H{
		"total_chunks":       0,
		"categories":         []string{"vat", "income_tax", "withholding", "compliance", "general"},
		"last_updated":       nil,
		"embedding_coverage": 0.0,
	})
}
