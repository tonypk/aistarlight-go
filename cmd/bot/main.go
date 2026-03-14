package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tonypk/aistarlight-go/internal/bot"
	"github.com/tonypk/aistarlight-go/internal/config"
	ocrclient "github.com/tonypk/aistarlight-go/internal/platform/ocr"
	oai "github.com/tonypk/aistarlight-go/internal/platform/openai"
	pg "github.com/tonypk/aistarlight-go/internal/platform/postgres"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Logger setup.
	var h slog.Handler
	opts := &slog.HandlerOptions{}
	switch cfg.Log.Level {
	case "debug":
		opts.Level = slog.LevelDebug
	case "warn":
		opts.Level = slog.LevelWarn
	default:
		opts.Level = slog.LevelInfo
	}
	if cfg.Log.Format == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(h))

	if cfg.Telegram.BotToken == "" {
		slog.Error("TELEGRAM_BOT_TOKEN is required")
		os.Exit(1)
	}

	// Database pool.
	pool, err := pg.NewPool(context.Background(), cfg.Database)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	q := sqlc.New(pool)

	// Services.
	aiClient := oai.New(cfg.OpenAI)
	ocrClient := ocrclient.NewClient(cfg.OCR.ServiceURL)
	vendorSvc := service.NewVendorService(q)
	receiptSvc := service.NewReceiptService(q, ocrClient, vendorSvc, aiClient)
	vendorMemorySvc := service.NewVendorMemoryService(q)
	classifier := service.NewClassifierService(aiClient, q)
	classifier.SetVendorMemory(vendorMemorySvc)
	bridgeSvc := service.NewReceiptBridge(q, classifier)

	journalSvc := service.NewJournalService(q, pool, nil)
	journalGen := service.NewJournalGenerator(q, journalSvc)
	knowledgeSvc := service.NewKnowledgeService(aiClient, q)
	chatSvc := service.NewChatService(aiClient, q, knowledgeSvc)
	correctionSvc := service.NewCorrectionService(q)
	docQualitySvc := service.NewDocumentQualityService(q)
	approvalSvc := service.NewApprovalService(q)

	// Bot.
	b, err := bot.New(cfg.Telegram.BotToken, q, receiptSvc, bridgeSvc, journalGen, classifier, chatSvc, correctionSvc, vendorMemorySvc, docQualitySvc, approvalSvc, cfg.UploadDir, cfg.Telegram.Projects, cfg.Telegram.BaseURL)
	if err != nil {
		slog.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	// Graceful shutdown.
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		b.Stop()
	}()

	slog.Info("bot starting", "upload_dir", cfg.UploadDir)
	b.Start() // blocks
}
