package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

// TaskService manages async task creation and querying.
type TaskService struct {
	q      *sqlc.Queries
	client *asynq.Client
}

// NewTaskService creates a task service with asynq client.
func NewTaskService(q *sqlc.Queries, redisOpt asynq.RedisClientOpt) *TaskService {
	var client *asynq.Client
	if redisOpt.Addr != "" {
		client = asynq.NewClient(redisOpt)
	}
	return &TaskService{q: q, client: client}
}

// TaskOutput is the API response for a task.
type TaskOutput struct {
	ID           uuid.UUID              `json:"id"`
	CompanyID    uuid.UUID              `json:"company_id"`
	TaskType     string                 `json:"task_type"`
	Status       string                 `json:"status"`
	Progress     int                    `json:"progress"`
	Result       map[string]interface{} `json:"result,omitempty"`
	ErrorMessage *string                `json:"error_message,omitempty"`
	CreatedAt    string                 `json:"created_at"`
	StartedAt    *string                `json:"started_at,omitempty"`
	CompletedAt  *string                `json:"completed_at,omitempty"`
}

// EnqueueTask creates a DB record and enqueues an asynq task.
func (s *TaskService) EnqueueTask(
	ctx context.Context,
	companyID, userID uuid.UUID,
	taskType string,
	payload interface{},
	asynqTask *asynq.Task,
) (*TaskOutput, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	row, err := s.q.CreateAsyncTask(ctx, sqlc.CreateAsyncTaskParams{
		CompanyID: companyID,
		CreatedBy: userID,
		TaskType:  taskType,
		Payload:   payloadJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("create task record: %w", err)
	}

	if s.client != nil && asynqTask != nil {
		if _, err := s.client.EnqueueContext(ctx, asynqTask); err != nil {
			slog.Error("enqueue task failed", "task_id", row.ID, "type", taskType, "error", err)
			// Mark as failed in DB if enqueue fails.
			msg := err.Error()
			_ = s.q.UpdateAsyncTaskError(ctx, sqlc.UpdateAsyncTaskErrorParams{
				ID:           row.ID,
				ErrorMessage: &msg,
			})
			return nil, fmt.Errorf("enqueue task: %w", err)
		}
	}

	return toTaskOutput(row), nil
}

// GetTask retrieves a task by ID.
func (s *TaskService) GetTask(ctx context.Context, id uuid.UUID) (*TaskOutput, error) {
	row, err := s.q.GetAsyncTask(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}
	return toTaskOutput(row), nil
}

// ListTasks lists tasks for a company with pagination.
func (s *TaskService) ListTasks(ctx context.Context, companyID uuid.UUID, limit, offset int32) ([]TaskOutput, int64, error) {
	rows, err := s.q.ListAsyncTasksByCompany(ctx, sqlc.ListAsyncTasksByCompanyParams{
		CompanyID: companyID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, 0, err
	}

	total, err := s.q.CountAsyncTasksByCompany(ctx, companyID)
	if err != nil {
		return nil, 0, err
	}

	out := make([]TaskOutput, len(rows))
	for i, r := range rows {
		out[i] = *toTaskOutput(r)
	}
	return out, total, nil
}

// Close closes the asynq client.
func (s *TaskService) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

func toTaskOutput(r sqlc.AsyncTask) *TaskOutput {
	out := &TaskOutput{
		ID:           r.ID,
		CompanyID:    r.CompanyID,
		TaskType:     r.TaskType,
		Status:       r.Status,
		Progress:     int(r.Progress),
		ErrorMessage: r.ErrorMessage,
		CreatedAt:    r.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if len(r.Result) > 0 {
		var result map[string]interface{}
		if json.Unmarshal(r.Result, &result) == nil {
			out.Result = result
		}
	}

	if r.StartedAt.Valid {
		t := r.StartedAt.Time.Format("2006-01-02T15:04:05Z")
		out.StartedAt = &t
	}

	if r.CompletedAt.Valid {
		t := r.CompletedAt.Time.Format("2006-01-02T15:04:05Z")
		out.CompletedAt = &t
	}

	return out
}
