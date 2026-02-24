package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/internal/service"
)

func (s *Server) handleBankReconcile(ctx context.Context, t *asynq.Task) error {
	var p BankReconcilePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal bank reconcile payload: %w", err)
	}

	slog.Info("bank reconcile task started",
		"task_id", p.TaskID,
		"company_id", p.CompanyID,
		"records", len(p.Records),
	)

	_ = s.q.UpdateAsyncTaskStatus(ctx, sqlc.UpdateAsyncTaskStatusParams{
		ID: p.TaskID, Status: "processing",
	})

	_ = s.q.UpdateAsyncTaskProgress(ctx, sqlc.UpdateAsyncTaskProgressParams{
		ID: p.TaskID, Progress: 10,
	})

	// Convert string maps to interface maps for the service.
	records := toInterfaceMaps(p.Records)
	bankRows := toInterfaceMaps(p.BankRows)

	input := service.CreateBatchInput{
		CompanyID:         p.CompanyID,
		CreatedBy:         p.UserID,
		Period:            p.Period,
		AmountTolerance:   p.AmountTolerance,
		DateToleranceDays: p.DateTolerance,
		Records:           records,
		BankColumns:       p.BankColumns,
		BankRows:          bankRows,
	}

	batch, err := s.svc.BankRecon.RunReconciliation(ctx, input)
	if err != nil {
		return s.failTask(ctx, p.TaskID, fmt.Errorf("run reconciliation: %w", err))
	}

	result, _ := json.Marshal(map[string]interface{}{
		"batch_id": batch.BatchID.String(),
		"status":   batch.Status,
	})

	if err := s.q.UpdateAsyncTaskResult(ctx, sqlc.UpdateAsyncTaskResultParams{
		ID: p.TaskID, Result: result,
	}); err != nil {
		return fmt.Errorf("update task result: %w", err)
	}

	slog.Info("bank reconcile task completed", "task_id", p.TaskID, "batch_id", batch.BatchID)
	return nil
}

func toInterfaceMaps(in []map[string]string) []map[string]interface{} {
	out := make([]map[string]interface{}, len(in))
	for i, m := range in {
		o := make(map[string]interface{}, len(m))
		for k, v := range m {
			o[k] = v
		}
		out[i] = o
	}
	return out
}
