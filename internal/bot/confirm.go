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
	btnConfirm      = tele.Btn{Unique: "rcpt_ok", Text: "Confirm"}
	btnEdit         = tele.Btn{Unique: "rcpt_ed", Text: "Edit"}
	btnCancel       = tele.Btn{Unique: "rcpt_no", Text: "Cancel"}
	btnProject      = tele.Btn{Unique: "rcpt_pj", Text: "Project"}
	btnAmountSelect       = tele.Btn{Unique: "rcpt_am", Text: "Amount"}
	btnAmountCustom       = tele.Btn{Unique: "rcpt_ac", Text: "Other amount"}
	btnCategory           = tele.Btn{Unique: "rcpt_ct", Text: "Category"}
)

// Default expense categories for receipt classification.
var receiptCategories = []string{"goods", "services", "capital", "imports", "other"}

const confirmationTimeout = 5 * time.Minute

// CustomCategoryPending tracks the state when waiting for a user to type a custom category.
type CustomCategoryPending struct {
	BatchID    uuid.UUID
	ProjectTag string
}

// CustomAmountPending tracks the state when waiting for a user to type a custom amount.
type CustomAmountPending struct {
	BatchID uuid.UUID
}

// callbackData encodes batchID + optional projectTag into callback data.
// Format: "batchID" or "batchID|projectTag"
func encodeCallbackData(batchID uuid.UUID, projectTag string) string {
	if projectTag == "" {
		return batchID.String()
	}
	return batchID.String() + "|" + projectTag
}

// parseCallbackData extracts batchID and optional projectTag from callback data.
func parseCallbackData(data string) (uuid.UUID, string, error) {
	parts := strings.SplitN(data, "|", 3)
	batchID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, "", err
	}
	projectTag := ""
	if len(parts) >= 2 {
		projectTag = parts[1]
	}
	return batchID, projectTag, nil
}

// parseCategoryCallbackData extracts batchID, projectTag, and category from callback data.
// Format: "batchID|projectTag|category"
func parseCategoryCallbackData(data string) (uuid.UUID, string, string, error) {
	parts := strings.SplitN(data, "|", 3)
	batchID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, "", "", err
	}
	projectTag := ""
	if len(parts) >= 2 {
		projectTag = parts[1]
	}
	category := ""
	if len(parts) >= 3 {
		category = parts[2]
	}
	return batchID, projectTag, category, nil
}

// storeReplyMapping stores the mapping from a bot message to transaction data
// so reply-to corrections can identify the transactions.
func (b *Bot) storeReplyMapping(chatID int64, msg *tele.Message, txnIDs []uuid.UUID, refNumbers []int32) {
	if msg == nil || len(txnIDs) == 0 {
		return
	}
	key := fmt.Sprintf("%d:%d", chatID, msg.ID)
	b.replyTxnMap.Store(key, &ReplyTxnData{
		TxnIDs:     txnIDs,
		RefNumbers: refNumbers,
	})
}

// projectSelectionMarkup builds inline keyboard with project buttons for selection.
func projectSelectionMarkup(batchID uuid.UUID, projects []string) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	var btns []tele.Btn
	for _, p := range projects {
		btns = append(btns, tele.Btn{
			Unique: btnProject.Unique,
			Text:   p,
			Data:   encodeCallbackData(batchID, p),
		})
	}
	rows := []tele.Row{markup.Row(btns...)}
	// Add Edit and Cancel buttons.
	rows = append(rows, markup.Row(
		tele.Btn{Unique: btnEdit.Unique, Text: "Edit", Data: batchID.String()},
		tele.Btn{Unique: btnCancel.Unique, Text: "Cancel", Data: batchID.String()},
	))
	markup.Inline(rows...)
	return markup
}

// confirmationMarkup builds the inline keyboard with selected project + confirm/edit/cancel.
func confirmationMarkup(batchID uuid.UUID, projectTag string) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	data := encodeCallbackData(batchID, projectTag)
	markup.Inline(
		markup.Row(
			tele.Btn{Unique: btnConfirm.Unique, Text: "Confirm", Data: data},
			tele.Btn{Unique: btnEdit.Unique, Text: "Edit", Data: batchID.String()},
			tele.Btn{Unique: btnCancel.Unique, Text: "Cancel", Data: batchID.String()},
		),
	)
	return markup
}

// categorySelectionMarkup builds inline keyboard with category buttons for receipt classification.
// Clicking a category confirms the receipt with that category.
// If aiCategory is non-empty, a Confirm button is shown first using the AI-detected category.
func categorySelectionMarkup(batchID uuid.UUID, projectTag string, categories []string, aiCategory ...string) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	var rows []tele.Row

	// If AI already detected a category, show a Confirm button on the first row.
	detectedCat := ""
	if len(aiCategory) > 0 {
		detectedCat = aiCategory[0]
	}
	if detectedCat != "" {
		confirmData := encodeCallbackData(batchID, projectTag)
		confirmLabel := "Confirm (" + strings.ToUpper(detectedCat[:1]) + detectedCat[1:] + ")"
		rows = append(rows, markup.Row(
			tele.Btn{Unique: btnConfirm.Unique, Text: confirmLabel, Data: confirmData},
		))
	}

	// Category buttons for manual override.
	var catBtns []tele.Btn
	for _, cat := range categories {
		if cat == detectedCat {
			continue // skip the AI-detected one, already shown as Confirm
		}
		data := batchID.String() + "|" + projectTag + "|" + cat
		label := strings.ToUpper(cat[:1]) + cat[1:]
		catBtns = append(catBtns, tele.Btn{
			Unique: btnCategory.Unique,
			Text:   label,
			Data:   data,
		})
	}
	if len(catBtns) > 0 {
		rows = append(rows, markup.Row(catBtns...))
	}

	rows = append(rows, markup.Row(
		tele.Btn{Unique: btnEdit.Unique, Text: "Edit", Data: batchID.String()},
		tele.Btn{Unique: btnCancel.Unique, Text: "Cancel", Data: batchID.String()},
	))
	markup.Inline(rows...)
	return markup
}

// handleProjectSelect handles the Project button click — user selected a project.
// Goes directly to category selection (skipping note step).
func (b *Bot) handleProjectSelect(c tele.Context) error {
	batchID, projectTag, err := parseCallbackData(c.Data())
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid data."})
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

	_ = c.Respond(&tele.CallbackResponse{Text: "Project: " + projectTag})

	// Show receipt preview with category selection.
	var results []service.ReceiptResult
	if err := json.Unmarshal(batch.Results, &results); err != nil || len(results) == 0 {
		_, _ = b.B.Edit(c.Message(), "Failed to load receipt data.")
		return nil
	}

	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	uploaderName := senderDisplayName(c.Sender())
	aiCat := extractAICategory(results)
	var preview string
	if len(results) > 1 {
		preview = formatMultiTripPreview(results, jCfg.CurrencySymbol, uploaderName)
		preview += fmt.Sprintf("\nProject: %s", projectTag)
	} else {
		preview = formatReceiptPreview(results[0], jCfg.CurrencySymbol, uploaderName, projectTag)
	}
	preview += "\n\nConfirm or select a category:"

	markup := categorySelectionMarkup(batchID, projectTag, receiptCategories, aiCat)
	_, _ = b.B.Edit(c.Message(), preview, markup)
	return nil
}

// handleReceiptConfirm handles the Confirm button click.
func (b *Bot) handleReceiptConfirm(c tele.Context) error {
	batchID, projectTag, err := parseCallbackData(c.Data())
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid batch ID."})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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

	// Check if approval is required before processing.
	vendorName, amount := extractReceiptInfo(batch)
	if b.checkAndRequestApproval(c, ctx, batchID, tgUser.CompanyID, tgUser.UserID, amount, vendorName, false) {
		_, _ = b.B.Edit(c.Message(), "Pending approval — an approver has been notified.")
		return nil
	}

	_, _ = b.B.Edit(c.Message(), "Recording transaction...")

	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	var projPtr *string
	if projectTag != "" {
		projPtr = &projectTag
	}

	// Read user note if present.
	var note string
	if rawNote, ok := b.receiptNotes.LoadAndDelete(batchID); ok {
		note, _ = rawNote.(string)
	}

	reply, txnIDs, refNumbers, err := b.confirmAndProcess(ctx, batch, tgUser, jCfg, projPtr, note, "")
	if err != nil {
		slog.Error("confirm processing failed", "batch_id", batchID, "error", err)
		_, _ = b.B.Edit(c.Message(), "Failed to record transaction.")
		return nil
	}

	msg, editErr := b.B.Edit(c.Message(), reply)
	if editErr != nil {
		slog.Warn("failed to edit confirmation message", "error", editErr)
	}
	b.storeReplyMapping(c.Chat().ID, msg, txnIDs, refNumbers)

	// Proactively suggest vendor rules after successful confirmation.
	go b.checkAndSendRuleSuggestions(c.Chat().ID, tgUser.CompanyID)

	return nil
}

// handleCategorySelect handles a category button click — confirms with the selected category.
func (b *Bot) handleCategorySelect(c tele.Context) error {
	batchID, projectTag, category, err := parseCategoryCallbackData(c.Data())
	if err != nil || category == "" {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid data."})
	}

	// "Other" category: prompt user to type a custom category name.
	if category == "other" {
		_ = c.Respond(&tele.CallbackResponse{Text: "Type your category"})
		b.pendingCustomCategory.Store(c.Sender().ID, &CustomCategoryPending{
			BatchID:    batchID,
			ProjectTag: projectTag,
		})
		_, _ = b.B.Edit(c.Message(), "Please type your custom category name:")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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

	_ = c.Respond(&tele.CallbackResponse{Text: "Category: " + category})

	// Check if approval is required before processing.
	vendorName, amount := extractReceiptInfo(batch)
	if b.checkAndRequestApproval(c, ctx, batchID, tgUser.CompanyID, tgUser.UserID, amount, vendorName, false) {
		_, _ = b.B.Edit(c.Message(), "Pending approval — an approver has been notified.")
		return nil
	}

	_, _ = b.B.Edit(c.Message(), "Recording transaction...")

	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	var projPtr *string
	if projectTag != "" {
		projPtr = &projectTag
	}

	var note string
	if rawNote, ok := b.receiptNotes.LoadAndDelete(batchID); ok {
		note, _ = rawNote.(string)
	}

	reply, txnIDs, refNumbers, err := b.confirmAndProcess(ctx, batch, tgUser, jCfg, projPtr, note, category)
	if err != nil {
		slog.Error("confirm processing failed", "batch_id", batchID, "error", err)
		_, _ = b.B.Edit(c.Message(), "Failed to record transaction.")
		return nil
	}

	msg, editErr := b.B.Edit(c.Message(), reply)
	if editErr != nil {
		slog.Warn("failed to edit confirmation message", "error", editErr)
	}
	b.storeReplyMapping(c.Chat().ID, msg, txnIDs, refNumbers)

	// Proactively suggest vendor rules after successful confirmation.
	go b.checkAndSendRuleSuggestions(c.Chat().ID, tgUser.CompanyID)

	return nil
}

// handleReceiptEdit handles the Edit button click.
func (b *Bot) handleReceiptEdit(c tele.Context) error {
	batchID, _, err := parseCallbackData(c.Data())
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

	b.receiptNotes.Delete(batchID)
	b.pendingEdits.Store(c.Sender().ID, batchID)

	editInstructions := "Reply with corrections (colon or space separated).\n" +
		"Supported fields: amount, vendor, date, vat, category, tin, receipt_no\n\n" +
		"Example:\n" +
		"amount 1500\n" +
		"vendor ABC Store\n" +
		"category services"

	_, _ = b.B.Edit(c.Message(), editInstructions)
	return nil
}

// handleReceiptCancel handles the Cancel button click.
func (b *Bot) handleReceiptCancel(c tele.Context) error {
	batchID, _, err := parseCallbackData(c.Data())
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

	var results []service.ReceiptResult
	if err := json.Unmarshal(batch.Results, &results); err != nil || len(results) == 0 {
		return c.Send("Failed to load receipt data.")
	}

	// Store original OCR results before user edits (for auto-learning).
	b.originalResults.Store(batchID, results[0])

	parsed := &results[0].Parsed
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Support both "key: value" and "key value" formats.
		var key, value string
		if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
			key = strings.TrimSpace(strings.ToLower(parts[0]))
			value = strings.TrimSpace(parts[1])
		} else if parts := strings.SplitN(line, " ", 2); len(parts) == 2 {
			key = strings.TrimSpace(strings.ToLower(parts[0]))
			value = strings.TrimSpace(parts[1])
		} else {
			continue
		}
		if value == "" {
			continue
		}

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

	resultsJSON, _ := json.Marshal(results)
	_ = b.q.UpdateReceiptBatch(ctx, sqlc.UpdateReceiptBatchParams{
		ID:      batch.ID,
		Status:  "pending_confirmation",
		Results: resultsJSON,
	})

	tgUser, _ := b.q.GetTelegramUser(ctx, c.Sender().ID)
	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	uploaderName := senderDisplayName(c.Sender())
	preview := formatReceiptPreview(results[0], jCfg.CurrencySymbol, uploaderName, "")

	// After edit, show project selection or category selection.
	if len(b.projects) > 0 {
		markup := projectSelectionMarkup(batch.ID, b.projects)
		return c.Send(preview+"\n\nPlease select a project:", markup)
	}

	// No projects — show category selection directly.
	aiCat := extractAICategory(results)
	markup := categorySelectionMarkup(batch.ID, "", receiptCategories, aiCat)
	return c.Send(preview+"\n\nConfirm or select a category:", markup)
}

// extractReceiptInfo extracts vendor name and total amount from batch results for approval checks.
func extractReceiptInfo(batch sqlc.ReceiptBatch) (vendorName string, amount float64) {
	var results []service.ReceiptResult
	if err := json.Unmarshal(batch.Results, &results); err != nil || len(results) == 0 {
		return "", 0
	}
	if v, ok := results[0].Parsed.VendorName.Value.(string); ok {
		vendorName = v
	}
	switch a := results[0].Parsed.TotalAmount.Value.(type) {
	case float64:
		amount = a
	case int:
		amount = float64(a)
	}
	return vendorName, amount
}

// confirmAndProcess executes Phase 2: create transactions, classify, generate journal.
// userCategory overrides AI classification when non-empty (user selected a category button).
// Returns the reply string, transaction IDs, ref numbers, and error.
func (b *Bot) confirmAndProcess(ctx context.Context, batch sqlc.ReceiptBatch, tgUser sqlc.TelegramUser, jCfg jurisdiction.Config, projectTag *string, note string, userCategory string) (string, []uuid.UUID, []int32, error) {
	jurisdictionCode := jCfg.Code
	if jurisdictionCode == "" {
		jurisdictionCode = "PH"
	}

	_ = b.q.UpdateReceiptBatchStatus(ctx, sqlc.UpdateReceiptBatchStatusParams{
		ID:     batch.ID,
		Status: "completed",
	})

	period := time.Now().UTC().Format("2006-01")
	sessionID, err := b.getOrCreateSession(ctx, tgUser.CompanyID, tgUser.UserID, period)
	if err != nil {
		return "", nil, nil, fmt.Errorf("create session: %w", err)
	}

	txns, err := b.bridge.ConvertReceiptToTransactions(ctx, tgUser.CompanyID, batch.ID, sessionID, projectTag, tgUser.UserID)
	if err != nil {
		return "", nil, nil, fmt.Errorf("convert receipt: %w", err)
	}

	// Append user note to transaction descriptions.
	if note != "" {
		for _, txn := range txns {
			desc := derefStr(txn.Description)
			if desc != "" {
				desc = desc + " — " + note
			} else {
				desc = note
			}
			_ = b.q.UpdateTransactionDescription(ctx, sqlc.UpdateTransactionDescriptionParams{
				ID:          txn.ID,
				Description: &desc,
			})
		}
	}

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
		// Use the user note as a classification hint if available.
		classHint := ""
		if note != "" {
			classHint = "User context: " + note
		}
		classResults, err = b.classifier.ClassifyTransactions(ctx, classInput, tgUser.CompanyID, jurisdictionCode, classHint)
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

		// Override category if user explicitly selected one via category buttons.
		if userCategory != "" {
			var userConf pgtype.Numeric
			_ = userConf.Scan("1.00")
			for i, txn := range txns {
				vatType := ""
				if i < len(classResults) {
					vatType = classResults[i].VATType
				}
				_ = b.q.BulkUpdateTransactionClassification(ctx, sqlc.BulkUpdateTransactionClassificationParams{
					ID:                   txn.ID,
					VatType:              vatType,
					Category:             userCategory,
					Confidence:           userConf,
					ClassificationSource: "user_category",
				})
				if i < len(classResults) {
					classResults[i].Category = userCategory
					classResults[i].Confidence = 1.0
				}
			}
		}
	}

	var journalEntries []*domain.JournalEntry
	for _, txn := range txns {
		je, jeErr := b.journalGen.GenerateFromTransaction(ctx, tgUser.CompanyID, txn.ID, tgUser.UserID)
		if jeErr != nil {
			slog.Warn("journal generation failed for txn", "txn_id", txn.ID, "error", jeErr)
			continue
		}
		journalEntries = append(journalEntries, je)
	}

	// Auto-learning: record corrections from user edits.
	if len(txns) > 0 {
		b.recordReceiptCorrections(ctx, batch.ID, tgUser.CompanyID, tgUser.UserID, txns[0].ID)
	}

	// Extract transaction IDs and ref numbers for reply mapping.
	txnIDs := make([]uuid.UUID, len(txns))
	refNumbers := make([]int32, len(txns))
	for i, txn := range txns {
		txnIDs[i] = txn.ID
		refNumbers[i] = txn.RefNumber
	}

	var results []service.ReceiptResult
	_ = json.Unmarshal(batch.Results, &results)

	// Vendor memory learning: record acceptance or correction for vendor-based predictions.
	var learnMsg string
	if b.vendorMemory != nil && len(txns) > 0 && len(results) > 0 {
		vendorName := ""
		if v, ok := results[0].Parsed.VendorName.Value.(string); ok {
			vendorName = v
		}
		if vendorName != "" {
			finalCategory := userCategory
			if finalCategory == "" && len(classResults) > 0 {
				finalCategory = classResults[0].Category
			}
			accountCode := ""
			taxCode := ""
			if len(classResults) > 0 {
				accountCode = classResults[0].AccountCode
				taxCode = classResults[0].TaxCode
			}
			department := ""
			project := ""
			if projectTag != nil {
				project = *projectTag
			}

			// If user explicitly chose a category different from AI suggestion, record as correction.
			isCorrection := userCategory != "" && len(classResults) > 0 && classResults[0].Category != userCategory && classResults[0].ClassificationSource != "default"
			if isCorrection {
				if err := b.vendorMemory.RecordCorrection(ctx, tgUser.CompanyID, vendorName, userCategory, accountCode, taxCode); err != nil {
					slog.Warn("vendor memory correction failed", "vendor", vendorName, "error", err)
				} else {
					learnMsg = fmt.Sprintf("\n🧠 Updated: %s → %s (correction recorded)", vendorName, userCategory)
				}
			} else {
				// Record as acceptance.
				if err := b.vendorMemory.RecordAcceptance(ctx, tgUser.CompanyID, vendorName, finalCategory, accountCode, taxCode, department, project); err != nil {
					slog.Warn("vendor memory acceptance failed", "vendor", vendorName, "error", err)
				} else if finalCategory != "" {
					learnMsg = fmt.Sprintf("\n🧠 Learned: %s → %s", vendorName, finalCategory)
				}
			}
		}
	}

	if len(results) == 0 {
		return "Transaction recorded." + learnMsg, txnIDs, refNumbers, nil
	}

	// Multi-trip reply for app screenshots (Uber/Grab).
	var reply string
	if len(results) > 1 {
		reply = formatMultiTripReply(results, jCfg.CurrencySymbol, refNumbers)
	} else {
		reply = formatReceiptReply(results[0], len(txns), classResults, journalEntries, jCfg.CurrencySymbol, refNumbers)
	}

	// Append project tag and note to reply.
	if projectTag != nil && *projectTag != "" {
		reply += fmt.Sprintf("\nProject: %s", *projectTag)
	}
	if note != "" {
		reply += fmt.Sprintf("\nNote: %s", note)
	}

	// Append learning status.
	reply += learnMsg

	return reply, txnIDs, refNumbers, nil
}

// amountSelectionMarkup builds inline keyboard with amount buttons for user to pick.
// Each button shows "Label: Amount" and encodes batchID|index in callback data.
func amountSelectionMarkup(batchID uuid.UUID, amounts []service.DetectedAmount, currencySymbol string) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	var rows []tele.Row
	for i, da := range amounts {
		text := fmt.Sprintf("%s: %s%.2f", da.Label, currencySymbol, da.Amount)
		rows = append(rows, markup.Row(tele.Btn{
			Unique: btnAmountSelect.Unique,
			Text:   text,
			Data:   fmt.Sprintf("%s|%d", batchID.String(), i),
		}))
	}
	// Add "Other amount" and Cancel buttons.
	rows = append(rows, markup.Row(
		tele.Btn{Unique: btnAmountCustom.Unique, Text: "Other amount / 自定义金额", Data: batchID.String()},
	))
	rows = append(rows, markup.Row(
		tele.Btn{Unique: btnCancel.Unique, Text: "Cancel", Data: batchID.String()},
	))
	markup.Inline(rows...)
	return markup
}

// handleAmountSelect handles the amount selection button click.
func (b *Bot) handleAmountSelect(c tele.Context) error {
	parts := strings.SplitN(c.Data(), "|", 2)
	if len(parts) != 2 {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid data."})
	}

	batchID, err := uuid.Parse(parts[0])
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid batch ID."})
	}

	amountIdx, err := strconv.Atoi(parts[1])
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid amount index."})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	var results []service.ReceiptResult
	if err := json.Unmarshal(batch.Results, &results); err != nil || len(results) == 0 {
		return c.Respond(&tele.CallbackResponse{Text: "Failed to load receipt data."})
	}

	detected := results[0].Parsed.DetectedAmounts
	if amountIdx < 0 || amountIdx >= len(detected) {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid amount selection."})
	}

	selected := detected[amountIdx]
	_ = c.Respond(&tele.CallbackResponse{Text: fmt.Sprintf("Selected: %s", selected.Label)})

	// Apply selected amount.
	results[0].Parsed.TotalAmount = service.ParsedField{Value: selected.Amount, Confidence: 1.0}
	results[0].OverallConfidence = service.AverageConfidence(results[0].Parsed)

	resultsJSON, _ := json.Marshal(results)
	_ = b.q.UpdateReceiptBatch(ctx, sqlc.UpdateReceiptBatchParams{
		ID:      batch.ID,
		Status:  "pending_confirmation",
		Results: resultsJSON,
	})

	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	uploaderName := senderDisplayName(c.Sender())
	preview := formatReceiptPreview(results[0], jCfg.CurrencySymbol, uploaderName, "")

	// Proceed to project selection or category selection (skip note step).
	aiCat := extractAICategory(results)
	if len(b.projects) > 0 {
		markup := projectSelectionMarkup(batch.ID, b.projects)
		_, _ = b.B.Edit(c.Message(), preview+"\n\nPlease select a project:", markup)
	} else {
		markup := categorySelectionMarkup(batch.ID, "", receiptCategories, aiCat)
		_, _ = b.B.Edit(c.Message(), preview+"\n\nConfirm or select a category:", markup)
	}

	return nil
}

// handleAmountCustom handles the "Other amount" button click in the amount selection keyboard.
// It prompts the user to type a custom amount.
func (b *Bot) handleAmountCustom(c tele.Context) error {
	batchID, err := uuid.Parse(c.Data())
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid batch ID."})
	}

	_ = c.Respond(&tele.CallbackResponse{Text: "Enter custom amount"})

	b.pendingCustomAmount.Store(c.Sender().ID, &CustomAmountPending{BatchID: batchID})

	_, _ = b.B.Edit(c.Message(), c.Message().Text+"\n\nPlease type the amount (e.g. 1500.00 or 1500):\n请输入金额（例如 1500.00）：")
	return nil
}

// handleCustomAmountInput processes user text when they clicked "Other amount".
// Returns true if the message was consumed.
func (b *Bot) handleCustomAmountInput(c tele.Context, text string) bool {
	raw, ok := b.pendingCustomAmount.LoadAndDelete(c.Sender().ID)
	if !ok {
		return false
	}
	pending, ok := raw.(*CustomAmountPending)
	if !ok {
		return false
	}

	// Parse the amount from user input (strip currency symbols, commas, spaces).
	cleaned := strings.NewReplacer(
		"₱", "", "PHP", "", "P", "",
		"S$", "", "SGD", "",
		"Rs", "", "LKR", "",
		",", "", " ", "",
	).Replace(strings.TrimSpace(text))

	amount, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || amount <= 0 {
		_ = c.Send("Invalid amount. Please type a number (e.g. 1500.00):\n金额无效，请输入数字（例如 1500.00）：")
		b.pendingCustomAmount.Store(c.Sender().ID, pending) // re-store for retry
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	batch, err := b.q.GetReceiptBatchByID(ctx, pending.BatchID)
	if err != nil {
		_ = c.Send("Batch not found. Please send a new receipt.")
		return true
	}

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil || tgUser.CompanyID != batch.CompanyID {
		_ = c.Send("Unauthorized.")
		return true
	}

	if batch.Status != "pending_confirmation" {
		_ = c.Send("This receipt has already been processed.")
		return true
	}

	var results []service.ReceiptResult
	if err := json.Unmarshal(batch.Results, &results); err != nil || len(results) == 0 {
		_ = c.Send("Failed to load receipt data.")
		return true
	}

	// Apply custom amount.
	results[0].Parsed.TotalAmount = service.ParsedField{Value: amount, Confidence: 1.0}
	results[0].OverallConfidence = service.AverageConfidence(results[0].Parsed)

	resultsJSON, _ := json.Marshal(results)
	_ = b.q.UpdateReceiptBatch(ctx, sqlc.UpdateReceiptBatchParams{
		ID:      batch.ID,
		Status:  "pending_confirmation",
		Results: resultsJSON,
	})

	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	uploaderName := senderDisplayName(c.Sender())
	preview := formatReceiptPreview(results[0], jCfg.CurrencySymbol, uploaderName, "")

	// Proceed to project selection or category selection.
	aiCat := extractAICategory(results)
	if len(b.projects) > 0 {
		markup := projectSelectionMarkup(batch.ID, b.projects)
		_ = c.Send(preview+"\n\nPlease select a project:", markup)
	} else {
		markup := categorySelectionMarkup(batch.ID, "", receiptCategories, aiCat)
		_ = c.Send(preview+"\n\nConfirm or select a category:", markup)
	}

	return true
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
		return
	}

	_ = b.q.UpdateReceiptBatchStatus(ctx, sqlc.UpdateReceiptBatchStatusParams{
		ID:     batchID,
		Status: "cancelled",
	})

	msg := &tele.Message{ID: msgID, Chat: &tele.Chat{ID: chatID}}
	_, _ = b.B.Edit(msg, "Receipt expired (5 min timeout). Please send the photo again.")
}

// handleCustomCategoryInput processes user text when they selected "Other" category.
// Returns true if the message was consumed.
func (b *Bot) handleCustomCategoryInput(c tele.Context, text string) bool {
	raw, ok := b.pendingCustomCategory.LoadAndDelete(c.Sender().ID)
	if !ok {
		return false
	}
	pending, ok := raw.(*CustomCategoryPending)
	if !ok {
		return false
	}

	category := strings.TrimSpace(text)
	if category == "" {
		_ = c.Send("Category cannot be empty. Please type a category name:")
		b.pendingCustomCategory.Store(c.Sender().ID, pending)
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	batch, err := b.q.GetReceiptBatchByID(ctx, pending.BatchID)
	if err != nil {
		_ = c.Send("Batch not found. Please send a new receipt.")
		return true
	}

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil || tgUser.CompanyID != batch.CompanyID {
		_ = c.Send("Unauthorized.")
		return true
	}

	if batch.Status != "pending_confirmation" {
		_ = c.Send("This receipt has already been processed.")
		return true
	}

	// Check if approval is required before processing.
	vendorName, amount := extractReceiptInfo(batch)
	isDuplicate := batch.ImageHash != nil
	if b.checkAndRequestApproval(c, ctx, pending.BatchID, tgUser.CompanyID, tgUser.UserID, amount, vendorName, isDuplicate) {
		_ = c.Send("Pending approval — an approver has been notified.")
		return true
	}

	_ = c.Send("Recording transaction...")

	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	var projPtr *string
	if pending.ProjectTag != "" {
		projPtr = &pending.ProjectTag
	}

	var note string
	if rawNote, ok := b.receiptNotes.LoadAndDelete(pending.BatchID); ok {
		note, _ = rawNote.(string)
	}

	reply, txnIDs, refNumbers, err := b.confirmAndProcess(ctx, batch, tgUser, jCfg, projPtr, note, category)
	if err != nil {
		slog.Error("confirm processing failed", "batch_id", pending.BatchID, "error", err)
		_ = c.Send("Failed to record transaction.")
		return true
	}

	sent, _ := b.B.Send(c.Chat(), reply)
	b.storeReplyMapping(c.Chat().ID, sent, txnIDs, refNumbers)

	// Proactively suggest vendor rules after successful confirmation.
	go b.checkAndSendRuleSuggestions(c.Chat().ID, tgUser.CompanyID)

	return true
}

// recordReceiptCorrections compares original OCR results with user-edited results
// and records corrections for auto-learning.
func (b *Bot) recordReceiptCorrections(ctx context.Context, batchID, companyID, userID, txnID uuid.UUID) {
	if b.corrections == nil {
		return
	}

	raw, ok := b.originalResults.LoadAndDelete(batchID)
	if !ok {
		return // no edits were made
	}
	original, ok := raw.(service.ReceiptResult)
	if !ok {
		return
	}

	// Load the current (edited) results from DB.
	batch, err := b.q.GetReceiptBatchByID(ctx, batchID)
	if err != nil {
		return
	}

	var current []service.ReceiptResult
	if err := json.Unmarshal(batch.Results, &current); err != nil || len(current) == 0 {
		return
	}

	edited := current[0]

	// Compare each field and record corrections for differences.
	type fieldPair struct {
		name     string
		oldField service.ParsedField
		newField service.ParsedField
	}
	pairs := []fieldPair{
		{"total_amount", original.Parsed.TotalAmount, edited.Parsed.TotalAmount},
		{"vendor_name", original.Parsed.VendorName, edited.Parsed.VendorName},
		{"date", original.Parsed.Date, edited.Parsed.Date},
		{"vat_amount", original.Parsed.VATAmount, edited.Parsed.VATAmount},
		{"vat_type", original.Parsed.VATType, edited.Parsed.VATType},
		{"category", original.Parsed.Category, edited.Parsed.Category},
		{"tin", original.Parsed.TIN, edited.Parsed.TIN},
		{"receipt_number", original.Parsed.ReceiptNumber, edited.Parsed.ReceiptNumber},
	}

	var recorded int
	for _, p := range pairs {
		oldStr := fieldValueString(p.oldField)
		newStr := fieldValueString(p.newField)
		if oldStr == newStr {
			continue
		}
		// User changed this field — record correction.
		reason := "telegram_bot_edit"
		_, err := b.corrections.RecordCorrection(ctx, service.RecordCorrectionInput{
			CompanyID:  companyID,
			UserID:     userID,
			EntityType: "receipt_field",
			EntityID:   txnID,
			FieldName:  p.name,
			OldValue:   ptrStr(oldStr),
			NewValue:   newStr,
			Reason:     &reason,
		})
		if err != nil {
			slog.Warn("record correction failed", "field", p.name, "error", err)
			continue
		}
		recorded++
	}

	if recorded > 0 {
		slog.Info("receipt corrections recorded (auto-learning)",
			"batch_id", batchID,
			"corrections", recorded,
		)
	}
}

// fieldValueString converts a ParsedField value to a comparable string.
func fieldValueString(f service.ParsedField) string {
	if f.Value == nil {
		return ""
	}
	return fmt.Sprintf("%v", f.Value)
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
