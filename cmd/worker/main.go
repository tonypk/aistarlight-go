package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/tonypk/aistarlight-go/internal/config"
	"github.com/tonypk/aistarlight-go/internal/event"
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
	vendorSvc := service.NewVendorService(q)
	complianceSvc := service.NewComplianceService(q, knowledge)

	// Redis + event publisher (worker also publishes events on re-entrant flows)
	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.Redis.AsynqAddr(),
		DB:       cfg.Redis.AsynqDB(),
		Password: cfg.Redis.AsynqPassword(),
	}
	asynqClient := asynq.NewClient(redisOpt)
	publisher := event.NewPublisher(asynqClient)

	fsSvc := service.NewFinancialStatementService(q)
	vendorMemorySvc := service.NewVendorMemoryService(q)

	svc := &worker.Services{
		Report:       service.NewReportService(q, complianceSvc),
		Receipt:      service.NewReceiptService(q, ocrclient.NewClient(cfg.OCR.ServiceURL), vendorSvc, ai),
		Classifier: func() *service.ClassifierService {
			c := service.NewClassifierService(ai, q)
			c.SetVendorMemory(vendorMemorySvc)
			return c
		}(),
		BankRecon:    service.NewBankReconService(q, matchAnalyzer, publisher, vendorMemorySvc),
		Compliance:   complianceSvc,
		Notification: service.NewNotificationService(q),
		Journal:      service.NewJournalService(q, pool, publisher),
		GLTaxBridge:  service.NewGLTaxBridge(q, fsSvc),
	}
	srv := worker.NewServer(redisOpt, q, svc, cfg.UploadDir)

	// HR Integration inbox processor (polls integration_event_inbox).
	glMappingSvc := service.NewGLMappingService(q)
	taxBridgeLogger := slog.Default().With("component", "tax-bridge")
	payrollTaxBridge := service.NewPayrollTaxBridge(q, taxBridgeLogger)
	integrationLogger := slog.Default().With("component", "hr-integration")
	hrIntegrationSvc := service.NewHRIntegrationService(q, glMappingSvc, svc.Journal, payrollTaxBridge, integrationLogger)
	inboxProcessor := service.NewHRInboxProcessor(q, hrIntegrationSvc, slog.Default().With("component", "inbox-processor"))

	// Register periodic tasks.
	scheduler := asynq.NewScheduler(redisOpt, nil)

	cleanupTask, _ := worker.NewCleanupTask()
	if _, err := scheduler.Register("@daily", cleanupTask); err != nil {
		slog.Error("failed to register cleanup task", "error", err)
	}

	deadlineTask, _ := worker.NewDeadlineCheckTask()
	if _, err := scheduler.Register("0 8 * * *", deadlineTask); err != nil { // 8 AM daily
		slog.Error("failed to register deadline check task", "error", err)
	}

	// Start scheduler in background.
	go func() {
		if err := scheduler.Run(); err != nil {
			slog.Error("scheduler error", "error", err)
		}
	}()

	// Start HR inbox processor in background.
	inboxCtx, inboxCancel := context.WithCancel(context.Background())
	go inboxProcessor.Run(inboxCtx)

	// Graceful shutdown.
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		slog.Info("shutting down worker...")
		inboxCancel()
		srv.Shutdown()
		scheduler.Shutdown()
	}()

	slog.Info("worker starting", "redis", cfg.Redis.AsynqAddr())
	if err := srv.Run(); err != nil {
		slog.Error("worker error", "error", err)
		os.Exit(1)
	}
}
