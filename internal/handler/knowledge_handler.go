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

// Create handles POST /api/v1/knowledge.
func (h *KnowledgeHandler) Create(c *gin.Context) {
	var req struct {
		Content  string `json:"content" binding:"required"`
		Source   string `json:"source"`
		Category string `json:"category"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.knowledge.AddChunk(c.Request.Context(), req.Source, req.Category, req.Content)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
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
