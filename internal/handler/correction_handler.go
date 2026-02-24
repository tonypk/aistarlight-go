package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// CorrectionHandler handles correction endpoints.
type CorrectionHandler struct {
	corrections *service.CorrectionService
	analyzer    *service.CorrectionAnalyzer
}

// NewCorrectionHandler creates a correction handler.
func NewCorrectionHandler(corrections *service.CorrectionService, analyzer *service.CorrectionAnalyzer) *CorrectionHandler {
	return &CorrectionHandler{corrections: corrections, analyzer: analyzer}
}

type recordCorrectionRequest struct {
	EntityType string  `json:"entity_type" binding:"required"`
	EntityID   string  `json:"entity_id" binding:"required"`
	FieldName  string  `json:"field_name" binding:"required"`
	OldValue   *string `json:"old_value"`
	NewValue   string  `json:"new_value" binding:"required"`
	Reason     *string `json:"reason"`
}

// Record handles POST /api/v1/corrections.
func (h *CorrectionHandler) Record(c *gin.Context) {
	var req recordCorrectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	entityID, err := uuid.Parse(req.EntityID)
	if err != nil {
		response.BadRequest(c, "invalid entity_id")
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	result, err := h.corrections.RecordCorrection(c.Request.Context(), service.RecordCorrectionInput{
		CompanyID:  companyID,
		UserID:     userID,
		EntityType: req.EntityType,
		EntityID:   entityID,
		FieldName:  req.FieldName,
		OldValue:   req.OldValue,
		NewValue:   req.NewValue,
		Reason:     req.Reason,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// List handles GET /api/v1/corrections.
func (h *CorrectionHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	corrections, total, err := h.corrections.ListCorrections(c.Request.Context(), companyID, p.Limit, p.Offset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, corrections, int(total), p.Page, p.Limit)
}

// GetByEntity handles GET /api/v1/corrections/entity/:type/:id.
func (h *CorrectionHandler) GetByEntity(c *gin.Context) {
	entityType := c.Param("type")
	entityID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid entity id")
		return
	}

	corrections, err := h.corrections.GetEntityCorrections(c.Request.Context(), entityType, entityID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, corrections)
}

// Stats handles GET /api/v1/corrections/stats.
func (h *CorrectionHandler) Stats(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	stats, err := h.corrections.GetCorrectionStats(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, stats)
}

// AnalyzePatterns handles POST /api/v1/corrections/analyze.
func (h *CorrectionHandler) AnalyzePatterns(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	candidates, err := h.analyzer.AnalyzeCorrections(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, candidates)
}

// PersistRules handles POST /api/v1/corrections/persist-rules.
func (h *CorrectionHandler) PersistRules(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	var req struct {
		Candidates []service.RuleCandidate `json:"candidates" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	rules, err := h.analyzer.PersistCandidateRules(c.Request.Context(), companyID, req.Candidates)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, rules)
}

// LearningStats handles GET /api/v1/corrections/learning-stats and /learning/stats.
func (h *CorrectionHandler) LearningStats(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	stats, err := h.analyzer.GetLearningStats(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, stats)
}

// ListRules handles GET /api/v1/corrections/rules.
func (h *CorrectionHandler) ListRules(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	// Use the analyzer to get learning stats which includes persisted rules count
	stats, err := h.analyzer.GetLearningStats(c.Request.Context(), companyID)
	if err != nil {
		response.OK(c, []interface{}{})
		return
	}

	response.OK(c, stats)
}

// UpdateRule handles PATCH /api/v1/corrections/rules/:ruleId.
func (h *CorrectionHandler) UpdateRule(c *gin.Context) {
	ruleID, err := uuid.Parse(c.Param("ruleId"))
	if err != nil {
		response.BadRequest(c, "invalid rule id")
		return
	}

	var req struct {
		IsActive *bool `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	if err := h.analyzer.UpdateRule(c.Request.Context(), ruleID, isActive); err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, gin.H{"updated": true})
}
