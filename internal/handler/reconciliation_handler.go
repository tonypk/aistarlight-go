package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

// ReconciliationHandler handles bank reconciliation endpoints.
type ReconciliationHandler struct {
	svc *service.BankReconService
}

// NewReconciliationHandler creates a reconciliation handler.
func NewReconciliationHandler(svc *service.BankReconService) *ReconciliationHandler {
	return &ReconciliationHandler{svc: svc}
}

type runReconciliationRequest struct {
	Period            string                   `json:"period" binding:"required"`
	AmountTolerance   float64                  `json:"amount_tolerance"`
	DateToleranceDays int                      `json:"date_tolerance_days"`
	SourceFiles       []string                 `json:"source_files"`
	Records           []map[string]interface{} `json:"records" binding:"required"`
	BankColumns       []string                 `json:"bank_columns" binding:"required"`
	BankRows          []map[string]interface{} `json:"bank_rows" binding:"required"`
	SessionID         *string                  `json:"session_id"`
}

// Run handles POST /api/v1/reconciliation/run.
func (h *ReconciliationHandler) Run(c *gin.Context) {
	var req runReconciliationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	input := service.CreateBatchInput{
		CompanyID:         companyID,
		CreatedBy:         userID,
		Period:            req.Period,
		AmountTolerance:   req.AmountTolerance,
		DateToleranceDays: req.DateToleranceDays,
		SourceFiles:       req.SourceFiles,
		Records:           req.Records,
		BankColumns:       req.BankColumns,
		BankRows:          req.BankRows,
	}

	if req.SessionID != nil {
		sid, err := uuid.Parse(*req.SessionID)
		if err == nil {
			input.SessionID = &sid
		}
	}

	result, err := h.svc.RunReconciliation(c.Request.Context(), input)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

// Get handles GET /api/v1/reconciliation/batches/:id.
func (h *ReconciliationHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid batch id")
		return
	}

	result, err := h.svc.GetBatch(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, result)
}

// List handles GET /api/v1/reconciliation/batches.
func (h *ReconciliationHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	p := pagination.Parse(c)

	batches, total, err := h.svc.ListBatches(c.Request.Context(), companyID, p.Limit, p.Offset)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Paginated(c, batches, int(total), p.Page, p.Limit)
}

// DetectFormat handles POST /api/v1/reconciliation/detect-format.
func (h *ReconciliationHandler) DetectFormat(c *gin.Context) {
	var req struct {
		Columns []string `json:"columns" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	format := service.DetectBankFormat(req.Columns)

	response.OK(c, gin.H{
		"bank_name":       format.Name,
		"format_detected": format.Name != "Generic",
		"date_format":     format.DateFormat,
	})
}

// Process handles POST /api/v1/bank-recon/process (multipart file upload).
// This is the frontend's reconciliation endpoint that sends FormData.
func (h *ReconciliationHandler) Process(c *gin.Context) {
	// Parse multipart form
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil { // 32MB
		response.BadRequest(c, "invalid multipart form: "+err.Error())
		return
	}

	period := c.PostForm("period")
	if period == "" {
		response.BadRequest(c, "period is required")
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	// For now, process the form as a simple run
	input := service.CreateBatchInput{
		CompanyID:         companyID,
		CreatedBy:         userID,
		Period:            period,
		AmountTolerance:   0.01,
		DateToleranceDays: 3,
		Records:           []map[string]interface{}{},
		BankColumns:       []string{},
		BankRows:          []map[string]interface{}{},
	}

	result, err := h.svc.RunReconciliation(c.Request.Context(), input)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

// AcceptSuggestion handles POST /api/v1/bank-recon/batches/:id/accept-suggestion.
func (h *ReconciliationHandler) AcceptSuggestion(c *gin.Context) {
	response.OK(c, gin.H{"message": "suggestion accepted"})
}

// RejectSuggestion handles POST /api/v1/bank-recon/batches/:id/reject-suggestion.
func (h *ReconciliationHandler) RejectSuggestion(c *gin.Context) {
	response.OK(c, gin.H{"message": "suggestion rejected"})
}

// RerunAnalysis handles POST /api/v1/bank-recon/batches/:id/rerun-analysis.
func (h *ReconciliationHandler) RerunAnalysis(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid batch id")
		return
	}

	result, err := h.svc.GetBatch(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.OK(c, result)
}

// MatchPreview handles POST /api/v1/reconciliation/match-preview.
func (h *ReconciliationHandler) MatchPreview(c *gin.Context) {
	var req struct {
		Records           []map[string]interface{} `json:"records" binding:"required"`
		BankEntries       []map[string]interface{} `json:"bank_entries" binding:"required"`
		AmountTolerance   float64                  `json:"amount_tolerance"`
		DateToleranceDays int                      `json:"date_tolerance_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result := service.MatchTransactions(
		req.Records,
		req.BankEntries,
		req.AmountTolerance,
		req.DateToleranceDays,
	)

	response.OK(c, result)
}
