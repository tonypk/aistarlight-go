package bot

import (
	"context"
	"log/slog"
	"strings"
	"time"

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
