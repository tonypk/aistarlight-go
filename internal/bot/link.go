package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	tele "gopkg.in/telebot.v3"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

const startMsg = `Welcome to AIStarlight — Your Smart Bookkeeping Assistant!

I can help you manage your business finances through natural conversation.

What I can do:
- Record expenses: "I spent 500 on office supplies"
- Check spending: "How much did I spend this month?"
- Search transactions: "Find all restaurant expenses"
- Process receipts: Just send a photo!
- Forex: /exchange
- Export data: /export [YYYY-MM]
- View stats: /status

To get started:
1. Get your API key from the web dashboard
2. Run /link <your_api_key>
3. Start chatting or send a receipt photo!`

func (b *Bot) handleStart(c tele.Context) error {
	payload := strings.TrimSpace(c.Message().Payload)

	// Deep link: /start lt_<token>
	if strings.HasPrefix(payload, "lt_") {
		return b.handleDeepLink(c, strings.TrimPrefix(payload, "lt_"))
	}

	return c.Send(startMsg)
}

func (b *Bot) handleDeepLink(c tele.Context, token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	lt, err := b.q.GetValidLinkToken(ctx, token)
	if err != nil {
		if isNotFound(err) {
			return c.Send("Link expired or invalid. Please request a new link.")
		}
		slog.Error("db error on GetValidLinkToken", "error", err)
		return c.Send("Service temporarily unavailable. Please try again later.")
	}

	// Mark token as used
	_ = b.q.MarkLinkTokenUsed(ctx, lt.ID)

	// Upsert telegram user
	sender := c.Sender()
	var username, fullName *string
	if sender.Username != "" {
		username = &sender.Username
	}
	name := strings.TrimSpace(sender.FirstName + " " + sender.LastName)
	if name != "" {
		fullName = &name
	}

	_, err = b.q.UpsertTelegramUser(ctx, sqlc.UpsertTelegramUserParams{
		TelegramID: sender.ID,
		UserID:     lt.UserID,
		CompanyID:  lt.CompanyID,
		ChatID:     c.Chat().ID,
		Username:   username,
		FullName:   fullName,
	})
	if err != nil {
		slog.Error("failed to upsert telegram user via deep link", "error", err)
		return c.Send("Failed to link account. Please try again.")
	}

	// Fetch company name for confirmation
	company, err := b.q.GetCompanyByID(ctx, lt.CompanyID)
	if err != nil {
		return c.Send("Account linked! You can now send receipt photos.")
	}

	return c.Send(fmt.Sprintf("Linked to %s\n\nYou can now send receipt photos!", company.CompanyName))
}

func (b *Bot) handleLink(c tele.Context) error {
	slog.Info("handleLink called", "user_id", c.Sender().ID, "text", c.Text())
	args := strings.TrimSpace(c.Message().Payload)
	if args == "" {
		return c.Send("Usage: /link <api_key>\n\nGet your API key from the web dashboard.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Look up user by API key
	apiKey := args // local copy for pointer safety
	user, err := b.q.GetUserByAPIKey(ctx, &apiKey)
	if err != nil {
		if isNotFound(err) {
			return c.Send("Invalid API key. Please check and try again.")
		}
		slog.Error("db error on GetUserByAPIKey", "error", err)
		return c.Send("Service temporarily unavailable. Please try again later.")
	}

	// Find the user's first company
	company, err := b.q.GetFirstCompanyByUser(ctx, user.ID)
	if err != nil {
		if isNotFound(err) {
			return c.Send("No company found for this account. Please create a company first.")
		}
		slog.Error("db error on GetFirstCompanyByUser", "error", err)
		return c.Send("Service temporarily unavailable. Please try again later.")
	}

	// Upsert telegram user mapping
	sender := c.Sender()
	var username, fullName *string
	if sender.Username != "" {
		username = &sender.Username
	}
	name := strings.TrimSpace(sender.FirstName + " " + sender.LastName)
	if name != "" {
		fullName = &name
	}

	_, err = b.q.UpsertTelegramUser(ctx, sqlc.UpsertTelegramUserParams{
		TelegramID: sender.ID,
		UserID:     user.ID,
		CompanyID:  company.ID,
		ChatID:     c.Chat().ID,
		Username:   username,
		FullName:   fullName,
	})
	if err != nil {
		slog.Error("failed to upsert telegram user", "error", err, "telegram_id", sender.ID)
		return c.Send("Failed to link account. Please try again.")
	}

	return c.Send(fmt.Sprintf("Linked to %s\n\nYou can now send receipt photos!", company.CompanyName))
}

func (b *Bot) handleStatus(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		if isNotFound(err) {
			return c.Send("Account not linked. Use /link <api_key> first.")
		}
		slog.Error("db error on GetTelegramUser", "error", err)
		return c.Send("Service temporarily unavailable. Please try again later.")
	}

	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	month := now.Format("Jan 2006")

	// Basic stats.
	stats, err := b.q.GetTransactionStatsSince(ctx, sqlc.GetTransactionStatsSinceParams{
		CompanyID: tgUser.CompanyID,
		CreatedAt: monthStart,
	})
	if err != nil {
		slog.Error("failed to fetch transaction stats", "error", err)
		return c.Send("Failed to fetch stats.")
	}

	total := formatInterface(stats.TotalAmount)
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s: %d transactions, %s total\n", month, stats.Count, total)

	// Category breakdown.
	catRows, err := b.q.GetSpendingSummaryByCategory(ctx, sqlc.GetSpendingSummaryByCategoryParams{
		CompanyID: tgUser.CompanyID,
		Date:      pgtype.Date{Time: monthStart, Valid: true},
		Date_2:    pgtype.Date{Time: now, Valid: true},
	})
	if err == nil && len(catRows) > 0 {
		sb.WriteString("\nBy category:\n")
		for _, r := range catRows {
			fmt.Fprintf(&sb, "  %s: %s (%d)\n", capitalize(r.Category), r.Total, r.Count)
		}
	}

	// Recent 3 transactions preview.
	recent, err := b.q.GetRecentTransactionsByCompany(ctx, sqlc.GetRecentTransactionsByCompanyParams{
		CompanyID: tgUser.CompanyID,
		Limit:     3,
	})
	if err == nil && len(recent) > 0 {
		sb.WriteString("\nRecent:\n")
		for _, t := range recent {
			desc := ""
			if t.Description != nil {
				desc = *t.Description
			}
			if len(desc) > 30 {
				desc = desc[:30] + "..."
			}
			amt := ""
			if f, fErr := t.Amount.Float64Value(); fErr == nil {
				amt = fmt.Sprintf("%.2f", f.Float64)
			}
			dateStr := ""
			if t.Date.Valid {
				dateStr = t.Date.Time.Format("01/02")
			}
			fmt.Fprintf(&sb, "  %s %s — %s\n", dateStr, desc, amt)
		}
	}

	return c.Send(sb.String())
}
