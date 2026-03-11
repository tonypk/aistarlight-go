package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	tele "gopkg.in/telebot.v3"

	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/pkg/jurisdiction"
)

// Inline keyboard buttons for receipt confirmation.
var (
	btnConfirm = tele.Btn{Unique: "rcpt_ok", Text: "Confirm"}
	btnEdit    = tele.Btn{Unique: "rcpt_ed", Text: "Edit"}
	btnCancel  = tele.Btn{Unique: "rcpt_no", Text: "Cancel"}
)

const confirmationTimeout = 5 * time.Minute

// confirmationMarkup builds the inline keyboard for receipt confirmation.
func confirmationMarkup(batchID uuid.UUID) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(
			tele.Btn{Unique: btnConfirm.Unique, Text: "Confirm", Data: batchID.String()},
			tele.Btn{Unique: btnEdit.Unique, Text: "Edit", Data: batchID.String()},
			tele.Btn{Unique: btnCancel.Unique, Text: "Cancel", Data: batchID.String()},
		),
	)
	return markup
}

// handleReceiptConfirm handles the Confirm button click.
func (b *Bot) handleReceiptConfirm(c tele.Context) error {
	batchID, err := uuid.Parse(c.Data())
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid batch ID."})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	batch, err := b.q.GetReceiptBatchByID(ctx, batchID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Batch not found."})
	}

	// Verify ownership via telegram user.
	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil || tgUser.CompanyID != batch.CompanyID {
		return c.Respond(&tele.CallbackResponse{Text: "Unauthorized."})
	}

	if batch.Status != "pending_confirmation" {
		return c.Respond(&tele.CallbackResponse{Text: "This receipt has already been processed."})
	}

	_ = c.Respond(&tele.CallbackResponse{})

	// Edit message to show processing state.
	_, _ = b.B.Edit(c.Message(), "Recording transaction...")

	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	reply, err := b.confirmAndProcess(ctx, batch, tgUser, jCfg)
	if err != nil {
		slog.Error("confirm processing failed", "batch_id", batchID, "error", err)
		_, _ = b.B.Edit(c.Message(), "Failed to record transaction.")
		return nil
	}

	_, _ = b.B.Edit(c.Message(), reply)
	return nil
}

// handleReceiptEdit handles the Edit button click.
func (b *Bot) handleReceiptEdit(c tele.Context) error {
	batchID, err := uuid.Parse(c.Data())
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid batch ID."})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batch, err := b.q.GetReceiptBatchByID(ctx, batchID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Batch not found."})
	}

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil || tgUser.CompanyID != batch.CompanyID {
		return c.Respond(&tele.CallbackResponse{Text: "Unauthorized."})
	}

	if batch.Status != "pending_confirmation" {
		return c.Respond(&tele.CallbackResponse{Text: "This receipt has already been processed."})
	}

	_ = c.Respond(&tele.CallbackResponse{})

	// Store pending edit keyed by telegram user ID.
	b.pendingEdits.Store(c.Sender().ID, batchID)

	editInstructions := "Please reply with corrections in key:value format.\n" +
		"Supported fields: amount, vendor, date, vat, category, tin, receipt_no\n\n" +
		"Example:\n" +
		"amount: 1500\n" +
		"vendor: ABC Store\n" +
		"category: services"

	_, _ = b.B.Edit(c.Message(), editInstructions)
	return nil
}

// handleReceiptCancel handles the Cancel button click.
func (b *Bot) handleReceiptCancel(c tele.Context) error {
	batchID, err := uuid.Parse(c.Data())
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid batch ID."})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batch, err := b.q.GetReceiptBatchByID(ctx, batchID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Batch not found."})
	}

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil || tgUser.CompanyID != batch.CompanyID {
		return c.Respond(&tele.CallbackResponse{Text: "Unauthorized."})
	}

	if batch.Status != "pending_confirmation" {
		return c.Respond(&tele.CallbackResponse{Text: "This receipt has already been processed."})
	}

	_ = b.q.UpdateReceiptBatchStatus(ctx, sqlc.UpdateReceiptBatchStatusParams{
		ID:     batchID,
		Status: "cancelled",
	})

	_ = c.Respond(&tele.CallbackResponse{})
	_, _ = b.B.Edit(c.Message(), "Receipt cancelled.")
	return nil
}

// handleReceiptEditReply processes a user's text correction for a pending edit.
func (b *Bot) handleReceiptEditReply(c tele.Context, batchID uuid.UUID, text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	batch, err := b.q.GetReceiptBatchByID(ctx, batchID)
	if err != nil {
		return c.Send("Batch not found. Please send a new receipt.")
	}

	if batch.Status != "pending_confirmation" {
		return c.Send("This receipt has already been processed.")
	}

	// Parse existing results.
	var results []service.ReceiptResult
	if err := json.Unmarshal(batch.Results, &results); err != nil || len(results) == 0 {
		return c.Send("Failed to load receipt data.")
	}

	// Apply corrections.
	parsed := &results[0].Parsed
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch key {
		case "amount":
			if f, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", ""), 64); err == nil {
				parsed.TotalAmount = service.ParsedField{Value: f, Confidence: 1.0}
			}
		case "vendor":
			parsed.VendorName = service.ParsedField{Value: value, Confidence: 1.0}
		case "date":
			parsed.Date = service.ParsedField{Value: value, Confidence: 1.0}
		case "vat":
			if f, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", ""), 64); err == nil {
				parsed.VATAmount = service.ParsedField{Value: f, Confidence: 1.0}
			}
		case "category":
			parsed.Category = service.ParsedField{Value: value, Confidence: 1.0}
		case "tin":
			parsed.TIN = service.ParsedField{Value: value, Confidence: 1.0}
		case "receipt_no":
			parsed.ReceiptNumber = service.ParsedField{Value: value, Confidence: 1.0}
		}
	}

	results[0].OverallConfidence = service.AverageConfidence(results[0].Parsed)

	// Save updated results to DB.
	resultsJSON, _ := json.Marshal(results)
	_ = b.q.UpdateReceiptBatch(ctx, sqlc.UpdateReceiptBatchParams{
		ID:      batch.ID,
		Status:  "pending_confirmation",
		Results: resultsJSON,
	})

	// Get jurisdiction for currency symbol.
	tgUser, _ := b.q.GetTelegramUser(ctx, c.Sender().ID)
	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	uploaderName := senderDisplayName(c.Sender())
	preview := formatReceiptPreview(results[0], jCfg.CurrencySymbol, uploaderName)
	markup := confirmationMarkup(batch.ID)

	return c.Send(preview, markup)
}

// confirmAndProcess executes Phase 2: create transactions, classify, generate journal.
func (b *Bot) confirmAndProcess(ctx context.Context, batch sqlc.ReceiptBatch, tgUser sqlc.TelegramUser, jCfg jurisdiction.Config) (string, error) {
	jurisdictionCode := jCfg.Code
	if jurisdictionCode == "" {
		jurisdictionCode = "PH"
	}

	// Mark batch as completed so ConvertReceiptToTransactions can proceed.
	_ = b.q.UpdateReceiptBatchStatus(ctx, sqlc.UpdateReceiptBatchStatusParams{
		ID:     batch.ID,
		Status: "completed",
	})

	period := time.Now().UTC().Format("2006-01")
	sessionID, err := b.getOrCreateSession(ctx, tgUser.CompanyID, tgUser.UserID, period)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	txns, err := b.bridge.ConvertReceiptToTransactions(ctx, tgUser.CompanyID, batch.ID, sessionID)
	if err != nil {
		return "", fmt.Errorf("convert receipt: %w", err)
	}

	// Classify transactions.
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
		classResults, err = b.classifier.ClassifyTransactions(ctx, classInput, tgUser.CompanyID, jurisdictionCode, "")
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

	// Parse results for formatting.
	var results []service.ReceiptResult
	_ = json.Unmarshal(batch.Results, &results)
	if len(results) == 0 {
		return "Transaction recorded.", nil
	}

	reply := formatReceiptReply(results[0], len(txns), classResults, journalEntries, jCfg.CurrencySymbol)
	return reply, nil
}

// receiptTimeout cancels a pending receipt after the timeout duration.
func (b *Bot) receiptTimeout(chatID int64, msgID int, batchID uuid.UUID) {
	time.Sleep(confirmationTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batch, err := b.q.GetReceiptBatchByID(ctx, batchID)
	if err != nil {
		return
	}

	if batch.Status != "pending_confirmation" {
		return // already confirmed or cancelled
	}

	_ = b.q.UpdateReceiptBatchStatus(ctx, sqlc.UpdateReceiptBatchStatusParams{
		ID:     batchID,
		Status: "cancelled",
	})

	msg := &tele.Message{ID: msgID, Chat: &tele.Chat{ID: chatID}}
	_, _ = b.B.Edit(msg, "Receipt expired (5 min timeout). Please send the photo again.")
}

// senderDisplayName returns a display name for the telegram sender.
func senderDisplayName(u *tele.User) string {
	if u == nil {
		return "Unknown"
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	name := u.FirstName
	if u.LastName != "" {
		name += " " + u.LastName
	}
	if name == "" {
		return fmt.Sprintf("User %d", u.ID)
	}
	return name
}
