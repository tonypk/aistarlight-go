package domain

import (
	"time"

	"github.com/google/uuid"
)

type ChatMessage struct {
	ID        uuid.UUID `json:"id"`
	CompanyID uuid.UUID `json:"company_id"`
	UserID    uuid.UUID `json:"user_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	ToolCalls JSON      `json:"tool_calls,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
