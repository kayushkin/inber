package server

import (
	"net/http"
	"os"
	"path/filepath"
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
// Returns inber agents from config + openclaw agents from filesystem scan.
func (g *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var agents []registryAgent

	// Inber agents from config — these are reachable via bus.
	for name := range g.config.Agents {
		agents = append(agents, registryAgent{
			Name:         name,
			Orchestrator: "inber",
			Enabled:      true,
		})
	}

	// OpenClaw agents — proxied through inber server.
	if g.config.OpenClawURL != "" {
		home, _ := os.UserHomeDir()
		agentsDir := filepath.Join(home, ".openclaw", "agents")
		if entries, err := os.ReadDir(agentsDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				agentSubdir := filepath.Join(agentsDir, entry.Name(), "agent")
				if _, err := os.Stat(agentSubdir); err != nil {
					continue
				}
				agents = append(agents, registryAgent{
					Name:         entry.Name(),
					Orchestrator: "openclaw",
					Enabled:      true,
				})
			}
		}
	}

	jsonResponse(w, agents)
}