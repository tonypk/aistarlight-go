package bot

import (
	"context"
	"log/slog"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// Bot wraps the Telegram bot and its dependencies.
type Bot struct {
	B            *tele.Bot
	q            *sqlc.Queries
	receipt      *service.ReceiptService
	bridge       *service.ReceiptBridge
	journalGen   *service.JournalGenerator
	classifier   *service.ClassifierService
	chat         *service.ChatService
	uploadDir    string
	projects     []string // configurable project tags (from BOT_PROJECTS env)
	pendingEdits sync.Map // map[int64]uuid.UUID — telegram user ID → batch ID awaiting edit
	pendingForex sync.Map // map[int64]*ForexPending — telegram user ID → forex exchange state
	pendingNotes sync.Map // map[int64]*ReceiptPendingNote — telegram user ID → awaiting note input
	receiptNotes sync.Map // map[uuid.UUID]string — batch ID → user note
}

// New creates and configures a new Telegram Bot.
// projects is an optional list of project tags for receipt classification.
func New(token string, q *sqlc.Queries, receipt *service.ReceiptService, bridge *service.ReceiptBridge, journalGen *service.JournalGenerator, classifier *service.ClassifierService, chat *service.ChatService, uploadDir string, projects []string) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		B:          b,
		q:          q,
		receipt:    receipt,
		bridge:     bridge,
		journalGen: journalGen,
		classifier: classifier,
		chat:       chat,
		uploadDir:  uploadDir,
		projects:   projects,
	}

	bot.registerHandlers()
	return bot, nil
}

func (b *Bot) registerHandlers() {
	b.B.Handle("/start", b.handleStart)
	b.B.Handle("/link", b.handleLink)
	b.B.Handle("/status", b.handleStatus)
	b.B.Handle("/export", b.handleExport)
	b.B.Handle("/exchange", b.handleExchange)
	b.B.Handle(tele.OnPhoto, b.handlePhoto)
	b.B.Handle(tele.OnDocument, b.handleDocument)
	b.B.Handle(tele.OnText, b.handleText)

	// Receipt confirmation callback handlers.
	b.B.Handle(&btnConfirm, b.handleReceiptConfirm)
	b.B.Handle(&btnEdit, b.handleReceiptEdit)
	b.B.Handle(&btnCancel, b.handleReceiptCancel)
	b.B.Handle(&btnProject, b.handleProjectSelect)

	// Forex exchange callback handlers.
	b.B.Handle(&btnFxConfirm, b.handleForexConfirm)
	b.B.Handle(&btnFxEdit, b.handleForexEdit)
	b.B.Handle(&btnFxCancel, b.handleForexCancel)
	b.B.Handle(&btnFxProject, b.handleForexProjectSelect)
}

// Start begins polling for updates (blocks until Stop is called).
func (b *Bot) Start() {
	// Cancel stale pending batches from previous runs.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := b.q.CancelStalePendingBatches(ctx); err != nil {
		slog.Warn("failed to cancel stale pending batches", "error", err)
	}
	cancel()

	slog.Info("telegram bot starting")
	b.B.Start()
}

// Stop gracefully stops the bot.
func (b *Bot) Stop() {
	slog.Info("telegram bot stopping")
	b.B.Stop()
}
