package handler

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// InvoiceHandler handles invoice API endpoints.
type InvoiceHandler struct {
	svc     *service.InvoiceService
	company *service.CompanyService
}

// NewInvoiceHandler creates an InvoiceHandler.
func NewInvoiceHandler(svc *service.InvoiceService, company *service.CompanyService) *InvoiceHandler {
	return &InvoiceHandler{svc: svc, company: company}
}

// Create handles POST /api/v1/invoices.
func (h *InvoiceHandler) Create(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	var req service.CreateInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.CustomerName == "" {
		response.BadRequest(c, "customer_name is required")
		return
	}
	if req.InvoiceDate == "" {
		response.BadRequest(c, "invoice_date is required")
		return
	}

	result, err := h.svc.Create(c.Request.Context(), companyID, userID, req)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "create invoice failed", "error", err)
		response.InternalError(c, "failed to create invoice")
		return
	}

	response.Created(c, result)
}

// Get handles GET /api/v1/invoices/:id.
func (h *InvoiceHandler) Get(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	invoiceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid invoice ID")
		return
	}

	result, err := h.svc.Get(c.Request.Context(), companyID, invoiceID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.NotFound(c, "invoice not found")
		} else {
			slog.ErrorContext(c.Request.Context(), "get invoice failed", "error", err)
			response.InternalError(c, "failed to get invoice")
		}
		return
	}

	response.OK(c, result)
}

// List handles GET /api/v1/invoices.
func (h *InvoiceHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	status := c.Query("status")
	invoiceType := c.Query("type")

	offset := (page - 1) * limit
	invoices, total, err := h.svc.List(c.Request.Context(), companyID, int32(limit), int32(offset), status, invoiceType)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "list invoices failed", "error", err)
		response.InternalError(c, "failed to list invoices")
		return
	}

	response.Paginated(c, invoices, int(total), page, limit)
}

// UpdateStatus handles PATCH /api/v1/invoices/:id/status.
func (h *InvoiceHandler) UpdateStatus(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	invoiceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid invoice ID")
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	allowedStatuses := map[string]bool{
		"draft": true, "sent": true, "paid": true, "cancelled": true, "overdue": true,
	}
	if !allowedStatuses[req.Status] {
		response.BadRequest(c, "invalid status value")
		return
	}

	if err := h.svc.UpdateStatus(c.Request.Context(), companyID, invoiceID, req.Status); err != nil {
		slog.ErrorContext(c.Request.Context(), "update invoice status failed", "error", err)
		response.InternalError(c, "failed to update invoice status")
		return
	}

	response.OK(c, map[string]string{"status": req.Status})
}

// Delete handles DELETE /api/v1/invoices/:id.
func (h *InvoiceHandler) Delete(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	invoiceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid invoice ID")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), companyID, invoiceID); err != nil {
		if strings.Contains(err.Error(), "cannot delete") {
			response.BadRequest(c, err.Error())
		} else {
			slog.ErrorContext(c.Request.Context(), "delete invoice failed", "error", err)
			response.InternalError(c, "failed to delete invoice")
		}
		return
	}

	response.OK(c, map[string]string{"deleted": invoiceID.String()})
}

// ExportEIS handles GET /api/v1/invoices/:id/eis-export.
func (h *InvoiceHandler) ExportEIS(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	invoiceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid invoice ID")
		return
	}

	inv, err := h.svc.Get(c.Request.Context(), companyID, invoiceID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	// Get company details for seller info
	company, err := h.company.GetByID(c.Request.Context(), companyID)
	if err != nil {
		response.InternalError(c, "failed to get company details")
		return
	}

	data, err := service.ExportEIS(inv.Invoice, inv.Items, company.CompanyName, derefOrEmpty(company.TINNumber))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	safeNum := strings.ReplaceAll(inv.Invoice.InvoiceNumber, `"`, "_")
	filename := fmt.Sprintf("EIS_%s.json", safeNum)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(200, "application/json", data)
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
