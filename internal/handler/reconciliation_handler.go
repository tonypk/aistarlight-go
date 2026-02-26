package handler

import (
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"

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
// Accepts two files: "records_file" (sales/purchases) and "bank_file" (bank statement).
func (h *ReconciliationHandler) Process(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil { // 32MB
		response.BadRequest(c, "invalid multipart form: "+err.Error())
		return
	}

	period := c.PostForm("period")
	if period == "" {
		response.BadRequest(c, "period is required")
		return
	}

	amountTolerance := 0.01
	if v := c.PostForm("amount_tolerance"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			amountTolerance = f
		}
	}
	dateToleranceDays := 3
	if v := c.PostForm("date_tolerance_days"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			dateToleranceDays = d
		}
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	var records []map[string]interface{}
	var bankColumns []string
	var bankRows []map[string]interface{}
	var sourceFiles []string

	// Read records file (sales/purchases)
	if recFile, header, err := c.Request.FormFile("records_file"); err == nil {
		defer recFile.Close()
		content, err := io.ReadAll(recFile)
		if err != nil {
			response.BadRequest(c, "failed to read records file: "+err.Error())
			return
		}
		filename := header.Filename
		sourceFiles = append(sourceFiles, filename)

		parsed, err := service.ParseUploadedFile(content, filename)
		if err != nil {
			response.BadRequest(c, "failed to parse records file: "+err.Error())
			return
		}
		for _, sheet := range parsed.Sheets {
			if len(sheet.Preview) > 0 {
				records = sheet.Preview
				break
			}
		}
		// Try to get all rows
		ext := strings.ToLower(filepath.Ext(filename))
		if ext == ".xlsx" || ext == ".xls" || ext == ".csv" {
			for sheetName := range parsed.Sheets {
				allRows, err := service.ParseUploadedFileAllRows(content, filename, sheetName)
				if err == nil && len(allRows) > 0 {
					records = allRows
					break
				}
			}
		}
		slog.Info("parsed records file", "filename", filename, "rows", len(records))
	}

	// Read bank statement file
	if bankFile, header, err := c.Request.FormFile("bank_file"); err == nil {
		defer bankFile.Close()
		content, err := io.ReadAll(bankFile)
		if err != nil {
			response.BadRequest(c, "failed to read bank file: "+err.Error())
			return
		}
		filename := header.Filename
		sourceFiles = append(sourceFiles, filename)

		parsed, err := service.ParseUploadedFile(content, filename)
		if err != nil {
			response.BadRequest(c, "failed to parse bank file: "+err.Error())
			return
		}
		for _, sheet := range parsed.Sheets {
			bankColumns = sheet.Columns
			if len(sheet.Preview) > 0 {
				bankRows = sheet.Preview
			}
			break
		}
		// Try all rows
		ext := strings.ToLower(filepath.Ext(filename))
		if ext == ".xlsx" || ext == ".xls" || ext == ".csv" {
			for sheetName := range parsed.Sheets {
				allRows, err := service.ParseUploadedFileAllRows(content, filename, sheetName)
				if err == nil && len(allRows) > 0 {
					bankRows = allRows
					break
				}
			}
		}
		slog.Info("parsed bank file", "filename", filename, "rows", len(bankRows))
	}

	if len(records) == 0 && len(bankRows) == 0 {
		response.BadRequest(c, "no data found — please upload at least one file with records or bank entries")
		return
	}

	input := service.CreateBatchInput{
		CompanyID:         companyID,
		CreatedBy:         userID,
		Period:            period,
		AmountTolerance:   amountTolerance,
		DateToleranceDays: dateToleranceDays,
		SourceFiles:       sourceFiles,
		Records:           records,
		BankColumns:       bankColumns,
		BankRows:          bankRows,
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
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid batch id")
		return
	}

	var req struct {
		SuggestionIndex int `json:"suggestion_index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.UpdateSuggestionStatus(c.Request.Context(), id, req.SuggestionIndex, "accepted")
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
}

// RejectSuggestion handles POST /api/v1/bank-recon/batches/:id/reject-suggestion.
func (h *ReconciliationHandler) RejectSuggestion(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid batch id")
		return
	}

	var req struct {
		SuggestionIndex int `json:"suggestion_index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.UpdateSuggestionStatus(c.Request.Context(), id, req.SuggestionIndex, "rejected")
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.OK(c, result)
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
