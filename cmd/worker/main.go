package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/tonypk/aistarlight-go/internal/config"
	ocrclient "github.com/tonypk/aistarlight-go/internal/platform/ocr"
	oai "github.com/tonypk/aistarlight-go/internal/platform/openai"
	pg "github.com/tonypk/aistarlight-go/internal/platform/postgres"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
	"github.com/tonypk/aistarlight-go/internal/worker"
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

	// Database pool.
	pool, err := pg.NewPool(context.Background(), cfg.Database)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	q := sqlc.New(pool)

	// OpenAI client.
	var ai *oai.Client
	if cfg.OpenAI.APIKey != "" {
		ai = oai.New(cfg.OpenAI)
	} else {
		slog.Warn("OPENAI_API_KEY not set, AI tasks will fail")
	}

	// Build service dependencies.
	knowledge := service.NewKnowledgeService(ai, q)
	matchAnalyzer := service.NewMatchAnalyzer(ai)

	svc := &worker.Services{
		Report:     service.NewReportService(q),
		Receipt:    service.NewReceiptService(q, ocrclient.NewClient(cfg.OCR.ServiceURL)),
		Classifier: service.NewClassifierService(ai, q),
		BankRecon:  service.NewBankReconService(q, matchAnalyzer),
		Compliance: service.NewComplianceService(q, knowledge),
	}

	// Create and start worker server.
	srv := worker.NewServer(cfg.Redis.URL, q, svc, cfg.UploadDir)

	// Register periodic tasks.
	scheduler := asynq.NewScheduler(
		asynq.RedisClientOpt{Addr: cfg.Redis.URL},
		nil,
	)

	cleanupTask, _ := worker.NewCleanupTask()
	if _, err := scheduler.Register("@daily", cleanupTask); err != nil {
		slog.Error("failed to register cleanup task", "error", err)
	}

	// Start scheduler in background.
	go func() {
		if err := scheduler.Run(); err != nil {
			slog.Error("scheduler error", "error", err)
		}
	}()

	// Graceful shutdown.
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		slog.Info("shutting down worker...")
		srv.Shutdown()
		scheduler.Shutdown()
	}()

	slog.Info("worker starting", "redis", cfg.Redis.URL)
	if err := srv.Run(); err != nil {
		slog.Error("worker error", "error", err)
		os.Exit(1)
	}
}
