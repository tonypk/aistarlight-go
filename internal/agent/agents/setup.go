package agents

import "github.com/tonypk/aistarlight-go/internal/agent"

// RegisterAll registers all built-in agent definitions to the registry.
func RegisterAll(r *agent.Registry) {
	r.Register(General())
	r.Register(Filing())
	r.Register(Recon())
	r.Register(Journal())
	r.Register(Compliance())
	r.Register(Classifier())
}
