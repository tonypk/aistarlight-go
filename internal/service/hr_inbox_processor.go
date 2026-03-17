package service

import (
	"context"
	"log/slog"
	"time"

	sqlc "github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// HRInboxProcessor polls the integration_event_inbox and processes events.
type HRInboxProcessor struct {
	q              *sqlc.Queries
	integrationSvc *HRIntegrationService
	logger         *slog.Logger
	batchSize      int32
	interval       time.Duration
}

// NewHRInboxProcessor creates a new processor.
func NewHRInboxProcessor(q *sqlc.Queries, integrationSvc *HRIntegrationService, logger *slog.Logger) *HRInboxProcessor {
	return &HRInboxProcessor{
		q:              q,
		integrationSvc: integrationSvc,
		logger:         logger,
		batchSize:      20,
		interval:       5 * time.Second,
	}
}

// Run starts the processing loop. Blocks until ctx is cancelled.
func (p *HRInboxProcessor) Run(ctx context.Context) {
	p.logger.Info("HR inbox processor started")
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("HR inbox processor stopped")
			return
		case <-ticker.C:
			p.processBatch(ctx)
		}
	}
}

func (p *HRInboxProcessor) processBatch(ctx context.Context) {
	events, err := p.q.ListPendingInboxEvents(ctx, p.batchSize)
	if err != nil {
		p.logger.Error("failed to list pending inbox events", "error", err)
		return
	}

	for _, evt := range events {
		if err := p.integrationSvc.ProcessEvent(ctx, evt); err != nil {
			p.logger.Error("event processing failed",
				"inbox_id", evt.ID,
				"event_type", evt.EventType,
				"event_id", evt.EventID,
				"error", err,
			)
			errMsg := err.Error()
			_ = p.q.MarkInboxFailed(ctx, sqlc.MarkInboxFailedParams{
				ID:           evt.ID,
				ErrorMessage: &errMsg,
			})
		} else {
			_ = p.q.MarkInboxProcessed(ctx, evt.ID)
			_ = p.q.UpdateIntegrationSourceLastEvent(ctx, evt.ID)
			p.logger.Info("event processed",
				"inbox_id", evt.ID,
				"event_type", evt.EventType,
			)
		}
	}
}
