package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	tele "gopkg.in/telebot.v3"

	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/birforms"
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
	defer os.Remove(localPath)

	period := time.Now().UTC().Format("2006-01")
	sessionID, err := b.getOrCreateSession(ctx, tgUser.CompanyID, tgUser.UserID, period)
	if err != nil {
		slog.Error("failed to get/create session", "error", err)
		return editError("Failed to create session.")
	}

	batch, results, err := b.receipt.ProcessBatch(ctx, tgUser.CompanyID, tgUser.UserID, []string{localPath}, period, birforms.FormBIR2550M)
	if err != nil {
		slog.Error("receipt processing failed", "error", err)
		return editError("Failed to process receipt.")
	}

	if len(results) == 0 || results[0].Error != "" {
		errMsg := "unknown error"
		if len(results) > 0 {
			errMsg = results[0].Error
		}
		slog.Warn("receipt OCR failed", "error", errMsg)
		return editError("Could not read receipt. Please try a clearer photo.")
	}

	txns, err := b.bridge.ConvertReceiptToTransactions(ctx, tgUser.CompanyID, batch.ID, sessionID)
	if err != nil {
		slog.Error("receipt bridge failed", "error", err)
		return editError("Failed to record transaction.")
	}

	_, _ = b.B.Edit(processing, "Classifying transactions...")

	// Classify transactions via AI.
	var classResults []service.ClassificationResult
	if len(txns) > 0 {
		classInput := make([]map[string]interface{}, len(txns))
		for i, txn := range txns {
			classInput[i] = map[string]interface{}{
				"id":          txn.ID.String(),
				"description": derefStr(txn.Description),
				"amount":      txn.Amount.String(),
				"source_type": txn.SourceType,
			}
		}
		classResults, err = b.classifier.ClassifyTransactions(ctx, classInput, tgUser.CompanyID, "")
		if err != nil {
			slog.Warn("classification failed, continuing without", "error", err)
		} else {
			for i, cr := range classResults {
				if i >= len(txns) {
					break
				}
				var conf pgtype.Numeric
				_ = conf.Scan(strconv.FormatFloat(cr.Confidence, 'f', -1, 64))
				_ = b.q.BulkUpdateTransactionClassification(ctx, sqlc.BulkUpdateTransactionClassificationParams{
					ID:                   txns[i].ID,
					VatType:              cr.VATType,
					Category:             cr.Category,
					Confidence:           conf,
					ClassificationSource: cr.ClassificationSource,
				})
			}
		}
	}

	// Generate journal entries.
	var journalEntries []*domain.JournalEntry
	for _, txn := range txns {
		je, jeErr := b.journalGen.GenerateFromTransaction(ctx, tgUser.CompanyID, txn.ID, tgUser.UserID)
		if jeErr != nil {
			slog.Warn("journal generation failed for txn", "txn_id", txn.ID, "error", jeErr)
			continue
		}
		journalEntries = append(journalEntries, je)
	}

	reply := formatReceiptReply(results[0], len(txns), classResults, journalEntries)
	_, _ = b.B.Edit(processing, reply)
	return nil
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
	defer rc.Close()

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
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		os.Remove(localPath)
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
