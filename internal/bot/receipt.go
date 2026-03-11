package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	tele "gopkg.in/telebot.v3"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/jurisdiction"
)

const (
	maxImageLongSide = 1920 // max pixels on longest side
	jpegQuality      = 80   // JPEG output quality (0-100)
)

const maxFileSizeBytes = 10 * 1024 * 1024 // 10 MB

var allowedImageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".heic": true,
}

func (b *Bot) handlePhoto(c tele.Context) error {
	photo := c.Message().Photo
	if photo == nil {
		return nil
	}
	return b.processReceipt(c, photo.FileID)
}

func (b *Bot) handleDocument(c tele.Context) error {
	doc := c.Message().Document
	if doc == nil {
		return nil
	}

	if !strings.HasPrefix(doc.MIME, "image/") {
		return c.Send("Please send an image file (JPEG, PNG).")
	}

	if doc.FileSize > maxFileSizeBytes {
		return c.Send("Image is too large. Please send a file under 10 MB.")
	}

	return b.processReceipt(c, doc.FileID)
}

func (b *Bot) processReceipt(c tele.Context, fileID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Account not linked. Use /link <api_key> first.")
	}

	// Look up company to get jurisdiction
	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	processing, err := b.B.Send(c.Chat(), "Processing receipt...")
	if err != nil {
		return err
	}

	// editError is a helper that edits the processing message and returns nil
	// (returning nil is intentional — telebot's error handler would send a duplicate message)
	editError := func(userMsg string) error {
		_, _ = b.B.Edit(processing, userMsg)
		return nil
	}

	localPath, err := b.downloadFile(fileID)
	if err != nil {
		slog.Error("failed to download file", "error", err)
		return editError("Failed to download image.")
	}

	period := time.Now().UTC().Format("2006-01")

	// Phase 1: OCR only
	batch, results, err := b.receipt.ProcessBatch(ctx, tgUser.CompanyID, tgUser.UserID, []string{localPath}, period, jCfg.DefaultReport, jurisdictionCode)
	if err != nil {
		_ = os.Remove(localPath)
		slog.Error("receipt processing failed", "error", err)
		return editError("Failed to process receipt.")
	}

	if len(results) == 0 || results[0].Error != "" {
		_ = os.Remove(localPath)
		errMsg := "unknown error"
		if len(results) > 0 {
			errMsg = results[0].Error
		}
		slog.Warn("receipt OCR failed", "error", errMsg)
		return editError("Could not read receipt. Please try a clearer photo.")
	}

	// Save image persistently.
	imagePath, saveErr := b.saveReceiptImage(localPath, tgUser.CompanyID, batch.ID)
	if saveErr != nil {
		slog.Warn("failed to save receipt image persistently", "error", saveErr)
		// Continue without persistent image — not fatal.
	} else {
		// Clean up temp file since we saved a persistent copy.
		_ = os.Remove(localPath)
	}

	// Update batch: set status to pending_confirmation and store image_path.
	_ = b.q.UpdateReceiptBatch(ctx, sqlc.UpdateReceiptBatchParams{
		ID:        batch.ID,
		Status:    "pending_confirmation",
		Results:   batch.Results,
		ImagePath: ptrStr(imagePath),
	})

	// Format preview with uploader info and send with buttons.
	uploaderName := senderDisplayName(c.Sender())
	preview := formatReceiptPreview(results[0], jCfg.CurrencySymbol, uploaderName, "")

	// If projects are configured, show project selection first; otherwise skip to confirm.
	var markup *tele.ReplyMarkup
	if len(b.projects) > 0 {
		markup = projectSelectionMarkup(batch.ID, b.projects)
	} else {
		markup = confirmationMarkup(batch.ID, "")
	}

	_, _ = b.B.Edit(processing, preview, markup)

	// Start timeout goroutine.
	go b.receiptTimeout(c.Chat().ID, processing.ID, batch.ID)

	return nil
}

// saveReceiptImage decodes, resizes (max 1920px long side), and saves as compressed JPEG.
func (b *Bot) saveReceiptImage(localPath string, companyID, batchID uuid.UUID) (string, error) {
	companyDir := filepath.Join(b.uploadDir, companyID.String())
	if err := os.MkdirAll(companyDir, 0o755); err != nil {
		return "", fmt.Errorf("create company dir: %w", err)
	}

	// Always output as .jpg regardless of input format.
	destPath := filepath.Join(companyDir, batchID.String()+".jpg")

	src, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = src.Close() }()

	img, _, err := image.Decode(src)
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}

	// Resize if either dimension exceeds maxImageLongSide.
	img = resizeIfNeeded(img, maxImageLongSide)

	dst, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create dest: %w", err)
	}
	defer func() { _ = dst.Close() }()

	if err := jpeg.Encode(dst, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		_ = os.Remove(destPath)
		return "", fmt.Errorf("encode jpeg: %w", err)
	}

	return destPath, nil
}

// resizeIfNeeded scales the image so the longest side is at most maxPx.
func resizeIfNeeded(img image.Image, maxPx int) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	longest := w
	if h > longest {
		longest = h
	}
	if longest <= maxPx {
		return img
	}

	ratio := float64(maxPx) / float64(longest)
	newW := int(float64(w) * ratio)
	newH := int(float64(h) * ratio)

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
	return dst
}

func ptrStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (b *Bot) downloadFile(fileID string) (string, error) {
	f, err := b.B.FileByID(fileID)
	if err != nil {
		return "", fmt.Errorf("get file: %w", err)
	}

	rc, err := b.B.File(&f)
	if err != nil {
		return "", fmt.Errorf("download file: %w", err)
	}
	defer func() { _ = rc.Close() }()

	if err := os.MkdirAll(b.uploadDir, 0o755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}

	// Whitelist extensions to prevent unexpected file types
	ext := strings.ToLower(filepath.Ext(f.FilePath))
	if !allowedImageExts[ext] {
		ext = ".jpg"
	}
	localPath := filepath.Join(b.uploadDir, fmt.Sprintf("tg_%s%s", uuid.New().String(), ext))

	out, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, rc); err != nil {
		_ = os.Remove(localPath)
		return "", fmt.Errorf("write file: %w", err)
	}

	return localPath, nil
}

func (b *Bot) getOrCreateSession(ctx context.Context, companyID, userID uuid.UUID, period string) (uuid.UUID, error) {
	session, err := b.q.GetActiveSessionByCompanyAndPeriod(ctx, sqlc.GetActiveSessionByCompanyAndPeriodParams{
		CompanyID: companyID,
		Period:    period,
	})
	if err == nil {
		return session.ID, nil
	}

	sourceFiles, err := json.Marshal([]map[string]string{{"source": "telegram_bot"}})
	if err != nil {
		return uuid.Nil, fmt.Errorf("marshal source files: %w", err)
	}

	newSession, err := b.q.CreateReconciliationSession(ctx, sqlc.CreateReconciliationSessionParams{
		ID:          uuid.New(),
		CompanyID:   companyID,
		CreatedBy:   userID,
		Period:      period,
		Status:      "active",
		SourceFiles: sourceFiles,
	})
	if err != nil {
		// TOCTOU: another goroutine may have created the session concurrently.
		// Retry the lookup once before giving up.
		session, retryErr := b.q.GetActiveSessionByCompanyAndPeriod(ctx, sqlc.GetActiveSessionByCompanyAndPeriodParams{
			CompanyID: companyID,
			Period:    period,
		})
		if retryErr == nil {
			return session.ID, nil
		}
		return uuid.Nil, fmt.Errorf("create session: %w", err)
	}
	return newSession.ID, nil
}

// isNotFound checks if the error is a pgx "no rows" error.
func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
