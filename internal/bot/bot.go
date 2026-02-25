package bot

import (
	"log/slog"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// Bot wraps the Telegram bot and its dependencies.
type Bot struct {
	B         *tele.Bot
	q         *sqlc.Queries
	receipt   *service.ReceiptService
	bridge    *service.ReceiptBridge
	uploadDir string
}

// New creates and configures a new Telegram Bot.
func New(token string, q *sqlc.Queries, receipt *service.ReceiptService, bridge *service.ReceiptBridge, uploadDir string) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		B:         b,
		q:         q,
		receipt:   receipt,
		bridge:    bridge,
		uploadDir: uploadDir,
	}

	bot.registerHandlers()
	return bot, nil
}

func (b *Bot) registerHandlers() {
	b.B.Handle("/start", b.handleStart)
	b.B.Handle("/link", b.handleLink)
	b.B.Handle("/status", b.handleStatus)
	b.B.Handle(tele.OnPhoto, b.handlePhoto)
	b.B.Handle(tele.OnDocument, b.handleDocument)
}

// Start begins polling for updates (blocks until Stop is called).
func (b *Bot) Start() {
	slog.Info("telegram bot starting")
	b.B.Start()
}

// Stop gracefully stops the bot.
func (b *Bot) Stop() {
	slog.Info("telegram bot stopping")
	b.B.Stop()
}
