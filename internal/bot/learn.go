package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	tele "gopkg.in/telebot.v3"
)

// Inline buttons for rule suggestion prompts.
var (
	btnLearnAccept = tele.Btn{Unique: "learn_ac", Text: "Accept"}
	btnLearnIgnore = tele.Btn{Unique: "learn_ig", Text: "Ignore"}
)

// checkAndSendRuleSuggestions is called asynchronously after a receipt is
// confirmed to proactively suggest vendor rules that have accumulated
// enough corrections but haven't been promoted to defaults yet.
func (b *Bot) checkAndSendRuleSuggestions(chatID int64, companyID uuid.UUID) {
	ctx := context.Background()

	suggestions, err := b.vendorMemory.SuggestRules(ctx, companyID, 5)
	if err != nil {
		slog.Warn("failed to fetch rule suggestions", "error", err)
		return
	}

	if len(suggestions) == 0 {
		return
	}

	// Send at most 1 suggestion per confirmation to avoid spamming.
	s := suggestions[0]

	text := fmt.Sprintf(
		"💡 *Learning Suggestion*\n\n"+
			"You've corrected *%s* %d times.\n"+
			"Suggested defaults:\n"+
			"• Category: `%s`\n"+
			"• Account: `%s`\n"+
			"• Tax Code: `%s`\n\n"+
			"Set as default for future receipts?",
		mdEscape(s.VendorNormalized),
		s.CorrectionCount,
		nvl(s.DefaultCategory, "-"),
		nvl(s.AccountCode, "-"),
		nvl(s.TaxCode, "-"),
	)

	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(
			tele.Btn{Unique: btnLearnAccept.Unique, Text: btnLearnAccept.Text, Data: s.VendorNormalized},
			tele.Btn{Unique: btnLearnIgnore.Unique, Text: btnLearnIgnore.Text, Data: s.VendorNormalized},
		),
	)

	chat := &tele.Chat{ID: chatID}
	if _, err := b.B.Send(chat, text, markup, tele.ModeMarkdown); err != nil {
		slog.Warn("failed to send rule suggestion", "error", err, "chat_id", chatID)
	}
}

// handleLearnAccept processes the "Accept" button on a rule suggestion.
func (b *Bot) handleLearnAccept(c tele.Context) error {
	ctx := context.Background()
	vendorName := c.Callback().Data

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Please /link your account first."})
	}

	if err := b.vendorMemory.PromoteRule(ctx, tgUser.CompanyID, vendorName); err != nil {
		slog.Error("failed to promote rule", "error", err, "vendor", vendorName)
		return c.Respond(&tele.CallbackResponse{Text: "Failed to save rule."})
	}

	_, _ = b.B.Edit(c.Message(),
		fmt.Sprintf("✅ Default set for *%s*. Future receipts will use these values automatically.", mdEscape(vendorName)),
		tele.ModeMarkdown,
	)

	return c.Respond(&tele.CallbackResponse{Text: "Rule accepted!"})
}

// handleLearnIgnore processes the "Ignore" button on a rule suggestion.
func (b *Bot) handleLearnIgnore(c tele.Context) error {
	vendorName := c.Callback().Data

	_, _ = b.B.Edit(c.Message(),
		fmt.Sprintf("⏭ Suggestion for *%s* dismissed.", mdEscape(vendorName)),
		tele.ModeMarkdown,
	)

	return c.Respond(&tele.CallbackResponse{Text: "Dismissed"})
}

// nvl returns val if non-empty, otherwise the fallback.
func nvl(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

var mdReplacer = strings.NewReplacer(
	"_", "\\_",
	"*", "\\*",
	"[", "\\[",
	"`", "\\`",
)

// mdEscape escapes special Markdown V1 characters for Telegram.
func mdEscape(s string) string {
	return mdReplacer.Replace(s)
}
