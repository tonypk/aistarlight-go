package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

type KnowledgeChunk struct {
	ID        uuid.UUID      `json:"id"`
	Source    *string        `json:"source,omitempty"`
	Category *string        `json:"category,omitempty"`
	Content  string         `json:"content"`
	Embedding pgvector.Vector `json:"-"`
	Metadata JSON           `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}
