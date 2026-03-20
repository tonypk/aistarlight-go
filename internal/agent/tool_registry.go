// internal/agent/tool_registry.go
package agent

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	oai "github.com/sashabaranov/go-openai"
)

// RiskLevel indicates whether a tool requires user confirmation.
type RiskLevel string

const (
	RiskLow  RiskLevel = "low"
	RiskHigh RiskLevel = "high"
)

// ToolContext carries per-invocation context for tool execution.
type ToolContext struct {
	Ctx          context.Context
	CompanyID    uuid.UUID
	UserID       uuid.UUID
	Jurisdiction string // "PH", "SG", "LK"
	AgentID      string
	ThreadID     uuid.UUID
}

// ToolDef defines a single executable tool.
type ToolDef struct {
	Name        string
	Description string          // Human-readable, used in LLM schema
	Parameters  json.RawMessage // JSON Schema for function calling
	RiskLevel   RiskLevel
	AgentIDs    []string // Which agents may use this tool
	SummaryTmpl string   // Template for generating human-readable summary, e.g. "Classify {count} transactions"
	Execute     func(tc ToolContext, args json.RawMessage) (json.RawMessage, error)
}

// OAITool converts a ToolDef to the OpenAI function calling format.
func (t *ToolDef) OAITool() oai.Tool {
	return oai.Tool{
		Type: oai.ToolTypeFunction,
		Function: &oai.FunctionDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		},
	}
}

// ToolRegistry manages all available tools.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]*ToolDef
}

// NewToolRegistry creates an empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]*ToolDef)}
}

// Register adds a tool to the registry.
func (r *ToolRegistry) Register(t *ToolDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
}

// Get returns a tool by name.
func (r *ToolRegistry) Get(name string) (*ToolDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// ForAgent returns all tools available to a specific agent as OAI tool definitions.
func (r *ToolRegistry) ForAgent(agentID string) []oai.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []oai.Tool
	for _, t := range r.tools {
		for _, aid := range t.AgentIDs {
			if aid == agentID || aid == "*" {
				result = append(result, t.OAITool())
				break
			}
		}
	}
	return result
}
