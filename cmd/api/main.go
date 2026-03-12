package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/tonypk/aistarlight-go/internal/config"
	"github.com/tonypk/aistarlight-go/internal/handler"
	"github.com/tonypk/aistarlight-go/internal/event"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/hibiken/asynq"
	"github.com/tonypk/aistarlight-go/internal/platform/crypto"
	ocrclient "github.com/tonypk/aistarlight-go/internal/platform/ocr"
	oai "github.com/tonypk/aistarlight-go/internal/platform/openai"
	pg "github.com/tonypk/aistarlight-go/internal/platform/postgres"
	"github.com/tonypk/aistarlight-go/internal/platform/qbo"
	rd "github.com/tonypk/aistarlight-go/internal/platform/redis"
	"github.com/tonypk/aistarlight-go/internal/agent"
	"github.com/tonypk/aistarlight-go/internal/agent/agents"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
)

func main() {
	app := fx.New(
		fx.Provide(
			config.Load,
			newLogger,
			newPostgres,
			newRedis,
			newQueries,
			newOpenAI,
			newServices,
			newHandlers,
			newGinEngine,
		),
		fx.Invoke(startServer),
	)

	app.Run()
}

func newLogger(cfg *config.Config) *slog.Logger {
	var h slog.Handler
	opts := &slog.HandlerOptions{}

	switch cfg.Log.Level {
	case "debug":
		opts.Level = slog.LevelDebug
	case "warn":
		opts.Level = slog.LevelWarn
	case "error":
		opts.Level = slog.LevelError
	default:
		opts.Level = slog.LevelInfo
	}

	if cfg.Log.Format == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(h)
	slog.SetDefault(logger)
	return logger
}

func newPostgres(lc fx.Lifecycle, cfg *config.Config) (*pgxpool.Pool, error) {
	pool, err := pg.NewPool(context.Background(), cfg.Database)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			pool.Close()
			return nil
		},
	})

	return pool, nil
}

func newRedis(lc fx.Lifecycle, cfg *config.Config) (*redis.Client, error) {
	client, err := rd.NewClient(context.Background(), cfg.Redis)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return client.Close()
		},
	})

	return client, nil
}

func newQueries(pool *pgxpool.Pool) *sqlc.Queries {
	return sqlc.New(pool)
}

func newOpenAI(cfg *config.Config) *oai.Client {
	if cfg.OpenAI.APIKey == "" {
		slog.Warn("OPENAI_API_KEY not set, AI features disabled")
		return nil
	}
	return oai.New(cfg.OpenAI)
}

type services struct {
	Auth        *service.AuthService
	Org         *service.OrgService
	Company     *service.CompanyService
	Report      *service.ReportService
	Chat        *service.ChatService
	Classifier  *service.ClassifierService
	ColMapper   *service.ColumnMapperService
	Knowledge   *service.KnowledgeService
	Augmenter   *service.PromptAugmenter
	Session     *service.SessionService
	BankRecon   *service.BankReconService
	Compliance  *service.ComplianceService
	Corrections *service.CorrectionService
	Analyzer    *service.CorrectionAnalyzer
	Supplier    *service.SupplierService
	Withholding *service.WithholdingService
	Dashboard   *service.DashboardService
	Receipt     *service.ReceiptService
	Audit       *service.AuditService
	Memory      *service.MemoryService
	Task        *service.TaskService
	Account     *service.AccountService
	Journal     *service.JournalService
	Period      *service.AccountingPeriodService
	GL          *service.GLService
	QBO         *service.QBOService
	Notification   *service.NotificationService
	TaxRule        *service.TaxRuleService
	FormSchema     *service.FormSchemaService
	RuleResolver   *service.RuleResolver
	FormRouter     *service.FormRouter
	// Pipeline bridge services
	ReceiptBridge  *service.ReceiptBridge
	JournalGen     *service.JournalGenerator
	FinStatement   *service.FinancialStatementService
	GLTaxBridge    *service.GLTaxBridge
}

func newServices(q *sqlc.Queries, cfg *config.Config, ai *oai.Client, pool *pgxpool.Pool) services {
	// Event publisher for domain events (fire-and-forget via asynq)
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.Redis.AsynqAddr(),
		DB:       cfg.Redis.AsynqDB(),
		Password: cfg.Redis.AsynqPassword(),
	})
	publisher := event.NewPublisher(asynqClient)

	knowledge := service.NewKnowledgeService(ai, q)
	matchAnalyzer := service.NewMatchAnalyzer(ai)

	accountSvc := service.NewAccountService(q)

	// QBO service (optional — only if credentials configured)
	var qboSvc *service.QBOService
	if cfg.QBO.ClientID != "" && cfg.Encryption.Key != "" {
		encryptor, err := crypto.NewAESEncryptor(cfg.Encryption.Key)
		if err != nil {
			slog.Warn("ENCRYPTION_KEY invalid, QBO disabled", "error", err)
		} else {
			oauthProvider := qbo.NewOAuthProvider(cfg.QBO)
			qboClient := qbo.NewClient(cfg.QBO.BaseURL, cfg.QBO.RateLimit, cfg.QBO.MaxConcur)
			qboSvc = service.NewQBOService(q, oauthProvider, qboClient, encryptor)
		}
	}

	classifierSvc := service.NewClassifierService(ai, q)
	supplierSvc := service.NewSupplierService(q)
	complianceSvc := service.NewComplianceService(q, knowledge)

	journalSvc := service.NewJournalService(q, pool, publisher)
	fsSvc := service.NewFinancialStatementService(q)

	return services{
		Auth:        service.NewAuthService(q, cfg.JWT),
		Org:         service.NewOrgService(q),
		Company:     service.NewCompanyService(q),
		Report:      service.NewReportService(q, complianceSvc),
		Chat:        service.NewChatService(ai, q, knowledge),
		Session:     service.NewSessionService(q, classifierSvc, ai, service.NewPromptAugmenter(q)),
		Classifier:  classifierSvc,
		ColMapper:   service.NewColumnMapperService(ai),
		Knowledge:   knowledge,
		Augmenter:   service.NewPromptAugmenter(q),
		BankRecon:   service.NewBankReconService(q, matchAnalyzer, publisher),
		Compliance:  complianceSvc,
		Corrections: service.NewCorrectionService(q),
		Analyzer:    service.NewCorrectionAnalyzer(q),
		Supplier:    supplierSvc,
		Withholding: service.NewWithholdingService(q, supplierSvc),
		Dashboard:   service.NewDashboardService(q),
		Receipt:     service.NewReceiptService(q, ocrclient.NewClient(cfg.OCR.ServiceURL), supplierSvc),
		Audit:       service.NewAuditService(q),
		Memory:      service.NewMemoryService(q),
		Task:        service.NewTaskService(q, asynq.RedisClientOpt{
			Addr:     cfg.Redis.AsynqAddr(),
			DB:       cfg.Redis.AsynqDB(),
			Password: cfg.Redis.AsynqPassword(),
		}),
		Account:     accountSvc,
		Journal:     journalSvc,
		Period:      service.NewAccountingPeriodService(q),
		GL:          service.NewGLService(q),
		QBO:         qboSvc,
		Notification: service.NewNotificationService(q),
		TaxRule:      service.NewTaxRuleService(q),
		FormSchema:   service.NewFormSchemaService(q),
		RuleResolver: service.NewRuleResolver(q),
		FormRouter:   service.NewFormRouter(),
		// Pipeline bridges
		ReceiptBridge: service.NewReceiptBridge(q, classifierSvc),
		JournalGen:    service.NewJournalGenerator(q, journalSvc),
		FinStatement:  fsSvc,
		GLTaxBridge:   service.NewGLTaxBridge(q, fsSvc),
	}
}

type handlers struct {
	Auth           *handler.AuthHandler
	Org            *handler.OrgHandler
	Company        *handler.CompanyHandler
	Report         *handler.ReportHandler
	Chat           *handler.ChatHandler
	Reconciliation *handler.ReconciliationHandler
	Session        *handler.SessionHandler
	Compliance     *handler.ComplianceHandler
	Correction     *handler.CorrectionHandler
	Withholding    *handler.WithholdingHandler
	Dashboard      *handler.DashboardHandler
	Receipt        *handler.ReceiptHandler
	Audit          *handler.AuditHandler
	Memory         *handler.MemoryHandler
	Task           *handler.TaskHandler
	Data           *handler.DataHandler
	Form           *handler.FormHandler
	Knowledge      *handler.KnowledgeHandler
	TaxRule        *handler.TaxRuleHandler
	FormRouter     *handler.FormRouterHandler
	Account        *handler.AccountHandler
	Journal        *handler.JournalHandler
	Period         *handler.AccountingPeriodHandler
	GL             *handler.GLHandler
	QBO            *handler.QBOHandler
	Settings       *handler.SettingsHandler
	Telegram       *handler.TelegramHandler
	Notification   *handler.NotificationHandler
	Health         *handler.HealthHandler
	Agent          *handler.AgentHandler
	Transaction    *handler.TransactionHandler
	// Pipeline bridge handlers
	ReceiptBridge  *handler.ReceiptBridgeHandler
	JournalBridge  *handler.JournalBridgeHandler
	FinStatement   *handler.FinancialStatementHandler
	TaxBridge      *handler.TaxBridgeHandler
}

func newAgentRuntime(ai *oai.Client, q *sqlc.Queries, chatSvc *service.ChatService) *agent.Runtime {
	registry := agent.NewRegistry()
	agents.RegisterAll(registry)
	toolExec := agent.NewToolExecutor(chatSvc)
	return agent.NewRuntime(registry, ai, q, toolExec.MakeExecuteFunc())
}

func newHandlers(svc services, cfg *config.Config, ai *oai.Client, q *sqlc.Queries) handlers {
	agentRuntime := newAgentRuntime(ai, q, svc.Chat)
	return handlers{
		Auth:           handler.NewAuthHandler(svc.Auth, svc.Company, q, cfg.Telegram.BotUsername),
		Org:            handler.NewOrgHandler(svc.Org),
		Company:        handler.NewCompanyHandler(svc.Company),
		Report:         handler.NewReportHandler(svc.Report, svc.Company),
		Chat:           handler.NewChatHandler(svc.Chat),
		Reconciliation: handler.NewReconciliationHandler(svc.BankRecon),
		Session:        handler.NewSessionHandler(svc.Session, svc.Report, ai, cfg),
		Compliance:     handler.NewComplianceHandler(svc.Compliance, svc.Report, ai, svc.RuleResolver),
		Correction:     handler.NewCorrectionHandler(svc.Corrections, svc.Analyzer),
		Withholding:    handler.NewWithholdingHandler(svc.Withholding, svc.Supplier),
		Dashboard:      handler.NewDashboardHandler(svc.Dashboard),
		Receipt:        handler.NewReceiptHandler(svc.Receipt, cfg, q),
		Audit:          handler.NewAuditHandler(svc.Audit),
		Memory:         handler.NewMemoryHandler(svc.Memory),
		Task:           handler.NewTaskHandler(svc.Task),
		Data:           handler.NewDataHandler(svc.ColMapper, svc.Memory, ai, cfg, q),
		Form:           handler.NewFormHandler(svc.FormSchema),
		TaxRule:        handler.NewTaxRuleHandler(svc.TaxRule),
		FormRouter:     handler.NewFormRouterHandler(svc.FormRouter),
		Knowledge:      handler.NewKnowledgeHandler(svc.Knowledge),
		Account:        handler.NewAccountHandler(svc.Account),
		Journal:        handler.NewJournalHandler(svc.Journal),
		Period:         handler.NewAccountingPeriodHandler(svc.Period),
		GL:             handler.NewGLHandler(svc.GL),
		QBO:            handler.NewQBOHandler(svc.QBO, svc.Account),
		Settings:       handler.NewSettingsHandler(svc.Company),
		Telegram:       handler.NewTelegramHandler(q, cfg.Telegram.BotUsername),
		Notification:   handler.NewNotificationHandler(svc.Notification),
		Health:         handler.NewHealthHandler(ai),
		Agent:          handler.NewAgentHandler(agentRuntime),
		Transaction:    handler.NewTransactionHandler(svc.Session, cfg.Telegram.BaseURL),
		// Pipeline bridges
		ReceiptBridge:  handler.NewReceiptBridgeHandler(svc.ReceiptBridge),
		JournalBridge:  handler.NewJournalBridgeHandler(svc.JournalGen),
		FinStatement:   handler.NewFinancialStatementHandler(svc.FinStatement),
		TaxBridge:      handler.NewTaxBridgeHandler(svc.GLTaxBridge, svc.Company, q),
	}
}

func newGinEngine(cfg *config.Config, rdb *redis.Client, svc services, h handlers) *gin.Engine {
	gin.SetMode(cfg.Server.Mode)

	r := gin.New()
	r.Use(
		middleware.Recovery(),
		middleware.Logger(),
		middleware.CORS(cfg.CORS.Origins),
	)

	// Health check (no auth)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Wire up all routes
	rt := &handler.Router{
		Auth:           h.Auth,
		Org:            h.Org,
		Company:        h.Company,
		Report:         h.Report,
		Chat:           h.Chat,
		Reconciliation: h.Reconciliation,
		Session:        h.Session,
		Compliance:     h.Compliance,
		Correction:     h.Correction,
		Withholding:    h.Withholding,
		Dashboard:      h.Dashboard,
		Receipt:        h.Receipt,
		Audit:          h.Audit,
		Memory:         h.Memory,
		Task:           h.Task,
		Data:           h.Data,
		Form:           h.Form,
		Knowledge:      h.Knowledge,
		TaxRule:        h.TaxRule,
		FormRouter:     h.FormRouter,
		Account:        h.Account,
		Journal:        h.Journal,
		Period:         h.Period,
		GL:             h.GL,
		QBO:            h.QBO,
		Settings:       h.Settings,
		Telegram:       h.Telegram,
		Notification:   h.Notification,
		Health:         h.Health,
		Agent:          h.Agent,
		Transaction:    h.Transaction,
		ReceiptBridge:  h.ReceiptBridge,
		JournalBridge:  h.JournalBridge,
		FinStatement:   h.FinStatement,
		TaxBridge:      h.TaxBridge,
		AuthSvc:        svc.Auth,
		OrgSvc:         svc.Org,
		CompanySvc:     svc.Company,
		Config:         cfg,
		Redis:          rdb,
	}
	rt.Setup(r)

	// JSON 404 handler for API routes (instead of Gin default plain text)
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "not found",
			})
			return
		}
		// Non-API routes: let nginx/frontend handle
		c.Status(http.StatusNotFound)
	})

	return r
}

func startServer(lc fx.Lifecycle, cfg *config.Config, r *gin.Engine) {
	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				slog.Info("server starting", "addr", srv.Addr)
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					slog.Error("server error", "error", err)
					os.Exit(1)
				}
			}()

			go func() {
				quit := make(chan os.Signal, 1)
				signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
				<-quit
				slog.Info("shutting down server...")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := srv.Shutdown(shutdownCtx); err != nil {
					slog.Error("server forced shutdown", "error", err)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})
}
