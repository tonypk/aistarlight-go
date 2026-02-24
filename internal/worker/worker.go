package worker

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
)

// Server wraps asynq.Server with task handler dependencies.
type Server struct {
	srv       *asynq.Server
	mux       *asynq.ServeMux
	q         *sqlc.Queries
	svc       *Services
	uploadDir string
}

// Services holds all service dependencies for task handlers.
type Services struct {
	Report      *service.ReportService
	Receipt     *service.ReceiptService
	Classifier  *service.ClassifierService
	BankRecon   *service.BankReconService
	Compliance  *service.ComplianceService
}

// NewServer creates an asynq worker server.
func NewServer(redisOpt asynq.RedisClientOpt, q *sqlc.Queries, svc *Services, uploadDir string) *Server {
	srv := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				QueueCritical: 6,
				QueueDefault:  3,
				QueueLow:      1,
			},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				slog.Error("task failed",
					"type", task.Type(),
					"error", err,
				)
			}),
			Logger: newAsynqLogger(),
		},
	)

	s := &Server{
		srv:       srv,
		mux:       asynq.NewServeMux(),
		q:         q,
		svc:       svc,
		uploadDir: uploadDir,
	}

	s.registerHandlers()
	return s
}

func (s *Server) registerHandlers() {
	s.mux.HandleFunc(TypePDFGenerate, s.handlePDFGenerate)
	s.mux.HandleFunc(TypeOCRProcess, s.handleOCRProcess)
	s.mux.HandleFunc(TypeAIClassify, s.handleAIClassify)
	s.mux.HandleFunc(TypeBankReconcile, s.handleBankReconcile)
	s.mux.HandleFunc(TypeComplianceCheck, s.handleComplianceCheck)
	s.mux.HandleFunc(TypeCleanupOldTasks, s.handleCleanup)
}

// Run starts the asynq server (blocks until shutdown).
func (s *Server) Run() error {
	return s.srv.Run(s.mux)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() {
	s.srv.Shutdown()
}

// asynqLogger adapts slog to asynq's Logger interface.
type asynqLogger struct{}

func newAsynqLogger() *asynqLogger { return &asynqLogger{} }

func (l *asynqLogger) Debug(args ...interface{}) {
	slog.Debug("asynq", "msg", args)
}

func (l *asynqLogger) Info(args ...interface{}) {
	slog.Info("asynq", "msg", args)
}

func (l *asynqLogger) Warn(args ...interface{}) {
	slog.Warn("asynq", "msg", args)
}

func (l *asynqLogger) Error(args ...interface{}) {
	slog.Error("asynq", "msg", args)
}

func (l *asynqLogger) Fatal(args ...interface{}) {
	slog.Error("asynq fatal", "msg", args)
}
