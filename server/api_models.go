package server

import (
	"net/http"
)

// registryAgent is the API response type for agent listings.
type registryAgent struct {
	Name         string `json:"name"`
	Orchestrator string `json:"orchestrator"`
	Description  string `json:"description,omitempty"`
	Enabled      bool   `json:"enabled"`
}

// GET /api/models — proxy to model store
func (g *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if g.modelStore == nil {
		jsonError(w, "model store not available", http.StatusServiceUnavailable)
		return
	}
	statuses, err := g.modelStore.AllModelsWithStatus()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, statuses)
}

// GET /api/agents — list all agents across all orchestrators.
// Returns inber agents from config.
func (g *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var agents []registryAgent

	// Inber agents from config — these are reachable via bus. Sorted, because
	// agent/registry/spawn_tool.go reads this body and renders it into a tool
	// description; a response whose order changes per request costs the CLI a
	// prompt-cache miss on every tool build.
	for _, name := range sortedAgentNames(g.config.Agents) {
		agents = append(agents, registryAgent{
			Name:         name,
			Orchestrator: "inber",
			Enabled:      true,
		})
	}

	jsonResponse(w, agents)
}
