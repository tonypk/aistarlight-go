package agent

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	oai "github.com/sashabaranov/go-openai"
)

// AgentDefinition describes a specialized AI agent.
type AgentDefinition struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Description     string                `json:"description"`
	Icon            string                `json:"icon"`
	Color           string                `json:"color"`
	Hint            string                `json:"hint"`
	SampleQuestions []string              `json:"sample_questions"`
	Recommended     bool                  `json:"recommended"`
	SystemPrompts   map[string]string     `json:"-"` // jurisdiction -> prompt
	Tools           map[string][]oai.Tool `json:"-"` // jurisdiction -> tools
	WorkflowTypes   []string              `json:"workflow_types"`
}

// SystemPrompt returns the system prompt for a given jurisdiction.
func (d *AgentDefinition) SystemPrompt(jurisdiction string) string {
	if p, ok := d.SystemPrompts[jurisdiction]; ok {
		return p
	}
	return d.SystemPrompts["PH"]
}

// ToolsFor returns the tool definitions for a given jurisdiction.
func (d *AgentDefinition) ToolsFor(jurisdiction string) []oai.Tool {
	if t, ok := d.Tools[jurisdiction]; ok {
		return t
	}
	return d.Tools["PH"]
}

// AgentInfo is the public-facing agent metadata (for API responses).
type AgentInfo struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Icon            string   `json:"icon"`
	Color           string   `json:"color"`
	Hint            string   `json:"hint"`
	SampleQuestions []string `json:"sample_questions"`
	Recommended     bool     `json:"recommended"`
	WorkflowTypes   []string `json:"workflow_types"`
}

// Info returns the public metadata for this agent.
func (d *AgentDefinition) Info() AgentInfo {
	return AgentInfo{
		ID:              d.ID,
		Name:            d.Name,
		Description:     d.Description,
		Icon:            d.Icon,
		Color:           d.Color,
		Hint:            d.Hint,
		SampleQuestions:  d.SampleQuestions,
		Recommended:     d.Recommended,
		WorkflowTypes:   d.WorkflowTypes,
	}
}

// StreamEvent represents a single event in the SSE stream.
type StreamEvent struct {
	Token      string          `json:"token,omitempty"`
	Done       bool            `json:"done,omitempty"`
	Content    string          `json:"content,omitempty"`
	Error      string          `json:"error,omitempty"`
	ThreadID   string          `json:"thread_id,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	Actions    json.RawMessage `json:"actions,omitempty"`
	Citations  json.RawMessage `json:"citations,omitempty"`
}

// AgentRequest is the input for an agent interaction.
type AgentRequest struct {
	AgentID      string                 `json:"agent_id"`
	Content      string                 `json:"content"`
	ThreadID     *uuid.UUID             `json:"thread_id"`
	CompanyID    uuid.UUID              `json:"company_id"`
	UserID       uuid.UUID              `json:"user_id"`
	Jurisdiction string                 `json:"jurisdiction"`
	WorkflowType string                 `json:"workflow_type"`
	EntityType   string                 `json:"entity_type"`
	EntityID     *uuid.UUID             `json:"entity_id"`
	Context      map[string]interface{} `json:"context"`
}

// ToolExecuteFunc is the function signature for executing a tool call.
type ToolExecuteFunc func(ctx context.Context, agentID, toolName string, args json.RawMessage, companyID uuid.UUID, userID uuid.UUID, jurisdiction string) (string, error)
