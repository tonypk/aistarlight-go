package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// TagHandler handles tag CRUD and transaction-tag endpoints.
type TagHandler struct {
	tags *service.TagService
}

// NewTagHandler creates a TagHandler.
func NewTagHandler(tags *service.TagService) *TagHandler {
	return &TagHandler{tags: tags}
}

type createTagRequest struct {
	Name  string `json:"name" binding:"required"`
	Color string `json:"color"`
}

// Create handles POST /api/v1/tags.
func (h *TagHandler) Create(c *gin.Context) {
	var req createTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	tag, err := h.tags.Create(c.Request.Context(), companyID, req.Name, req.Color)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, tag)
}

// List handles GET /api/v1/tags.
func (h *TagHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)
	search := c.Query("search")

	tags, total, err := h.tags.List(c.Request.Context(), companyID, search, p.Limit, p.Offset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, tags, int(total), p.Page, p.Limit)
}

type updateTagRequest struct {
	Name  string `json:"name" binding:"required"`
	Color string `json:"color" binding:"required"`
}

// Update handles PUT /api/v1/tags/:id.
func (h *TagHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid tag id")
		return
	}

	var req updateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	tag, err := h.tags.Update(c.Request.Context(), id, companyID, req.Name, req.Color)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, tag)
}

// Delete handles DELETE /api/v1/tags/:id.
func (h *TagHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid tag id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	if err := h.tags.Delete(c.Request.Context(), id, companyID); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, nil)
}

type setTransactionTagsRequest struct {
	TagIDs []string `json:"tag_ids" binding:"required"`
}

// SetTransactionTags handles PUT /api/v1/transactions/:id/tags.
func (h *TagHandler) SetTransactionTags(c *gin.Context) {
	txnID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid transaction id")
		return
	}

	var req setTransactionTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	tagIDs := make([]uuid.UUID, 0, len(req.TagIDs))
	for _, s := range req.TagIDs {
		id, err := uuid.Parse(s)
		if err != nil {
			response.BadRequest(c, "invalid tag_id: "+s)
			return
		}
		tagIDs = append(tagIDs, id)
	}

	if err := h.tags.SetTransactionTags(c.Request.Context(), txnID, tagIDs); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// Return the updated tags
	tags, err := h.tags.GetTransactionTags(c.Request.Context(), txnID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, tags)
}

// GetTransactionTags handles GET /api/v1/transactions/:id/tags.
func (h *TagHandler) GetTransactionTags(c *gin.Context) {
	txnID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid transaction id")
		return
	}

	tags, err := h.tags.GetTransactionTags(c.Request.Context(), txnID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, tags)
}
