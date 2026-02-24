package worker

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"
)

func (s *Server) handleCleanup(ctx context.Context, _ *asynq.Task) error {
	slog.Info("cleanup task started")

	deleted, err := s.q.CleanupOldTasks(ctx)
	if err != nil {
		slog.Error("cleanup old tasks failed", "error", err)
		return err
	}

	slog.Info("cleanup task completed", "deleted_tasks", deleted)
	return nil
}
