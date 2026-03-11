package handler

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/config"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/handler/response"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

const (
	maxReceiptUploadSize = 20 << 20 // 20MB per file (browser pre-compressed ~2MB)
	maxReceiptBatchSize  = 50
)

var allowedImageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".bmp": true, ".tiff": true, ".tif": true, ".webp": true,
}

// ReceiptHandler handles receipt processing endpoints.
type ReceiptHandler struct {
	svc *service.ReceiptService
	cfg *config.Config
}

// NewReceiptHandler creates a receipt handler.
func NewReceiptHandler(svc *service.ReceiptService, cfg *config.Config) *ReceiptHandler {
	return &ReceiptHandler{svc: svc, cfg: cfg}
}

// Upload handles POST /api/v1/receipts/upload.
// Accepts multipart/form-data with files, period, and report_type.
//
// Flow: save uploaded image → OCR on full-quality image → compress for storage.
// This ensures OCR accuracy is not affected by compression.
func (h *ReceiptHandler) Upload(c *gin.Context) {
	period := c.PostForm("period")
	if period == "" {
		response.BadRequest(c, "period is required")
		return
	}
	reportType := c.PostForm("report_type")

	form, err := c.MultipartForm()
	if err != nil {
		response.BadRequest(c, "invalid multipart form")
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		response.BadRequest(c, "no files uploaded")
		return
	}
	if len(files) > maxReceiptBatchSize {
		response.BadRequest(c, fmt.Sprintf("maximum %d images per batch", maxReceiptBatchSize))
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	// Ensure upload directories exist
	receiptDir := filepath.Join(h.cfg.UploadDir, "receipts", companyID.String())
	if err := os.MkdirAll(receiptDir, 0o755); err != nil {
		slog.Error("create receipt dir", "error", err)
		response.InternalError(c, "failed to prepare storage")
		return
	}

	// --- Phase 1: Save uploaded files (full quality for OCR) ---
	var ocrPaths []string
	type fileInfo struct {
		filename     string
		ocrPath      string
		originalSize int64
	}
	var fileInfos []fileInfo

	for _, fh := range files {
		if fh.Size > maxReceiptUploadSize {
			response.BadRequest(c, fmt.Sprintf("file %s exceeds %dMB limit", fh.Filename, maxReceiptUploadSize>>20))
			return
		}

		ext := strings.ToLower(filepath.Ext(fh.Filename))
		if !allowedImageExts[ext] {
			response.BadRequest(c, fmt.Sprintf("unsupported format: %s (accepted: jpg, png, bmp, tiff, webp)", ext))
			return
		}

		// Save original for OCR (full quality)
		saveName := fmt.Sprintf("%s_ocr%s", uuid.New().String(), ext)
		ocrPath := filepath.Join(receiptDir, saveName)
		if err := c.SaveUploadedFile(fh, ocrPath); err != nil {
			slog.Error("save uploaded file", "file", fh.Filename, "error", err)
			response.InternalError(c, "failed to save uploaded file")
			return
		}

		ocrPaths = append(ocrPaths, ocrPath)
		fileInfos = append(fileInfos, fileInfo{
			filename:     fh.Filename,
			ocrPath:      ocrPath,
			originalSize: fh.Size,
		})
	}

	// --- Phase 2: Run OCR on full-quality images ---
	batch, results, err := h.svc.ProcessBatch(
		c.Request.Context(),
		companyID, userID,
		ocrPaths,
		period,
		reportType,
		"", // jurisdictionCode: uses company default (PH)
	)
	if err != nil {
		slog.Error("process receipt batch", "error", err)
		response.InternalError(c, "receipt processing failed")
		return
	}

	// --- Phase 3: Compress images for long-term storage (after OCR) ---
	var compressionInfo []gin.H
	for _, fi := range fileInfos {
		data, err := os.ReadFile(fi.ocrPath)
		if err != nil {
			slog.Warn("read OCR file for compression", "error", err)
			compressionInfo = append(compressionInfo, gin.H{
				"filename":   fi.filename,
				"compressed": false,
			})
			continue
		}

		compressed, err := service.CompressReceiptImage(bytes.NewReader(data), len(data))
		if err != nil {
			slog.Warn("post-OCR compression failed", "file", fi.filename, "error", err)
			compressionInfo = append(compressionInfo, gin.H{
				"filename":      fi.filename,
				"original_size": fi.originalSize,
				"compressed":    false,
			})
			continue
		}

		// Save compressed version
		archiveName := fmt.Sprintf("%s.jpg", uuid.New().String())
		archivePath := filepath.Join(receiptDir, archiveName)
		if err := os.WriteFile(archivePath, compressed.Data, 0o644); err != nil {
			slog.Warn("save compressed archive", "error", err)
			continue
		}

		// Remove the full-quality OCR file (no longer needed)
		_ = os.Remove(fi.ocrPath)

		compressionInfo = append(compressionInfo, gin.H{
			"filename":        fi.filename,
			"original_size":   fi.originalSize,
			"compressed_size": compressed.CompressedSize,
			"dimensions":      fmt.Sprintf("%dx%d", compressed.Width, compressed.Height),
			"compressed":      true,
			"ratio":           fmt.Sprintf("%.0f%%", (1-float64(compressed.CompressedSize)/float64(fi.originalSize))*100),
		})

		slog.Info("receipt archived (post-OCR compression)",
			"file", fi.filename,
			"original", fi.originalSize,
			"archived", compressed.CompressedSize,
		)
	}

	response.Created(c, gin.H{
		"batch":       batch,
		"results":     results,
		"compression": compressionInfo,
	})
}

// UploadJSON handles POST /api/v1/receipts/upload-json (legacy: pre-saved paths).
func (h *ReceiptHandler) UploadJSON(c *gin.Context) {
	var req struct {
		Period     string   `json:"period" binding:"required"`
		ReportType string   `json:"report_type"`
		ImagePaths []string `json:"image_paths" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if len(req.ImagePaths) > maxReceiptBatchSize {
		response.BadRequest(c, fmt.Sprintf("maximum %d images per batch", maxReceiptBatchSize))
		return
	}

	companyID := middleware.GetCompanyID(c)
	userID := middleware.GetUserID(c)

	// OCR first on original files
	batch, results, err := h.svc.ProcessBatch(
		c.Request.Context(),
		companyID, userID,
		req.ImagePaths,
		req.Period,
		req.ReportType,
		"", // jurisdictionCode: uses company default (PH)
	)
	if err != nil {
		slog.Error("process receipt batch", "error", err)
		response.InternalError(c, "receipt processing failed")
		return
	}

	// Compress each image after OCR for storage
	for _, imgPath := range req.ImagePaths {
		data, err := os.ReadFile(imgPath)
		if err != nil {
			continue
		}

		compressed, err := service.CompressReceiptImage(bytes.NewReader(data), len(data))
		if err != nil {
			slog.Warn("post-OCR compress failed", "path", imgPath, "error", err)
			continue
		}

		// Save compressed version alongside original
		archivePath := strings.TrimSuffix(imgPath, filepath.Ext(imgPath)) + "_archived.jpg"
		if err := os.WriteFile(archivePath, compressed.Data, 0o644); err != nil {
			continue
		}

		// Remove original
		_ = os.Remove(imgPath)

		slog.Info("receipt archived",
			"original", len(data), "compressed", compressed.CompressedSize,
		)
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
