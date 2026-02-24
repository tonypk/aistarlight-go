package worker

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// failTask marks a task as failed and returns the original error.
func (s *Server) failTask(ctx context.Context, taskID uuid.UUID, err error) error {
	msg := err.Error()
	_ = s.q.UpdateAsyncTaskError(ctx, sqlc.UpdateAsyncTaskErrorParams{
		ID:           taskID,
		ErrorMessage: &msg,
	})
	return fmt.Errorf("task %s failed: %w", taskID, err)
}
