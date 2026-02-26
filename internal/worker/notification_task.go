package worker

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"
)

// handleDeadlineCheck processes the deadline:check task.
func (s *Server) handleDeadlineCheck(ctx context.Context, task *asynq.Task) error {
	slog.Info("running deadline check")

	if s.svc.Notification == nil {
		slog.Warn("notification service not configured, skipping deadline check")
		return nil
	}

	if err := s.svc.Notification.CheckDeadlinesAndNotify(ctx); err != nil {
		slog.Error("deadline check failed", "error", err)
		return err
	}

	slog.Info("deadline check completed")
	return nil
}
