package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// SessionHandler handles reconciliation session endpoints.
type SessionHandler struct {
	svc       *service.SessionService
	reportSvc *service.ReportService
}

// NewSessionHandler creates a session handler.
func NewSessionHandler(svc *service.SessionService, reportSvc *service.ReportService) *SessionHandler {
	return &SessionHandler{svc: svc, reportSvc: reportSvc}
}

type createSessionRequest struct {
	Period   string  `json:"period" binding:"required"`
	ReportID *string `json:"report_id"`
}

// CreateSession handles POST /api/v1/reconciliation/sessions.
func (h *SessionHandler) CreateSession(c *gin.Context) {
	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	var reportID *uuid.UUID
	if req.ReportID != nil {
		id, err := uuid.Parse(*req.ReportID)
		if err == nil {
			reportID = &id
		}
	}

	result, err := h.svc.CreateSession(c.Request.Context(), companyID, userID, req.Period, reportID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// ListSessions handles GET /api/v1/reconciliation/sessions.
func (h *SessionHandler) ListSessions(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	sessions, total, err := h.svc.ListSessions(c.Request.Context(), companyID, p.Limit, p.Offset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, sessions, int(total), p.Page, p.Limit)
}

// GetSession handles GET /api/v1/reconciliation/sessions/:id.
func (h *SessionHandler) GetSession(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	session, err := h.svc.GetSession(c.Request.Context(), id, companyID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, session)
}

// DeleteSession handles DELETE /api/v1/reconciliation/sessions/:id.
func (h *SessionHandler) DeleteSession(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	if err := h.svc.DeleteSession(c.Request.Context(), id, companyID); err != nil {
		if err.Error() == "can only delete draft sessions" {
			response.Conflict(c, err.Error())
		} else {
			response.NotFound(c, err.Error())
		}
		return
	}

	response.OK(c, gin.H{"deleted": true})
}

type addFileRequest struct {
	FileID         string                   `json:"file_id" binding:"required"`
	Filename       string                   `json:"filename"`
	SourceType     string                   `json:"source_type" binding:"required"`
	SheetName      string                   `json:"sheet_name"`
	ColumnMappings map[string]string        `json:"column_mappings"`
	Rows           []map[string]interface{} `json:"rows" binding:"required"`
}

// AddFile handles POST /api/v1/reconciliation/sessions/:id/files.
func (h *SessionHandler) AddFile(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}

	var req addFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	result, err := h.svc.AddFile(c.Request.Context(), sessionID, companyID, service.AddFileInput{
		FileID:         req.FileID,
		Filename:       req.Filename,
		SourceType:     req.SourceType,
		SheetName:      req.SheetName,
		ColumnMappings: req.ColumnMappings,
		Rows:           req.Rows,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

type classifyRequest struct {
	Force bool `json:"force"`
}

// Classify handles POST /api/v1/reconciliation/sessions/:id/classify.
func (h *SessionHandler) Classify(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}

	var req classifyRequest
	_ = c.ShouldBindJSON(&req) // optional body

	companyID := middleware.GetCompanyID(c)

	result, err := h.svc.ClassifySession(c.Request.Context(), sessionID, companyID, req.Force)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

// ListTransactions handles GET /api/v1/reconciliation/sessions/:id/transactions.
func (h *SessionHandler) ListTransactions(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}

	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	filters := map[string]string{
		"vat_type":       c.Query("vat_type"),
		"category":       c.Query("category"),
		"source_type":    c.Query("source_type"),
		"match_status":   c.Query("match_status"),
		"min_confidence": c.Query("min_confidence"),
		"search":         c.Query("search"),
	}

	txns, total, err := h.svc.ListTransactions(c.Request.Context(), sessionID, companyID, p.Limit, p.Offset, filters)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, txns, int(total), p.Page, p.Limit)
}

type updateTransactionRequest struct {
	VATType  *string `json:"vat_type"`
	Category *string `json:"category"`
	TIN      *string `json:"tin"`
}

// UpdateTransaction handles PATCH /api/v1/reconciliation/sessions/:id/transactions/:txnId.
func (h *SessionHandler) UpdateTransaction(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}
	txnID, err := uuid.Parse(c.Param("txnId"))
	if err != nil {
		response.BadRequest(c, "invalid transaction id")
		return
	}

	var req updateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)

	updates := make(map[string]interface{})
	if req.VATType != nil {
		updates["vat_type"] = *req.VATType
	}
	if req.Category != nil {
		updates["category"] = *req.Category
	}
	if req.TIN != nil {
		updates["tin"] = *req.TIN
	}

	result, err := h.svc.UpdateTransaction(c.Request.Context(), txnID, sessionID, companyID, updates)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, result)
}

// DetectAnomalies handles POST /api/v1/reconciliation/sessions/:id/detect-anomalies.
func (h *SessionHandler) DetectAnomalies(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	result, err := h.svc.DetectAnomalies(c.Request.Context(), sessionID, companyID, userID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

// ListAnomalies handles GET /api/v1/reconciliation/sessions/:id/anomalies.
func (h *SessionHandler) ListAnomalies(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}

	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	var statusFilter *string
	if s := c.Query("status"); s != "" {
		statusFilter = &s
	}

	anomalies, total, err := h.svc.ListAnomalies(c.Request.Context(), sessionID, companyID, p.Limit, p.Offset, statusFilter)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, anomalies, int(total), p.Page, p.Limit)
}

type resolveAnomalyRequest struct {
	Status         string  `json:"status" binding:"required"`
	ResolutionNote *string `json:"resolution_note"`
}

// ResolveAnomaly handles PATCH /api/v1/reconciliation/sessions/:id/anomalies/:anomalyId.
func (h *SessionHandler) ResolveAnomaly(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}
	anomalyID, err := uuid.Parse(c.Param("anomalyId"))
	if err != nil {
		response.BadRequest(c, "invalid anomaly id")
		return
	}

	var req resolveAnomalyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	result, err := h.svc.ResolveAnomaly(c.Request.Context(), anomalyID, sessionID, companyID, userID, req.Status, req.ResolutionNote)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, result)
}

// GetSummary handles GET /api/v1/reconciliation/sessions/:id/summary.
func (h *SessionHandler) GetSummary(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	summary, err := h.svc.GetVATSummary(c.Request.Context(), sessionID, companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, summary)
}

type reconcileRequest struct {
	ReportID          *string `json:"report_id"`
	AmountTolerance   float64 `json:"amount_tolerance"`
	DateToleranceDays int     `json:"date_tolerance_days"`
}

// Reconcile handles POST /api/v1/reconciliation/sessions/:id/reconcile.
func (h *SessionHandler) Reconcile(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}

	var req reconcileRequest
	_ = c.ShouldBindJSON(&req) // optional body

	if req.AmountTolerance <= 0 {
		req.AmountTolerance = 0.01
	}
	if req.DateToleranceDays <= 0 {
		req.DateToleranceDays = 3
	}

	companyID := middleware.GetCompanyID(c)

	var reportID *uuid.UUID
	if req.ReportID != nil {
		if id, err := uuid.Parse(*req.ReportID); err == nil {
			reportID = &id
		}
	}

	result, err := h.svc.ReconcileSession(c.Request.Context(), sessionID, companyID, reportID, req.AmountTolerance, req.DateToleranceDays)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

type generateReportRequest struct {
	ReportType string `json:"report_type"`
}

// GenerateReport handles POST /api/v1/reconciliation/sessions/:id/generate-report.
func (h *SessionHandler) GenerateReport(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}

	var req generateReportRequest
	_ = c.ShouldBindJSON(&req)
	if req.ReportType == "" {
		req.ReportType = c.DefaultQuery("report_type", "BIR_2550M")
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	report, err := h.reportSvc.GenerateFromSession(c.Request.Context(), sessionID, companyID, userID, req.ReportType)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, report)
}

// ExportPDF handles GET /api/v1/reconciliation/sessions/:id/export-pdf.
func (h *SessionHandler) ExportPDF(c *gin.Context) {
	// For now, return the session summary as JSON (PDF generation can be added later)
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	session, err := h.svc.GetSession(c.Request.Context(), sessionID, companyID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	if session.Summary == nil {
		response.BadRequest(c, "no summary available — run reconciliation first")
		return
	}

	// TODO: Generate actual PDF using fpdf
	response.OK(c, gin.H{
		"message":  "PDF export not yet implemented, use /export for CSV",
		"session":  session,
	})
}

// Export handles GET /api/v1/reconciliation/sessions/:id/export.
func (h *SessionHandler) Export(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid session id")
		return
	}

	companyID := middleware.GetCompanyID(c)

	rows, err := h.svc.ExportTransactionsCSV(c.Request.Context(), sessionID, companyID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	session, _ := h.svc.GetSession(c.Request.Context(), sessionID, companyID)
	period := "export"
	if session != nil {
		period = session.Period
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=reconciliation_%s.csv", period))

	w := csv.NewWriter(c.Writer)
	for _, row := range rows {
		_ = w.Write(row)
	}
	w.Flush()

	c.Status(http.StatusOK)
}
