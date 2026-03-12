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
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	tele "gopkg.in/telebot.v3"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
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
	// Collect instruction from photo caption or pending text instruction.
	instruction := strings.TrimSpace(c.Message().Caption)
	if instruction == "" {
		if raw, ok := b.pendingInstructions.LoadAndDelete(c.Sender().ID); ok {
			instruction, _ = raw.(string)
		}
	}
	return b.processReceipt(c, photo.FileID, instruction)
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

	instruction := strings.TrimSpace(c.Message().Caption)
	if instruction == "" {
		if raw, ok := b.pendingInstructions.LoadAndDelete(c.Sender().ID); ok {
			instruction, _ = raw.(string)
		}
	}
	return b.processReceipt(c, doc.FileID, instruction)
}

func (b *Bot) processReceipt(c tele.Context, fileID string, instruction string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		if b.tryAutoLink(c) {
			tgUser, err = b.q.GetTelegramUser(ctx, c.Sender().ID)
		}
		if err != nil {
			return c.Send("Account not linked. Use /link <api_key> first.")
		}
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
	batch, results, err := b.receipt.ProcessBatch(ctx, tgUser.CompanyID, tgUser.UserID, []string{localPath}, period, jCfg.DefaultReport, jurisdictionCode, instruction)
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

	// Multi-trip screenshot (Uber/Grab): skip amount selection, show multi-trip preview.
	if len(results) > 1 {
		resultsJSON, _ := json.Marshal(results)
		_ = b.q.UpdateReceiptBatch(ctx, sqlc.UpdateReceiptBatchParams{
			ID:        batch.ID,
			Status:    "pending_confirmation",
			Results:   resultsJSON,
			ImagePath: ptrStr(imagePath),
		})

		if instruction != "" {
			b.receiptNotes.Store(batch.ID, instruction)
		}

		uploaderName := senderDisplayName(c.Sender())
		preview := formatMultiTripPreview(results, jCfg.CurrencySymbol, uploaderName)

		if len(b.projects) > 0 {
			markup := projectSelectionMarkup(batch.ID, b.projects)
			_, _ = b.B.Edit(processing, preview+"\n\nPlease select a project:", markup)
		} else {
			markup := categorySelectionMarkup(batch.ID, "", receiptCategories)
			_, _ = b.B.Edit(processing, preview+"\n\nSelect a category:", markup)
		}

		go b.receiptTimeout(c.Chat().ID, processing.ID, batch.ID)
		return nil
	}

	// Single receipt flow.
	// Apply user instruction to OCR results (e.g., "use net total", "amount: 1500").
	if instruction != "" && len(results) > 0 {
		applyInstruction(&results[0], instruction)
	}

	// Multi-amount selection logic (Approach C).
	// If multiple amounts detected and no instruction resolved it, ask user to pick.
	detected := results[0].Parsed.DetectedAmounts
	needsSelection := false
	if len(detected) > 0 && instruction != "" {
		// Try to match instruction against detected amounts.
		if matched, ok := service.SelectAmountByInstruction(detected, instruction); ok {
			results[0].Parsed.TotalAmount = service.ParsedField{Value: matched.Amount, Confidence: 1.0}
		}
	}
	if service.NeedsAmountSelection(detected) && results[0].Parsed.TotalAmount.Confidence < 1.0 {
		needsSelection = true
	}

	// Update batch: set status to pending_confirmation and store image_path.
	// Use the actual results (not batch.Results which is the stale initial empty value).
	resultsJSON, _ := json.Marshal(results)
	_ = b.q.UpdateReceiptBatch(ctx, sqlc.UpdateReceiptBatchParams{
		ID:        batch.ID,
		Status:    "pending_confirmation",
		Results:   resultsJSON,
		ImagePath: ptrStr(imagePath),
	})

	// If instruction provided, auto-store as receipt note.
	if instruction != "" {
		b.receiptNotes.Store(batch.ID, instruction)
	}

	// Format preview with uploader info and send with buttons.
	uploaderName := senderDisplayName(c.Sender())

	// If amount selection needed, show amount picker buttons instead of normal flow.
	if needsSelection {
		preview := formatAmountSelectionPreview(results[0], jCfg.CurrencySymbol, uploaderName)
		markup := amountSelectionMarkup(batch.ID, detected, jCfg.CurrencySymbol)
		_, _ = b.B.Edit(processing, preview, markup)
		go b.receiptTimeout(c.Chat().ID, processing.ID, batch.ID)
		return nil
	}

	preview := formatReceiptPreview(results[0], jCfg.CurrencySymbol, uploaderName, "")

	// Show project selection or category selection directly (skip note step).
	if len(b.projects) > 0 {
		markup := projectSelectionMarkup(batch.ID, b.projects)
		_, _ = b.B.Edit(processing, preview+"\n\nPlease select a project:", markup)
	} else {
		markup := categorySelectionMarkup(batch.ID, "", receiptCategories)
		_, _ = b.B.Edit(processing, preview+"\n\nSelect a category:", markup)
	}

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

// applyInstruction parses a user instruction and applies field overrides to the receipt result.
// Supports:
//   - Direct values: "amount: 1500", "vendor: ABC Store"
//   - Field hints: "use net total", "record the vat amount", "总金额是1500"
func applyInstruction(result *service.ReceiptResult, instruction string) {
	lower := strings.ToLower(instruction)

	// Parse key:value pairs (same format as edit).
	for _, line := range strings.Split(instruction, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			parts = strings.SplitN(line, "：", 2) // Chinese colon
		}
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])
		if value == "" {
			continue
		}

		switch key {
		case "amount", "金额", "总额", "total":
			if f, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", ""), 64); err == nil {
				result.Parsed.TotalAmount = service.ParsedField{Value: f, Confidence: 1.0}
			}
		case "vendor", "商家", "店家", "供应商":
			result.Parsed.VendorName = service.ParsedField{Value: value, Confidence: 1.0}
		case "date", "日期":
			result.Parsed.Date = service.ParsedField{Value: value, Confidence: 1.0}
		case "vat", "税额":
			if f, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", ""), 64); err == nil {
				result.Parsed.VATAmount = service.ParsedField{Value: f, Confidence: 1.0}
			}
		case "category", "类别":
			result.Parsed.Category = service.ParsedField{Value: value, Confidence: 1.0}
		}
	}

	// Extract embedded amounts from natural language (e.g., "总金额是1500", "amount 1500").
	if amountRe := extractAmountFromText(lower); amountRe > 0 {
		// Only override if the instruction seems to be about the total amount.
		amountKeywords := []string{"amount", "total", "金额", "总额", "总计", "net total"}
		for _, kw := range amountKeywords {
			if strings.Contains(lower, kw) {
				result.Parsed.TotalAmount = service.ParsedField{Value: amountRe, Confidence: 1.0}
				break
			}
		}
	}

	// Handle "use net total" / "use vat amount" style hints via DetectedAmounts.
	if len(result.Parsed.DetectedAmounts) > 0 {
		if matched, ok := service.SelectAmountByInstruction(result.Parsed.DetectedAmounts, instruction); ok {
			result.Parsed.TotalAmount = service.ParsedField{Value: matched.Amount, Confidence: 1.0}
			return // instruction matched a detected amount, done
		}
	}

	// Legacy hint: boost confidence for net total keywords.
	if strings.Contains(lower, "net total") || strings.Contains(lower, "net amount") {
		if result.Parsed.TotalAmount.Confidence < 1.0 {
			result.Parsed.TotalAmount.Confidence = 0.95
		}
	}
}

// extractAmountFromText finds the first decimal/integer number in text.
func extractAmountFromText(text string) float64 {
	// Match patterns like: 1500, 1,500, 1500.00, 1,500.00
	var numStr strings.Builder
	inNumber := false
	for _, ch := range text {
		if ch >= '0' && ch <= '9' {
			numStr.WriteRune(ch)
			inNumber = true
		} else if inNumber && (ch == '.' || ch == ',') {
			numStr.WriteRune(ch)
		} else if inNumber {
			break
		}
	}
	s := strings.TrimRight(numStr.String(), ",.")
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		return 0
	}
	return f
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
