package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// ReceiptHandler handles receipt processing endpoints.
type ReceiptHandler struct {
	svc *service.ReceiptService
}

// NewReceiptHandler creates a receipt handler.
func NewReceiptHandler(svc *service.ReceiptService) *ReceiptHandler {
	return &ReceiptHandler{svc: svc}
}

type uploadReceiptRequest struct {
	Period     string   `json:"period" binding:"required"`
	ReportType string   `json:"report_type"`
	ImagePaths []string `json:"image_paths" binding:"required"`
}

// Upload handles POST /api/v1/receipts/upload.
func (h *ReceiptHandler) Upload(c *gin.Context) {
	var req uploadReceiptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if len(req.ImagePaths) > 50 {
		response.BadRequest(c, "maximum 50 images per batch")
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	batch, results, err := h.svc.ProcessBatch(
		c.Request.Context(),
		companyID, userID,
		req.ImagePaths,
		req.Period,
		req.ReportType,
	)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, gin.H{
		"batch":   batch,
		"results": results,
	})
}

// Get handles GET /api/v1/receipts/batches/:id.
func (h *ReceiptHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid batch id")
		return
	}

	batch, err := h.svc.GetBatch(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, batch)
}

// List handles GET /api/v1/receipts/batches.
func (h *ReceiptHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	batches, total, err := h.svc.ListBatches(c.Request.Context(), companyID, p.Limit, p.Offset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, batches, int(total), p.Page, p.Limit)
}
