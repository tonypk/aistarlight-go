package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	tele "gopkg.in/telebot.v3"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/jurisdiction"
)

// ForexPending tracks multi-step /exchange conversation state.
type ForexPending struct {
	Step        string          // "from_amount", "to_amount", "description"
	FromAmount  decimal.Decimal
	ToAmount    decimal.Decimal
	Rate        decimal.Decimal
	Description string
	ProjectTag  string
	ChatID      int64
	MsgID       int
	CreatedAt   time.Time
}

// Inline keyboard buttons for forex confirmation (separate Unique IDs from receipt).
var (
	btnFxConfirm = tele.Btn{Unique: "fx_ok", Text: "Confirm"}
	btnFxEdit    = tele.Btn{Unique: "fx_ed", Text: "Edit"}
	btnFxCancel  = tele.Btn{Unique: "fx_no", Text: "Cancel"}
	btnFxProject = tele.Btn{Unique: "fx_pj", Text: "Project"}
)

const forexTimeout = 5 * time.Minute

// handleExchange is the /exchange command entry point.
func (b *Bot) handleExchange(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Account not linked. Use /link <api_key> first.")
	}

	msg, err := b.B.Send(c.Chat(), "Enter the USDT amount sent: (e.g., 500)")
	if err != nil {
		return err
	}

	pending := &ForexPending{
		Step:      "from_amount",
		ChatID:    c.Chat().ID,
		MsgID:     msg.ID,
		CreatedAt: time.Now(),
	}
	b.pendingForex.Store(c.Sender().ID, pending)

	go b.forexTimeout(c.Sender().ID, c.Chat().ID, msg.ID)

	return nil
}

// handleForexInput processes text input for pending forex flows.
// Returns true if the message was consumed by a forex flow.
func (b *Bot) handleForexInput(c tele.Context, text string) bool {
	raw, ok := b.pendingForex.Load(c.Sender().ID)
	if !ok {
		return false
	}
	pending, ok := raw.(*ForexPending)
	if !ok {
		b.pendingForex.Delete(c.Sender().ID)
		return false
	}

	switch pending.Step {
	case "from_amount":
		amount, err := parseDecimalInput(text)
		if err != nil || amount.LessThanOrEqual(decimal.Zero) {
			_ = c.Send("Invalid amount. Please enter a positive number (e.g., 500).")
			return true
		}
		pending.FromAmount = amount
		pending.Step = "to_amount"
		b.pendingForex.Store(c.Sender().ID, pending)

		_ = c.Send(fmt.Sprintf("USDT %s\nEnter the local currency amount received: (e.g., 153,500)",
			addCommas(amount.StringFixed(2))))
		return true

	case "to_amount":
		amount, err := parseDecimalInput(text)
		if err != nil || amount.LessThanOrEqual(decimal.Zero) {
			_ = c.Send("Invalid amount. Please enter a positive number (e.g., 153500).")
			return true
		}
		pending.ToAmount = amount
		pending.Rate = amount.Div(pending.FromAmount).Round(2)
		pending.Step = "description"
		b.pendingForex.Store(c.Sender().ID, pending)

		_ = c.Send("Enter a description (e.g., Binance P2P, OTC broker):")
		return true

	case "description":
		pending.Description = text
		pending.Step = "project_select"
		b.pendingForex.Store(c.Sender().ID, pending)

		b.showForexProjectOrConfirm(c, pending)
		return true
	}

	return false
}

// showForexProjectOrConfirm shows project selection or confirmation depending on config.
func (b *Bot) showForexProjectOrConfirm(c tele.Context, pending *ForexPending) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		_ = c.Send("Account not linked.")
		b.pendingForex.Delete(c.Sender().ID)
		return
	}

	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	preview := formatForexPreview(pending, jCfg.CurrencySymbol, "")

	if len(b.projects) > 0 {
		markup := forexProjectSelectionMarkup(b.projects)
		_, _ = b.B.Send(c.Chat(), preview, markup)
	} else {
		markup := forexConfirmationMarkup("")
		_, _ = b.B.Send(c.Chat(), preview, markup)
	}
}

// handleForexProjectSelect handles project button click in forex flow.
func (b *Bot) handleForexProjectSelect(c tele.Context) error {
	projectTag := c.Data()

	raw, ok := b.pendingForex.Load(c.Sender().ID)
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "No pending exchange."})
	}
	pending, ok := raw.(*ForexPending)
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid state."})
	}

	pending.ProjectTag = projectTag
	b.pendingForex.Store(c.Sender().ID, pending)

	_ = c.Respond(&tele.CallbackResponse{Text: "Project: " + projectTag})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		_, _ = b.B.Edit(c.Message(), "Account not linked.")
		return nil
	}
	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	preview := formatForexPreview(pending, jCfg.CurrencySymbol, projectTag)
	markup := forexConfirmationMarkup(projectTag)
	_, _ = b.B.Edit(c.Message(), preview, markup)

	return nil
}

// handleForexConfirm records the forex exchange transaction.
func (b *Bot) handleForexConfirm(c tele.Context) error {
	projectTag := c.Data()

	raw, ok := b.pendingForex.Load(c.Sender().ID)
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "No pending exchange."})
	}
	pending, ok := raw.(*ForexPending)
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid state."})
	}

	b.pendingForex.Delete(c.Sender().ID)

	_ = c.Respond(&tele.CallbackResponse{})
	_, _ = b.B.Edit(c.Message(), "Recording exchange...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tgUser, err := b.q.GetTelegramUser(ctx, c.Sender().ID)
	if err != nil {
		_, _ = b.B.Edit(c.Message(), "Account not linked.")
		return nil
	}

	company, compErr := b.q.GetCompanyByID(ctx, tgUser.CompanyID)
	jurisdictionCode := "PH"
	if compErr == nil {
		jurisdictionCode = company.Jurisdiction
	}
	jCfg := jurisdiction.Get(jurisdictionCode)

	period := time.Now().UTC().Format("2006-01")
	sessionID, err := b.getOrCreateSession(ctx, tgUser.CompanyID, tgUser.UserID, period)
	if err != nil {
		slog.Error("forex: failed to get/create session", "error", err)
		_, _ = b.B.Edit(c.Message(), "Failed to create session.")
		return nil
	}

	// Build numeric values for DB.
	amountNum := pgtype.Numeric{}
	_ = amountNum.Scan(pending.ToAmount.String())

	vatAmountNum := pgtype.Numeric{}
	_ = vatAmountNum.Scan("0")

	confidenceNum := pgtype.Numeric{}
	_ = confidenceNum.Scan("1.00")

	rateNum := pgtype.Numeric{}
	_ = rateNum.Scan(pending.Rate.StringFixed(6))

	fromAmountNum := pgtype.Numeric{}
	_ = fromAmountNum.Scan(pending.FromAmount.String())

	fromCurrency := "USDT"
	toCurrency := jCfg.Currency
	desc := pending.Description
	if desc == "" {
		desc = "P2P Exchange"
	}

	var projPtr *string
	if projectTag != "" {
		projPtr = &projectTag
	} else if pending.ProjectTag != "" {
		projPtr = &pending.ProjectTag
	}

	txnDate := pgtype.Date{Time: time.Now().UTC(), Valid: true}

	submittedByPg := pgtype.UUID{Bytes: tgUser.UserID, Valid: true}

	dbTxn, err := b.q.CreateTransaction(ctx, sqlc.CreateTransactionParams{
		ID:                   uuid.New(),
		CompanyID:            tgUser.CompanyID,
		SessionID:            sessionID,
		SourceType:           "forex_exchange",
		SourceFileID:         "telegram_bot",
		RowIndex:             0,
		Date:                 txnDate,
		Description:          &desc,
		Amount:               amountNum,
		VatAmount:            vatAmountNum,
		VatType:              "exempt",
		Category:             "services",
		Confidence:           confidenceNum,
		ClassificationSource: "manual",
		MatchStatus:          "unmatched",
		ProjectTag:           projPtr,
		FromCurrency:         &fromCurrency,
		ToCurrency:           &toCurrency,
		ExchangeRate:         rateNum,
		FromAmount:           fromAmountNum,
		SubmittedBy:          submittedByPg,
	})
	if err != nil {
		slog.Error("forex: failed to create transaction", "error", err)
		_, _ = b.B.Edit(c.Message(), "Failed to record exchange.")
		return nil
	}

	var refNum int32
	if dbTxn.RefNumber != nil {
		refNum = *dbTxn.RefNumber
	}

	reply := formatForexReply(pending, jCfg.CurrencySymbol, projectTag, refNum)
	msg, _ := b.B.Edit(c.Message(), reply)
	b.storeReplyMapping(c.Chat().ID, msg, []uuid.UUID{dbTxn.ID}, []int32{refNum})
	return nil
}

// handleForexEdit resets the exchange flow to step 1.
func (b *Bot) handleForexEdit(c tele.Context) error {
	b.pendingForex.Delete(c.Sender().ID)
	_ = c.Respond(&tele.CallbackResponse{})
	_, _ = b.B.Edit(c.Message(), "Enter the USDT amount sent: (e.g., 500)")

	pending := &ForexPending{
		Step:      "from_amount",
		ChatID:    c.Chat().ID,
		MsgID:     c.Message().ID,
		CreatedAt: time.Now(),
	}
	b.pendingForex.Store(c.Sender().ID, pending)

	go b.forexTimeout(c.Sender().ID, c.Chat().ID, c.Message().ID)

	return nil
}

// handleForexCancel cancels the pending exchange.
func (b *Bot) handleForexCancel(c tele.Context) error {
	b.pendingForex.Delete(c.Sender().ID)
	_ = c.Respond(&tele.CallbackResponse{})
	_, _ = b.B.Edit(c.Message(), "Exchange cancelled.")
	return nil
}

// forexProjectSelectionMarkup builds inline keyboard with project buttons.
func forexProjectSelectionMarkup(projects []string) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	var btns []tele.Btn
	for _, p := range projects {
		btns = append(btns, tele.Btn{
			Unique: btnFxProject.Unique,
			Text:   p,
			Data:   p,
		})
	}
	rows := []tele.Row{markup.Row(btns...)}
	rows = append(rows, markup.Row(
		tele.Btn{Unique: btnFxCancel.Unique, Text: "Cancel"},
	))
	markup.Inline(rows...)
	return markup
}

// forexConfirmationMarkup builds confirm/edit/cancel buttons.
func forexConfirmationMarkup(projectTag string) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(
			tele.Btn{Unique: btnFxConfirm.Unique, Text: "Confirm", Data: projectTag},
			tele.Btn{Unique: btnFxEdit.Unique, Text: "Edit"},
			tele.Btn{Unique: btnFxCancel.Unique, Text: "Cancel"},
		),
	)
	return markup
}

// formatForexPreview builds the exchange preview message.
func formatForexPreview(p *ForexPending, currencySymbol, projectTag string) string {
	var lines []string
	lines = append(lines, "Exchange Preview\n")
	lines = append(lines, fmt.Sprintf("Send: USDT %s", addCommas(p.FromAmount.StringFixed(2))))
	lines = append(lines, fmt.Sprintf("Receive: %s%s", currencySymbol, addCommas(p.ToAmount.StringFixed(2))))
	lines = append(lines, fmt.Sprintf("Rate: 1 USDT = %s%s", currencySymbol, addCommas(p.Rate.StringFixed(2))))
	if p.Description != "" {
		lines = append(lines, fmt.Sprintf("Description: %s", p.Description))
	}

	if projectTag != "" {
		lines = append(lines, fmt.Sprintf("\nProject: %s", projectTag))
		lines = append(lines, "\nPlease review and confirm:")
	} else {
		lines = append(lines, "\nPlease select a project:")
	}

	return strings.Join(lines, "\n")
}

// formatForexReply builds the final "Exchange Recorded" message.
func formatForexReply(p *ForexPending, currencySymbol, projectTag string, refNumber int32) string {
	var lines []string
	if refNumber > 0 {
		lines = append(lines, fmt.Sprintf("Exchange Recorded #TXN-%d", refNumber))
	} else {
		lines = append(lines, "Exchange Recorded")
	}
	lines = append(lines, fmt.Sprintf("%s USDT → %s%s",
		addCommas(p.FromAmount.StringFixed(2)),
		currencySymbol,
		addCommas(p.ToAmount.StringFixed(2))))
	lines = append(lines, fmt.Sprintf("Rate: 1 USDT = %s%s", currencySymbol, addCommas(p.Rate.StringFixed(2))))
	if p.Description != "" {
		lines = append(lines, fmt.Sprintf("Description: %s", p.Description))
	}
	if projectTag != "" {
		lines = append(lines, fmt.Sprintf("Project: %s", projectTag))
	}
	return strings.Join(lines, "\n")
}

// forexTimeout cancels a pending forex exchange after timeout.
func (b *Bot) forexTimeout(userID int64, chatID int64, msgID int) {
	time.Sleep(forexTimeout)

	raw, ok := b.pendingForex.Load(userID)
	if !ok {
		return
	}
	pending, ok := raw.(*ForexPending)
	if !ok {
		return
	}
	// Only expire if this is the same flow (same message).
	if pending.MsgID != msgID {
		return
	}

	b.pendingForex.Delete(userID)

	msg := &tele.Message{ID: msgID, Chat: &tele.Chat{ID: chatID}}
	_, _ = b.B.Edit(msg, "Exchange expired (5 min timeout). Use /exchange to start again.")
}

// parseDecimalInput parses a user-entered amount string, stripping commas.
func parseDecimalInput(s string) (decimal.Decimal, error) {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	// Handle currency prefixes users might type.
	for _, prefix := range []string{"$", "₱", "Rs", "S$", "USDT", "LKR", "PHP", "SGD"} {
		s = strings.TrimPrefix(s, prefix)
	}
	s = strings.TrimSpace(s)
	if _, err := strconv.ParseFloat(s, 64); err != nil {
		return decimal.Zero, err
	}
	return decimal.NewFromString(s)
}
