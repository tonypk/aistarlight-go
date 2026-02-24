package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

func (s *Server) handleComplianceCheck(ctx context.Context, t *asynq.Task) error {
	var p CompliancePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal compliance payload: %w", err)
	}

	slog.Info("compliance task started", "task_id", p.TaskID, "report_id", p.ReportID)

	_ = s.q.UpdateAsyncTaskStatus(ctx, sqlc.UpdateAsyncTaskStatusParams{
		ID: p.TaskID, Status: "processing",
	})

	_ = s.q.UpdateAsyncTaskProgress(ctx, sqlc.UpdateAsyncTaskProgressParams{
		ID: p.TaskID, Progress: 20,
	})

	validation, err := s.svc.Compliance.ValidateReport(ctx, p.CompanyID, p.ReportID)
	if err != nil {
		return s.failTask(ctx, p.TaskID, fmt.Errorf("validate report: %w", err))
	}

	result, _ := json.Marshal(validation)

	if err := s.q.UpdateAsyncTaskResult(ctx, sqlc.UpdateAsyncTaskResultParams{
		ID: p.TaskID, Result: result,
	}); err != nil {
		return fmt.Errorf("update task result: %w", err)
	}

	slog.Info("compliance task completed", "task_id", p.TaskID, "score", validation.OverallScore)
	return nil
}
