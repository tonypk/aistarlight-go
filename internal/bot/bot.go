package bot

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	tele "gopkg.in/telebot.v3"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// ReplyTxnData stores transaction info linked to a bot success message,
// enabling reply-to correction.
type ReplyTxnData struct {
	TxnIDs     []uuid.UUID
	RefNumbers []int32
}

// Bot wraps the Telegram bot and its dependencies.
type Bot struct {
	B            *tele.Bot
	q            *sqlc.Queries
	receipt      *service.ReceiptService
	bridge       *service.ReceiptBridge
	journalGen   *service.JournalGenerator
	classifier   *service.ClassifierService
	chat         *service.ChatService
	corrections  *service.CorrectionService
	vendorMemory *service.VendorMemoryService
	docQuality   *service.DocumentQualityService
	approvals    *service.ApprovalService
	tags         *service.TagService
	uploadDir    string
	baseURL      string // public base URL for receipt image links
	pendingEdits sync.Map // map[int64]uuid.UUID — telegram user ID → batch ID awaiting edit
	pendingForex sync.Map // map[int64]*ForexPending — telegram user ID → forex exchange state
	receiptNotes sync.Map // map[uuid.UUID]string — batch ID → user note

	// Auto-learning: store original OCR results before user edits.
	originalResults sync.Map // map[uuid.UUID]service.ReceiptResult — batch ID → pre-edit results

	// Smart instructions: store text instructions for upcoming photo receipt.
	pendingInstructions sync.Map // map[int64]string — telegram user ID → instruction text

	// Custom category: store pending state when user selects "Other" category.
	pendingCustomCategory sync.Map // map[int64]*CustomCategoryPending — telegram user ID → pending state

	// Custom amount: store pending state when user clicks "Other amount" in amount picker.
	pendingCustomAmount sync.Map // map[int64]*CustomAmountPending — telegram user ID → pending state

	// Field-specific edit: store pending state when user clicked a field button in edit mode.
	pendingFieldEdit sync.Map // map[int64]*PendingFieldEdit — telegram user ID → pending field edit

	// Reply-to correction: maps "chatID:msgID" → ReplyTxnData for reply-based editing.
	replyTxnMap sync.Map // map[string]*ReplyTxnData
}

// New creates and configures a new Telegram Bot.
// baseURL is the public URL prefix for receipt image links (e.g. https://tax.clawpapa.win).
func New(token string, q *sqlc.Queries, receipt *service.ReceiptService, bridge *service.ReceiptBridge, journalGen *service.JournalGenerator, classifier *service.ClassifierService, chat *service.ChatService, corrections *service.CorrectionService, vendorMemory *service.VendorMemoryService, docQuality *service.DocumentQualityService, approvals *service.ApprovalService, tags *service.TagService, uploadDir string, baseURL string) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		B:            b,
		q:            q,
		receipt:      receipt,
		bridge:       bridge,
		journalGen:   journalGen,
		classifier:   classifier,
		chat:         chat,
		corrections:  corrections,
		vendorMemory: vendorMemory,
		docQuality:   docQuality,
		approvals:    approvals,
		tags:         tags,
		uploadDir:    uploadDir,
		baseURL:      baseURL,
	}

	bot.registerHandlers()
	bot.registerMiddleware()
	return bot, nil
}

// registerMiddleware installs global middleware.
// In group chats, the bot only responds to messages that @ mention it.
func (b *Bot) registerMiddleware() {
	b.B.Use(func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			// Always process callback queries (inline button presses).
			if c.Callback() != nil {
				return next(c)
			}
			// Always process private chats.
			if c.Chat() != nil {
				chatType := c.Chat().Type
				if chatType == tele.ChatGroup || chatType == tele.ChatSuperGroup {
					// Group chat: require @mention.
					mention := "@" + b.B.Me.Username
					text := c.Text()
					if text == "" && c.Message() != nil {
						text = c.Message().Caption
					}
					if !strings.Contains(text, mention) {
						return nil // silently ignore
					}
				}
			}
			return next(c)
		}
	})
}

func (b *Bot) registerHandlers() {
	b.B.Handle("/start", b.handleStart)
	b.B.Handle("/link", b.handleLink)
	b.B.Handle("/status", b.handleStatus)
	b.B.Handle("/export", b.handleExport)
	b.B.Handle("/audit", b.handleAudit)
	b.B.Handle("/exchange", b.handleExchange)
	b.B.Handle(tele.OnPhoto, b.handlePhoto)
	b.B.Handle(tele.OnDocument, b.handleDocument)
	b.B.Handle(tele.OnText, b.handleText)

	// Receipt confirmation callback handlers.
	b.B.Handle(&btnConfirm, b.handleReceiptConfirm)
	b.B.Handle(&btnEdit, b.handleReceiptEdit)
	b.B.Handle(&btnCancel, b.handleReceiptCancel)
	b.B.Handle(&btnProject, b.handleProjectSelect)
	b.B.Handle(&btnAmountSelect, b.handleAmountSelect)
	b.B.Handle(&btnAmountCustom, b.handleAmountCustom)
	b.B.Handle(&btnCategory, b.handleCategorySelect)
	b.B.Handle(&btnEditField, b.handleEditFieldSelect)
	b.B.Handle(&btnEditBack, b.handleEditBack)

	// Approval callback handlers.
	b.B.Handle(&btnApprove, b.handleApproveReceipt)
	b.B.Handle(&btnReject, b.handleRejectReceipt)

	// Forex exchange callback handlers.
	b.B.Handle(&btnFxConfirm, b.handleForexConfirm)
	b.B.Handle(&btnFxEdit, b.handleForexEdit)
	b.B.Handle(&btnFxCancel, b.handleForexCancel)
	b.B.Handle(&btnFxProject, b.handleForexProjectSelect)

	// Learning rule suggestion callback handlers.
	b.B.Handle(&btnLearnAccept, b.handleLearnAccept)
	b.B.Handle(&btnLearnIgnore, b.handleLearnIgnore)
}

// Start begins polling for updates (blocks until Stop is called).
func (b *Bot) Start() {
	// Cancel stale pending batches from previous runs.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := b.q.CancelStalePendingBatches(ctx); err != nil {
		slog.Warn("failed to cancel stale pending batches", "error", err)
	}
	cancel()

	// Register bot commands so users see hints when typing "/".
	commands := []tele.Command{
		{Text: "start", Description: "Get started / show help"},
		{Text: "link", Description: "Link account: /link <api_key>"},
		{Text: "status", Description: "View monthly stats & breakdown"},
		{Text: "export", Description: "Export bookkeeping: /export [YYYY-MM]"},
		{Text: "audit", Description: "Check duplicate expenses"},
		{Text: "exchange", Description: "Record a P2P forex exchange"},
	}
	if err := b.B.SetCommands(commands); err != nil {
		slog.Warn("failed to set bot commands", "error", err)
	}

	slog.Info("telegram bot starting")
	b.B.Start()
}

// Stop gracefully stops the bot.
func (b *Bot) Stop() {
	slog.Info("telegram bot stopping")
	b.B.Stop()
}

// getProjectNames fetches project tag names from the database for a company.
func (b *Bot) getProjectNames(ctx context.Context, companyID uuid.UUID) []string {
	if b.tags == nil {
		return nil
	}
	names, err := b.tags.ListProjectTags(ctx, companyID)
	if err != nil {
		slog.Warn("failed to get project tags", "company_id", companyID, "error", err)
		return nil
	}
	return names
}
