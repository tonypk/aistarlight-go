package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

func (s *Server) handleOCRProcess(ctx context.Context, t *asynq.Task) error {
	var p OCRPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal ocr payload: %w", err)
	}

	slog.Info("ocr task started", "task_id", p.TaskID, "batch_id", p.BatchID, "images", len(p.ImagePaths))

	_ = s.q.UpdateAsyncTaskStatus(ctx, sqlc.UpdateAsyncTaskStatusParams{
		ID: p.TaskID, Status: "processing",
	})

	// Process images using receipt service.
	batch, results, err := s.svc.Receipt.ProcessBatch(
		ctx, p.CompanyID, p.UserID,
		p.ImagePaths, p.Period, p.ReportType,
	)
	if err != nil {
		return s.failTask(ctx, p.TaskID, fmt.Errorf("process batch: %w", err))
	}

	// Store result.
	result, _ := json.Marshal(map[string]interface{}{
		"batch_id": batch.ID.String(),
		"total":    len(results),
	})

	if err := s.q.UpdateAsyncTaskResult(ctx, sqlc.UpdateAsyncTaskResultParams{
		ID: p.TaskID, Result: result,
	}); err != nil {
		return fmt.Errorf("update task result: %w", err)
	}

	slog.Info("ocr task completed", "task_id", p.TaskID, "processed", len(results))
	return nil
}
