package event

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
)

// Publisher wraps an asynq.Client for fire-and-forget event publishing.
// It is nil-safe: calling Publish on a nil Publisher or with a nil client is a no-op.
type Publisher struct {
	client *asynq.Client
}

// NewPublisher creates a Publisher backed by the given asynq client.
func NewPublisher(client *asynq.Client) *Publisher {
	return &Publisher{client: client}
}

// Publish enqueues a domain event as an asynq task.
// Errors are logged but never returned — the caller's operation must not fail
// because an event could not be published.
func (p *Publisher) Publish(_ context.Context, eventType string, payload any) {
	if p == nil || p.client == nil {
		return
	}

	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("event publish: marshal failed",
			"event", eventType,
			"error", err,
		)
		return
	}

	task := asynq.NewTask(eventType, data, asynq.Queue("default"), asynq.MaxRetry(2))
	info, err := p.client.Enqueue(task)
	if err != nil {
		slog.Error("event publish: enqueue failed",
			"event", eventType,
			"error", err,
		)
		return
	}

	slog.Info("event published",
		"event", eventType,
		"task_id", info.ID,
		"queue", info.Queue,
	)
}
