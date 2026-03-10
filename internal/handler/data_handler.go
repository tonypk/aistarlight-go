package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/config"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// DataHandler handles file upload and data processing endpoints.
type DataHandler struct {
	colMapper *service.ColumnMapperService
	memorySvc *service.MemoryService
	ai        *openai.Client
	cfg       *config.Config
	q         *sqlc.Queries
}

// NewDataHandler creates a new data handler.
func NewDataHandler(colMapper *service.ColumnMapperService, memorySvc *service.MemoryService, ai *openai.Client, cfg *config.Config, q *sqlc.Queries) *DataHandler {
	return &DataHandler{colMapper: colMapper, memorySvc: memorySvc, ai: ai, cfg: cfg, q: q}
}

var allowedExtensions = map[string]bool{
	".xlsx": true,
	".xls":  true,
	".csv":  true,
}

// Upload handles POST /api/v1/data/upload.
// Accepts multipart file upload, validates, parses, and returns structured data.
func (h *DataHandler) Upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "No file provided. Please select a file to upload.")
		return
	}
	defer file.Close()

	// Validate filename.
	if header.Filename == "" {
		response.BadRequest(c, "File has no name.")
		return
	}

	// Validate extension.
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExtensions[ext] {
		response.Err(c, http.StatusUnsupportedMediaType, fmt.Sprintf(
			"Unsupported file format: %s. Only .xlsx, .xls, and .csv files are accepted.",
			ext,
		))
		return
	}

	// Read content with size limit.
	limitReader := io.LimitReader(file, service.MaxUploadSize+1)
	content, err := io.ReadAll(limitReader)
	if err != nil {
		response.InternalError(c, "Failed to read uploaded file.")
		return
	}

	if len(content) > service.MaxUploadSize {
		response.Err(c, http.StatusRequestEntityTooLarge, fmt.Sprintf(
			"File exceeds %dMB limit (actual: %.1fMB). Please reduce file size or split into multiple files.",
			service.MaxUploadSizeMB, float64(len(content))/(1024*1024),
		))
		return
	}

	if len(content) == 0 {
		response.BadRequest(c, "File is empty. Please upload a file with data.")
		return
	}

	// Parse file with cleaning pipeline (falls back to AI/heuristic).
	companyID := middleware.GetCompanyID(c)
	cleaningParsed, err := service.ParseUploadedFileWithCleaning(
		c.Request.Context(), h.ai, h.q, content, header.Filename, companyID, "",
	)
	if err != nil {
		slog.Error("file parse failed", "error", err, "filename", header.Filename)
		response.BadRequest(c, "Could not parse file. Ensure it is a valid, non-corrupted .xlsx, .xls, or .csv file with a header row.")
		return
	}

	// Save to upload directory.
	fileID := uuid.New().String()
	uploadDir := h.cfg.UploadDir
	if uploadDir == "" {
		uploadDir = "/tmp/aistarlight-uploads"
	}
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		response.InternalError(c, "Failed to prepare upload storage.")
		return
	}

	savePath := filepath.Join(uploadDir, fileID+ext)
	if err := os.WriteFile(savePath, content, 0o640); err != nil {
		response.InternalError(c, "Failed to save uploaded file.")
		return
	}

	// Also save cleaned data as JSON so loadRowsFromUpload can find it.
	// Without this, AddFile re-parses the raw file with a different parser
	// that may produce different column names or miss trailing empty cells,
	// causing column mappings to fail (amounts = 0.00).
	type jsonSheet struct {
		Columns  []string                 `json:"columns"`
		RowCount int                      `json:"row_count"`
		Rows     []map[string]interface{} `json:"rows"`
		Preview  []map[string]interface{} `json:"preview"`
	}
	type jsonSave struct {
		Filename string                   `json:"filename"`
		Sheets   map[string]*jsonSheet    `json:"sheets"`
	}
	jsonData := jsonSave{
		Filename: header.Filename,
		Sheets:   make(map[string]*jsonSheet),
	}
	for sheetName, sheet := range cleaningParsed.Sheets {
		js := &jsonSheet{
			Columns:  sheet.Columns,
			RowCount: sheet.RowCount,
			Preview:  sheet.Preview,
		}
		// Use full DataRows from cleaning result (not just preview)
		if cr, ok := cleaningParsed.CleaningResults[sheetName]; ok && len(cr.DataRows) > 0 {
			js.Rows = cr.DataRows
			js.RowCount = len(cr.DataRows)
		}
		jsonData.Sheets[sheetName] = js
	}
	if jsonBytes, err := json.Marshal(jsonData); err == nil {
		jsonPath := filepath.Join(uploadDir, fileID+".json")
		if err := os.WriteFile(jsonPath, jsonBytes, 0o640); err != nil {
			slog.Warn("failed to save cleaned JSON", "error", err)
		}
	}

	// Extract first sheet info for convenience.
	var columns []string
	var sampleRows []map[string]interface{}
	var cleaningReport interface{}
	for sheetName, sheet := range cleaningParsed.Sheets {
		columns = sheet.Columns
		sampleRows = sheet.Preview
		if cr, ok := cleaningParsed.CleaningResults[sheetName]; ok {
			cleaningReport = cr.Report
		}
		break
	}

	resp := gin.H{
		"file_id":     fileID,
		"filename":    header.Filename,
		"size":        len(content),
		"columns":     columns,
		"sample_rows": sampleRows,
		"sheets":      cleaningParsed.Sheets,
	}
	if cleaningReport != nil {
		resp["cleaning_report"] = cleaningReport
	}
	response.OK(c, resp)
}

// Preview handles POST /api/v1/data/preview.
// Parses file without saving — for quick preview.
func (h *DataHandler) Preview(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "No file provided.")
		return
	}
	defer file.Close()

	limitReader := io.LimitReader(file, service.MaxUploadSize+1)
	content, err := io.ReadAll(limitReader)
	if err != nil {
		response.InternalError(c, "Failed to read file.")
		return
	}

	if len(content) > service.MaxUploadSize {
		response.Err(c, http.StatusRequestEntityTooLarge, fmt.Sprintf(
			"File exceeds %dMB limit.", service.MaxUploadSizeMB,
		))
		return
	}

	companyID := middleware.GetCompanyID(c)
	cleaningParsed, err := service.ParseUploadedFileWithCleaning(
		c.Request.Context(), h.ai, h.q, content, header.Filename, companyID, "",
	)
	if err != nil {
		slog.Error("file preview parse failed", "error", err, "filename", header.Filename)
		response.BadRequest(c, "Could not parse file. Ensure it is a valid, non-corrupted spreadsheet.")
		return
	}

	resp := gin.H{
		"type":   cleaningParsed.Type,
		"sheets": cleaningParsed.Sheets,
	}
	// Include cleaning reports if available
	if len(cleaningParsed.CleaningResults) > 0 {
		reports := make(map[string]interface{})
		for name, cr := range cleaningParsed.CleaningResults {
			reports[name] = cr.Report
		}
		resp["cleaning_reports"] = reports
	}
	response.OK(c, resp)
}

type suggestMappingRequest struct {
	Columns      []string                 `json:"columns" binding:"required"`
	SampleRows   []map[string]interface{} `json:"sample_rows" binding:"required"`
	ReportType   string                   `json:"report_type"`
	DataCategory string                   `json:"data_category"` // sales, purchases, general (helps AI disambiguate)
}

// parsedSheetRequest represents a single sheet from browser-side parsing.
type parsedSheetRequest struct {
	Columns  []string                 `json:"columns" binding:"required"`
	RowCount int                      `json:"row_count"`
	Rows     []map[string]interface{} `json:"rows" binding:"required"`
	Preview  []map[string]interface{} `json:"preview"`
}

// uploadParsedRequest is the payload from browser-side SheetJS parsing.
type uploadParsedRequest struct {
	Filename string                         `json:"filename" binding:"required"`
	Type     string                         `json:"type"`
	Sheets   map[string]*parsedSheetRequest `json:"sheets" binding:"required"`
}

// MaxParsedPayloadSize is the max JSON body size for pre-parsed uploads (50MB of JSON data).
const MaxParsedPayloadSize = 50 * 1024 * 1024

// UploadParsed handles POST /api/v1/data/upload-parsed.
// Receives pre-parsed data from browser-side SheetJS (no raw file upload needed).
// This enables handling of large Excel files (50-200MB) that would be too slow to upload raw.
func (h *DataHandler) UploadParsed(c *gin.Context) {
	// Limit request body size.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxParsedPayloadSize)

	var req uploadParsedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid data. Please try uploading the file again.")
		return
	}

	if len(req.Sheets) == 0 {
		response.BadRequest(c, "No sheet data found. The file may be empty.")
		return
	}

	// Save parsed data as JSON for later column mapping / processing.
	fileID := uuid.New().String()
	uploadDir := h.cfg.UploadDir
	if uploadDir == "" {
		uploadDir = "/tmp/aistarlight-uploads"
	}
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		response.InternalError(c, "Failed to prepare upload storage.")
		return
	}

	jsonBytes, err := json.Marshal(req)
	if err != nil {
		response.InternalError(c, "Failed to serialize data.")
		return
	}

	savePath := filepath.Join(uploadDir, fileID+".json")
	if err := os.WriteFile(savePath, jsonBytes, 0o640); err != nil {
		response.InternalError(c, "Failed to save data.")
		return
	}

	// Build response matching the standard upload response format.
	var columns []string
	var sampleRows []map[string]interface{}
	sheetsResp := make(map[string]*service.SheetData)

	for name, sheet := range req.Sheets {
		preview := sheet.Preview
		if len(preview) == 0 && len(sheet.Rows) > 0 {
			limit := 10
			if len(sheet.Rows) < limit {
				limit = len(sheet.Rows)
			}
			preview = sheet.Rows[:limit]
		}
		sheetsResp[name] = &service.SheetData{
			Columns:  sheet.Columns,
			RowCount: sheet.RowCount,
			Preview:  preview,
		}
		if columns == nil {
			columns = sheet.Columns
			sampleRows = preview
		}
	}

	response.OK(c, gin.H{
		"file_id":     fileID,
		"filename":    req.Filename,
		"size":        len(jsonBytes),
		"columns":     columns,
		"sample_rows": sampleRows,
		"sheets":      sheetsResp,
	})
}

// SuggestMapping handles POST /api/v1/data/mapping.
func (h *DataHandler) SuggestMapping(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1*1024*1024) // 1MB cap

	var req suggestMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if len(req.Columns) > 200 {
		response.BadRequest(c, "Too many columns. Maximum 200 allowed.")
		return
	}
	if len(req.SampleRows) > 50 {
		response.BadRequest(c, "Too many sample rows. Maximum 50 allowed.")
		return
	}

	if req.ReportType == "" {
		switch middleware.GetJurisdiction(c) {
		case "SG":
			req.ReportType = "IRAS_GST_F5"
		case "LK":
			req.ReportType = "IRDSL_VAT_RETURN"
		default:
			req.ReportType = "BIR_2550M"
		}
	}

	// Look up saved preferences to seed AI mapping
	var existingMappings map[string]string
	companyID := middleware.GetCompanyID(c)
	if h.memorySvc != nil {
		pref, err := h.memorySvc.GetPreference(c.Request.Context(), companyID, req.ReportType)
		if err == nil && pref != nil && len(pref.ColumnMappings) > 0 {
			existingMappings = make(map[string]string, len(pref.ColumnMappings))
			for k, v := range pref.ColumnMappings {
				if s, ok := v.(string); ok {
					existingMappings[k] = s
				}
			}
		}
	}

	// Phase 4: Fetch past mapping corrections for this company
	var correctionHints []service.MappingCorrection
	if h.q != nil {
		rows, err := h.q.ListMappingCorrections(c.Request.Context(), sqlc.ListMappingCorrectionsParams{
			CompanyID:  companyID,
			EntityType: "column_mapping",
			Limit:      50,
		})
		if err == nil {
			for _, row := range rows {
				correctionHints = append(correctionHints, service.MappingCorrection{
					ColumnName: row.FieldName,
					OldTarget:  derefString(row.OldValue),
					NewTarget:  row.NewValue,
				})
			}
		}
	}

	result, err := h.colMapper.AutoMapColumns(c.Request.Context(), req.Columns, req.SampleRows, req.ReportType, existingMappings, service.AutoMapColumnsOpts{
		DataCategory:    req.DataCategory,
		CorrectionHints: correctionHints,
		Jurisdiction:    middleware.GetJurisdiction(c),
	})
	if err != nil {
		slog.Error("column mapping failed", "error", err)
		response.InternalError(c, "Column mapping failed. Please try again.")
		return
	}

	response.OK(c, result)
}

type mappingCorrectionRequest struct {
	ReportType  string `json:"report_type" binding:"required"`
	Corrections []struct {
		ColumnName   string        `json:"column_name" binding:"required"`
		OldTarget    string        `json:"old_target"`
		NewTarget    string        `json:"new_target" binding:"required"`
		SampleValues []interface{} `json:"sample_values,omitempty"`
	} `json:"corrections" binding:"required"`
}

// RecordMappingCorrections handles POST /api/v1/data/mapping/corrections.
// Records user corrections to AI column mappings for future learning.
func (h *DataHandler) RecordMappingCorrections(c *gin.Context) {
	var req mappingCorrectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	recorded := 0
	for _, corr := range req.Corrections {
		contextData := map[string]interface{}{
			"report_type":   req.ReportType,
			"sample_values": corr.SampleValues,
		}
		contextJSON, _ := json.Marshal(contextData)

		var oldVal *string
		if corr.OldTarget != "" {
			oldVal = &corr.OldTarget
		}

		_, err := h.q.CreateCorrection(c.Request.Context(), sqlc.CreateCorrectionParams{
			ID:          uuid.New(),
			CompanyID:   companyID,
			UserID:      userID,
			EntityType:  "column_mapping",
			EntityID:    uuid.New(), // No specific entity — use random ID
			FieldName:   corr.ColumnName,
			OldValue:    oldVal,
			NewValue:    corr.NewTarget,
			Reason:      nil,
			ContextData: contextJSON,
		})
		if err != nil {
			slog.Warn("failed to record mapping correction", "error", err, "column", corr.ColumnName)
			continue
		}
		recorded++
	}

	response.OK(c, gin.H{"recorded": recorded})
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
