package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

func (s *Server) handlePDFGenerate(ctx context.Context, t *asynq.Task) error {
	var p PDFPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal pdf payload: %w", err)
	}

	slog.Info("pdf task started", "task_id", p.TaskID, "report_id", p.ReportID)

	_ = s.q.UpdateAsyncTaskStatus(ctx, sqlc.UpdateAsyncTaskStatusParams{
		ID: p.TaskID, Status: "processing",
	})

	_ = s.q.UpdateAsyncTaskProgress(ctx, sqlc.UpdateAsyncTaskProgressParams{
		ID: p.TaskID, Progress: 10,
	})

	// Load report data.
	report, err := s.q.GetReportByID(ctx, p.ReportID)
	if err != nil {
		return s.failTask(ctx, p.TaskID, fmt.Errorf("get report: %w", err))
	}

	_ = s.q.UpdateAsyncTaskProgress(ctx, sqlc.UpdateAsyncTaskProgressParams{
		ID: p.TaskID, Progress: 30,
	})

	// TODO: Actual PDF generation with gofpdf once report_generator.go is implemented.
	// For now, just record the report metadata as result.
	result, _ := json.Marshal(map[string]interface{}{
		"report_id":   p.ReportID.String(),
		"report_type": report.ReportType,
		"period":      report.Period,
		"status":      "pdf_generation_pending",
		"message":     "PDF generator not yet implemented",
	})

	if err := s.q.UpdateAsyncTaskResult(ctx, sqlc.UpdateAsyncTaskResultParams{
		ID: p.TaskID, Result: result,
	}); err != nil {
		return fmt.Errorf("update task result: %w", err)
	}

	slog.Info("pdf task completed", "task_id", p.TaskID)
	return nil
}
