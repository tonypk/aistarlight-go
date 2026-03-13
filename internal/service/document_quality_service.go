package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	_ "golang.org/x/image/webp"
)

// QualityResult holds the document quality assessment for a receipt image.
type QualityResult struct {
	QualityScore     float64 `json:"quality_score"`      // 0.0-1.0
	IsBlurry         bool    `json:"is_blurry"`          // Laplacian variance below threshold
	IsTooSmall       bool    `json:"is_too_small"`       // Below minimum usable resolution
	IsTooLarge       bool    `json:"is_too_large"`       // Above max file size
	FileHash         string  `json:"file_hash"`          // SHA-256 of file content
	IsDuplicate      bool    `json:"is_duplicate"`       // Matching hash found in DB
	DuplicateBatchID *string `json:"duplicate_batch_id"` // Batch ID of the duplicate
	SuggestedAction  string  `json:"suggested_action"`   // "proceed" | "warn_user" | "request_reupload"
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	FileSizeBytes    int64   `json:"file_size_bytes"`
}

const (
	minImageDimension = 200  // pixels — below this, OCR will likely fail
	maxFileSizeLimit  = 20 * 1024 * 1024 // 20 MB
	blurThreshold     = 100.0 // Laplacian variance threshold
)

// DocumentQualityService assesses image quality before OCR processing.
type DocumentQualityService struct {
	q *sqlc.Queries
}

// NewDocumentQualityService creates a new DocumentQualityService.
func NewDocumentQualityService(q *sqlc.Queries) *DocumentQualityService {
	return &DocumentQualityService{q: q}
}

// AssessFile checks image quality and duplicate status for a local file.
func (s *DocumentQualityService) AssessFile(ctx context.Context, filePath string, companyID uuid.UUID) (*QualityResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	// Compute SHA-256 hash.
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return nil, fmt.Errorf("hash file: %w", err)
	}
	fileHash := hex.EncodeToString(hasher.Sum(nil))

	// Rewind to read image dimensions and assess blur.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek: %w", err)
	}

	result := &QualityResult{
		FileHash:      fileHash,
		FileSizeBytes: stat.Size(),
	}

	// Decode image for dimension and blur checks.
	img, _, decErr := image.Decode(f)
	if decErr != nil {
		// Can't decode image — still return hash result for dedup.
		result.QualityScore = 0.0
		result.SuggestedAction = "request_reupload"
		return result, nil
	}

	bounds := img.Bounds()
	result.Width = bounds.Dx()
	result.Height = bounds.Dy()

	// Dimension checks.
	result.IsTooSmall = result.Width < minImageDimension || result.Height < minImageDimension
	result.IsTooLarge = stat.Size() > maxFileSizeLimit

	// Blur detection via Laplacian variance.
	result.IsBlurry = isBlurry(img)

	// Compute quality score (weighted factors).
	result.QualityScore = computeQualityScore(result)

	// Check for duplicate in DB.
	dup, dupErr := s.q.FindReceiptBatchByImageHash(ctx, sqlc.FindReceiptBatchByImageHashParams{
		CompanyID: companyID,
		ImageHash: &fileHash,
	})
	if dupErr == nil {
		// Found duplicate — only flag if it's recent (within 90 days).
		if time.Since(dup.CreatedAt) < 90*24*time.Hour {
			result.IsDuplicate = true
			dupID := dup.ID.String()
			result.DuplicateBatchID = &dupID
		}
	}

	// Determine suggested action.
	result.SuggestedAction = suggestedAction(result)

	return result, nil
}

// HashFile computes SHA-256 of a file without full quality assessment.
func HashFile(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// isBlurry estimates blur using Laplacian variance on a grayscale version.
// Low variance = blurry image.
func isBlurry(img image.Image) bool {
	variance := laplacianVariance(img)
	return variance < blurThreshold
}

// LaplacianVariance computes the variance of the Laplacian filter response.
// Higher values indicate sharper images.
func laplacianVariance(img image.Image) float64 {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 3 || h < 3 {
		return 0
	}

	// Sample at most 500x500 pixels for performance.
	stepX, stepY := 1, 1
	if w > 500 {
		stepX = w / 500
	}
	if h > 500 {
		stepY = h / 500
	}

	var sum, sumSq float64
	var count int

	for y := bounds.Min.Y + 1; y < bounds.Max.Y-1; y += stepY {
		for x := bounds.Min.X + 1; x < bounds.Max.X-1; x += stepX {
			// Laplacian kernel: [[0,1,0],[1,-4,1],[0,1,0]]
			center := grayValue(img, x, y)
			top := grayValue(img, x, y-1)
			bottom := grayValue(img, x, y+1)
			left := grayValue(img, x-1, y)
			right := grayValue(img, x+1, y)

			lap := float64(top+bottom+left+right) - 4*float64(center)
			sum += lap
			sumSq += lap * lap
			count++
		}
	}

	if count == 0 {
		return 0
	}

	mean := sum / float64(count)
	variance := sumSq/float64(count) - mean*mean
	return variance
}

// grayValue returns the grayscale luminance (0-255) of a pixel.
func grayValue(img image.Image, x, y int) int {
	r, g, b, _ := img.At(x, y).RGBA()
	// Luminosity formula: 0.299R + 0.587G + 0.114B
	lum := (299*r + 587*g + 114*b) / 1000
	return int(lum >> 8)
}

// computeQualityScore returns a 0-1 score from multiple quality factors.
func computeQualityScore(r *QualityResult) float64 {
	score := 1.0

	// Penalty for blur.
	if r.IsBlurry {
		score -= 0.4
	}

	// Penalty for too small.
	if r.IsTooSmall {
		score -= 0.3
	}

	// Penalty for too large (unusual, may indicate wrong file).
	if r.IsTooLarge {
		score -= 0.1
	}

	// Bonus for good resolution: ideal is 800-2000px on long side.
	longSide := math.Max(float64(r.Width), float64(r.Height))
	if longSide >= 800 && longSide <= 2000 {
		// Ideal range, no penalty.
	} else if longSide >= 400 && longSide < 800 {
		score -= 0.1 // Slightly below ideal.
	} else if longSide > 2000 {
		// Large but fine for OCR.
	}

	if score < 0 {
		score = 0
	}
	return score
}

// suggestedAction determines what the bot should do based on quality.
func suggestedAction(r *QualityResult) string {
	if r.IsTooSmall {
		return "request_reupload"
	}
	if r.IsBlurry && r.QualityScore < 0.5 {
		return "warn_user"
	}
	if r.IsDuplicate {
		return "warn_user"
	}
	return "proceed"
}
