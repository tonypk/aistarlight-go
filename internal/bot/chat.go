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

	resp, err := b.chat.ProcessMessage(ctx, text, history, tgUser.CompanyID, company.Jurisdiction)
	if err != nil {
		slog.Error("chat processing failed", "error", err)
		return c.Send("Sorry, I couldn't process your message. Please try again.")
	}

	// Save assistant response.
	_ = b.chat.SaveMessage(ctx, tgUser.CompanyID, tgUser.UserID, "assistant", resp.Response, resp.ToolCalls)

	// Split long messages to respect Telegram's 4096-char limit.
	return sendLongMessage(c, resp.Response)
}

// isReceiptInstruction detects if a message is an instruction for receipt processing.
// Returns true for messages like:
//   - "记录图片中的 net total" / "请你记录这张发票"
//   - "record the net total" / "use the amount 1500"
//   - "amount: 1500" / "vendor: ABC"
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
			case "amount", "vendor", "date", "vat", "category", "金额", "总额", "商家", "日期", "税额", "类别", "total":
				return true
			}
		}
	}

	// Chinese instruction keywords.
	cnKeywords := []string{"记录", "识别", "这张", "图片中", "照片中", "发票", "收据", "请你记", "帮我记", "帮我识别",
		"总金额", "金额是", "总额是", "用这个", "net total"}
	for _, kw := range cnKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}

	// English instruction keywords (must match 2+ to avoid false positives with general chat).
	enKeywords := []string{"record", "receipt", "invoice", "amount", "total", "vendor", "use the"}
	matchCount := 0
	for _, kw := range enKeywords {
		if strings.Contains(lower, kw) {
			matchCount++
		}
	}
	return matchCount >= 2
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
