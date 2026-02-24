package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/tonypk/aistarlight-go/internal/config"
	"github.com/tonypk/aistarlight-go/internal/handler"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	ocrclient "github.com/tonypk/aistarlight-go/internal/platform/ocr"
	oai "github.com/tonypk/aistarlight-go/internal/platform/openai"
	pg "github.com/tonypk/aistarlight-go/internal/platform/postgres"
	rd "github.com/tonypk/aistarlight-go/internal/platform/redis"
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
	BankRecon   *service.BankReconService
	Compliance  *service.ComplianceService
	Corrections *service.CorrectionService
	Analyzer    *service.CorrectionAnalyzer
	Withholding *service.WithholdingService
	Dashboard   *service.DashboardService
	Receipt     *service.ReceiptService
	Audit       *service.AuditService
	Memory      *service.MemoryService
	Task        *service.TaskService
}

func newServices(q *sqlc.Queries, cfg *config.Config, ai *oai.Client) services {
	knowledge := service.NewKnowledgeService(ai, q)
	matchAnalyzer := service.NewMatchAnalyzer(ai)
	return services{
		Auth:        service.NewAuthService(q, cfg.JWT),
		Org:         service.NewOrgService(q),
		Company:     service.NewCompanyService(q),
		Report:      service.NewReportService(q),
		Chat:        service.NewChatService(ai, q, knowledge),
		Classifier:  service.NewClassifierService(ai, q),
		ColMapper:   service.NewColumnMapperService(ai),
		Knowledge:   knowledge,
		Augmenter:   service.NewPromptAugmenter(q),
		BankRecon:   service.NewBankReconService(q, matchAnalyzer),
		Compliance:  service.NewComplianceService(q, knowledge),
		Corrections: service.NewCorrectionService(q),
		Analyzer:    service.NewCorrectionAnalyzer(q),
		Withholding: service.NewWithholdingService(q),
		Dashboard:   service.NewDashboardService(q),
		Receipt:     service.NewReceiptService(q, ocrclient.NewClient(cfg.OCR.ServiceURL)),
		Audit:       service.NewAuditService(q),
		Memory:      service.NewMemoryService(q),
		Task:        service.NewTaskService(q, cfg.Redis.URL),
	}
}

type handlers struct {
	Auth           *handler.AuthHandler
	Org            *handler.OrgHandler
	Company        *handler.CompanyHandler
	Report         *handler.ReportHandler
	Chat           *handler.ChatHandler
	Reconciliation *handler.ReconciliationHandler
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
}

func newHandlers(svc services, cfg *config.Config) handlers {
	return handlers{
		Auth:           handler.NewAuthHandler(svc.Auth, svc.Company),
		Org:            handler.NewOrgHandler(svc.Org),
		Company:        handler.NewCompanyHandler(svc.Company),
		Report:         handler.NewReportHandler(svc.Report, svc.Company),
		Chat:           handler.NewChatHandler(svc.Chat),
		Reconciliation: handler.NewReconciliationHandler(svc.BankRecon),
		Compliance:     handler.NewComplianceHandler(svc.Compliance),
		Correction:     handler.NewCorrectionHandler(svc.Corrections, svc.Analyzer),
		Withholding:    handler.NewWithholdingHandler(svc.Withholding),
		Dashboard:      handler.NewDashboardHandler(svc.Dashboard),
		Receipt:        handler.NewReceiptHandler(svc.Receipt, cfg),
		Audit:          handler.NewAuditHandler(svc.Audit),
		Memory:         handler.NewMemoryHandler(svc.Memory),
		Task:           handler.NewTaskHandler(svc.Task),
		Data:           handler.NewDataHandler(svc.ColMapper, cfg),
		Form:           handler.NewFormHandler(svc.Report),
		Knowledge:      handler.NewKnowledgeHandler(svc.Knowledge),
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
		AuthSvc:        svc.Auth,
		OrgSvc:         svc.Org,
		CompanySvc:     svc.Company,
		Config:         cfg,
		Redis:          rdb,
	}
	rt.Setup(r)

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
