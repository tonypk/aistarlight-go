package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

func (s *Server) handleAIClassify(ctx context.Context, t *asynq.Task) error {
	var p AIClassifyPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal ai classify payload: %w", err)
	}

	slog.Info("ai classify task started",
		"task_id", p.TaskID,
		"session_id", p.SessionID,
		"transactions", len(p.TransactionIDs),
	)

	_ = s.q.UpdateAsyncTaskStatus(ctx, sqlc.UpdateAsyncTaskStatusParams{
		ID: p.TaskID, Status: "processing",
	})

	_ = s.q.UpdateAsyncTaskProgress(ctx, sqlc.UpdateAsyncTaskProgressParams{
		ID: p.TaskID, Progress: 10,
	})

	// Load transactions from DB.
	var transactions []map[string]interface{}
	for _, txnID := range p.TransactionIDs {
		txn, err := s.q.GetTransactionByID(ctx, txnID)
		if err != nil {
			slog.Warn("get transaction failed", "txn_id", txnID, "error", err)
			continue
		}
		tx := map[string]interface{}{
			"id": txn.ID.String(),
		}
		if txn.Description != nil {
			tx["description"] = *txn.Description
		}
		if txn.Tin != nil {
			tx["tin"] = *txn.Tin
		}
		transactions = append(transactions, tx)
	}

	_ = s.q.UpdateAsyncTaskProgress(ctx, sqlc.UpdateAsyncTaskProgressParams{
		ID: p.TaskID, Progress: 30,
	})

	// Run batch classification.
	results, err := s.svc.Classifier.ClassifyTransactions(ctx, transactions, p.CompanyID, "", "")
	if err != nil {
		return s.failTask(ctx, p.TaskID, fmt.Errorf("classify transactions: %w", err))
	}

	_ = s.q.UpdateAsyncTaskProgress(ctx, sqlc.UpdateAsyncTaskProgressParams{
		ID: p.TaskID, Progress: 80,
	})

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"total":      len(p.TransactionIDs),
		"classified": len(results),
	})

	if err := s.q.UpdateAsyncTaskResult(ctx, sqlc.UpdateAsyncTaskResultParams{
		ID: p.TaskID, Result: resultJSON,
	}); err != nil {
		return fmt.Errorf("update task result: %w", err)
	}

	slog.Info("ai classify task completed", "task_id", p.TaskID, "classified", len(results))
	return nil
}
