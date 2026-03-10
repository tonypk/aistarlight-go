package agent

import "sync"

// Registry holds all registered agent definitions.
type Registry struct {
	mu     sync.RWMutex
	agents map[string]*AgentDefinition
}

// NewRegistry creates a new agent registry.
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]*AgentDefinition),
	}
}

// Register adds an agent definition to the registry.
func (r *Registry) Register(def *AgentDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[def.ID] = def
}

// Get returns an agent definition by ID.
func (r *Registry) Get(agentID string) (*AgentDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.agents[agentID]
	return def, ok
}

// ListAll returns all registered agents.
func (r *Registry) ListAll() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]AgentInfo, 0, len(r.agents))
	for _, def := range r.agents {
		result = append(result, def.Info())
	}
	return result
}

// ListForWorkflow returns agents that apply to a given workflow type.
func (r *Registry) ListForWorkflow(workflowType string) []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []AgentInfo
	for _, def := range r.agents {
		if workflowType == "" {
			result = append(result, def.Info())
			continue
		}
		for _, wt := range def.WorkflowTypes {
			if wt == workflowType || wt == "*" {
				result = append(result, def.Info())
				break
			}
		}
	}
	return result
}
