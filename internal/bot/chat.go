package bot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	tele "gopkg.in/telebot.v3"
)

const telegramMaxMessageLen = 4096

func (b *Bot) handleText(c tele.Context) error {
	text := strings.TrimSpace(c.Text())
	if text == "" {
		return nil
	}

	// Ignore commands (handled by other handlers).
	if strings.HasPrefix(text, "/") {
		return nil
	}

	// Check for pending receipt edit reply.
	if rawBatchID, ok := b.pendingEdits.LoadAndDelete(c.Sender().ID); ok {
		if batchID, ok := rawBatchID.(uuid.UUID); ok {
			return b.handleReceiptEditReply(c, batchID, text)
		}
	}

	// Check for pending receipt note input.
	if b.handleReceiptNoteInput(c, text) {
		return nil
	}

	// Check for pending forex exchange input.
	if b.handleForexInput(c, text) {
		return nil
	}

	// Check if this looks like a receipt instruction (e.g., "record the net total").
	if isReceiptInstruction(text) {
		b.pendingInstructions.Store(c.Sender().ID, text)
		return c.Send("Got it! Now send the receipt photo.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Account not linked. Use /link <api_key> first.")
	}

	company, err := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	if err != nil {
		slog.Error("failed to get company", "error", err)
		return c.Send("Failed to load company info.")
	}

	// Load recent chat history.
	history, err := b.chat.ListHistory(ctx, tgUser.CompanyID, 10)
	if err != nil {
		slog.Warn("failed to load chat history", "error", err)
		// Continue without history.
		history = nil
	}

	// Save user message.
	_ = b.chat.SaveMessage(ctx, tgUser.CompanyID, tgUser.UserID, "user", text, nil)

	resp, err := b.chat.ProcessMessage(ctx, text, history, tgUser.CompanyID, company.Jurisdiction, tgUser.UserID)
	if err != nil {
		slog.Error("chat processing failed", "error", err)
		return c.Send("Sorry, I couldn't process your message. Please try again.")
	}

	// Save assistant response.
	_ = b.chat.SaveMessage(ctx, tgUser.CompanyID, tgUser.UserID, "assistant", resp.Response, resp.ToolCalls)

	// Split long messages to respect Telegram's 4096-char limit.
	return sendLongMessage(c, resp.Response)
}

// isReceiptInstruction detects if a message is an instruction for upcoming receipt photo processing.
// Must NOT match standalone expense descriptions like "I spent 500 on lunch" — those go to AI chat.
// Matches: "record the net total", "记录net total", "amount: 1500", etc.
func isReceiptInstruction(text string) bool {
	lower := strings.ToLower(text)

	// Direct key:value format (same as edit format).
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			parts = strings.SplitN(line, "：", 2) // Chinese colon
		}
		if len(parts) == 2 {
			key := strings.TrimSpace(strings.ToLower(parts[0]))
			switch key {
			case "amount", "vendor", "date", "vat", "category", "total",
				"金额", "总额", "商家", "日期", "税额", "类别":
				return true
			}
		}
	}

	// Chinese: standalone photo/receipt keywords — always match.
	standaloneKw := []string{"图片中", "照片中", "这张发票", "这张收据", "帮我识别"}
	for _, kw := range standaloneKw {
		if strings.Contains(lower, kw) {
			return true
		}
	}

	// Chinese: "记录/识别" combined with receipt-related words.
	cnAction := strings.Contains(lower, "记录") || strings.Contains(lower, "识别") ||
		strings.Contains(lower, "帮我记") || strings.Contains(lower, "请你记")
	cnReceiptRef := strings.Contains(lower, "total") || strings.Contains(lower, "net total") ||
		strings.Contains(lower, "总金额") || strings.Contains(lower, "总额") ||
		strings.Contains(lower, "金额是") || strings.Contains(lower, "发票") ||
		strings.Contains(lower, "收据")
	if cnAction && cnReceiptRef {
		return true
	}

	// English: 2+ receipt-related keywords (won't match "I spent 500 on lunch").
	enKeywords := []string{"record", "receipt", "invoice", "total", "vendor", "use the", "net total"}
	matchCount := 0
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			matchCount++
		}
	}
	if matchCount >= 2 {
		return true
	}

	return false
}

func sendLongMessage(c tele.Context, text string) error {
	if len(text) <= telegramMaxMessageLen {
		return c.Send(text)
	}

	for len(text) > 0 {
		end := telegramMaxMessageLen
		if end > len(text) {
			end = len(text)
		}

		// Try to break at a newline for cleaner splits.
		if end < len(text) {
			if idx := strings.LastIndex(text[:end], "\n"); idx > 0 {
				end = idx + 1
			}
		}

		if err := c.Send(text[:end]); err != nil {
			return err
		}
		text = text[end:]
	}
	return nil
}
