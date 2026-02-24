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
	"github.com/tonypk/aistarlight-go/internal/service"
)

// DataHandler handles file upload and data processing endpoints.
type DataHandler struct {
	colMapper *service.ColumnMapperService
	memorySvc *service.MemoryService
	cfg       *config.Config
}

// NewDataHandler creates a new data handler.
func NewDataHandler(colMapper *service.ColumnMapperService, memorySvc *service.MemoryService, cfg *config.Config) *DataHandler {
	return &DataHandler{colMapper: colMapper, memorySvc: memorySvc, cfg: cfg}
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

	// Parse file.
	parsed, err := service.ParseUploadedFile(content, header.Filename)
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

	// Extract first sheet info for convenience.
	var columns []string
	var sampleRows []map[string]interface{}
	for _, sheet := range parsed.Sheets {
		columns = sheet.Columns
		sampleRows = sheet.Preview
		break
	}

	response.OK(c, gin.H{
		"file_id":     fileID,
		"filename":    header.Filename,
		"size":        len(content),
		"columns":     columns,
		"sample_rows": sampleRows,
		"sheets":      parsed.Sheets,
	})
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

	parsed, err := service.ParseUploadedFile(content, header.Filename)
	if err != nil {
		slog.Error("file preview parse failed", "error", err, "filename", header.Filename)
		response.BadRequest(c, "Could not parse file. Ensure it is a valid, non-corrupted spreadsheet.")
		return
	}

	response.OK(c, parsed)
}

type suggestMappingRequest struct {
	Columns    []string                 `json:"columns" binding:"required"`
	SampleRows []map[string]interface{} `json:"sample_rows" binding:"required"`
	ReportType string                   `json:"report_type"`
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
		req.ReportType = "BIR_2550M"
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

	result, err := h.colMapper.AutoMapColumns(c.Request.Context(), req.Columns, req.SampleRows, req.ReportType, existingMappings)
	if err != nil {
		slog.Error("column mapping failed", "error", err)
		response.InternalError(c, "Column mapping failed. Please try again.")
		return
	}

	response.OK(c, result)
}
