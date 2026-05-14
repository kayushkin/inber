package server

import (
	"encoding/json"
	"net/http"
	"time"

	agentstore "github.com/kayushkin/agent-store"
	"github.com/kayushkin/inber/agent/registry"
	"github.com/kayushkin/inber/logger"
)

type agentConfigEntry struct {
	Slug            string `json:"slug"`
	DisplayName     string `json:"display_name"`
	Emoji           string `json:"emoji,omitempty"`
	Model           string `json:"model"`
	MaxTurns        int    `json:"max_turns"`
	MaxInputTokens  int    `json:"max_input_tokens"`
	MaxResponseTime int    `json:"max_response_time"`
	ThinkingBudget  int    `json:"thinking_budget"`
	ContextBudget   int    `json:"context_budget"`
	Enabled         bool   `json:"enabled"`
	Shelved         bool   `json:"shelved"`
}

type agentConfigPatch struct {
	Slug            string  `json:"slug"`
	Model           *string `json:"model,omitempty"`
	MaxTurns        *int    `json:"max_turns,omitempty"`
	MaxInputTokens  *int    `json:"max_input_tokens,omitempty"`
	MaxResponseTime *int    `json:"max_response_time,omitempty"`
	ThinkingBudget  *int    `json:"thinking_budget,omitempty"`
	ContextBudget   *int    `json:"context_budget,omitempty"`
}

// GET /api/agents/config — list all inber agent configs with limits
// PATCH /api/agents/config — update an agent's config
func (g *Server) handleAgentConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, PATCH, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if g.agentStore == nil {
		jsonError(w, "agent-store not available", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		g.handleAgentConfigGet(w)
	case http.MethodPatch:
		g.handleAgentConfigPatch(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (g *Server) handleAgentConfigGet(w http.ResponseWriter) {
	agents, err := g.agentStore.ListAgentsExpanded()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var entries []agentConfigEntry
	for _, a := range agents {
		if a.Harness != "inber" {
			continue
		}
		// Get full harness config for limits
		aos, err := g.agentStore.GetAgentBySlug(a.Slug)
		if err != nil {
			continue
		}
		harnessConfigs, err := g.agentStore.GetAgentHarnesses(aos.ID)
		if err != nil {
			continue
		}
		for _, ah := range harnessConfigs {
			if ah.HarnessID != "inber" {
				continue
			}
			entries = append(entries, agentConfigEntry{
				Slug:            a.Slug,
				DisplayName:     a.DisplayName,
				Emoji:           a.Emoji,
				Model:           ah.ModelPrimary,
				MaxTurns:        ah.MaxTurns,
				MaxInputTokens:  ah.MaxInputTokens,
				MaxResponseTime: ah.MaxResponseTime,
				ThinkingBudget:  ah.ThinkingBudget,
				ContextBudget:   ah.ContextBudget,
				Enabled:         ah.Enabled,
				Shelved:         ah.Shelved,
			})
		}
	}

	jsonResponse(w, entries)
}

func (g *Server) handleAgentConfigPatch(w http.ResponseWriter, r *http.Request) {
	var patch agentConfigPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if patch.Slug == "" {
		jsonError(w, "slug is required", http.StatusBadRequest)
		return
	}

	agent, err := g.agentStore.GetAgentBySlug(patch.Slug)
	if err != nil {
		jsonError(w, "agent not found: "+patch.Slug, http.StatusNotFound)
		return
	}

	harnessConfigs, err := g.agentStore.GetAgentHarnesses(agent.ID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var ah *agentstore.AgentHarness
	for i := range harnessConfigs {
		if harnessConfigs[i].HarnessID == "inber" {
			ah = &harnessConfigs[i]
			break
		}
	}
	if ah == nil {
		jsonError(w, "agent not registered with inber", http.StatusNotFound)
		return
	}

	// Apply patches
	if patch.Model != nil {
		ah.ModelPrimary = *patch.Model
	}
	if patch.MaxTurns != nil {
		ah.MaxTurns = *patch.MaxTurns
	}
	if patch.MaxInputTokens != nil {
		ah.MaxInputTokens = *patch.MaxInputTokens
	}
	if patch.MaxResponseTime != nil {
		ah.MaxResponseTime = *patch.MaxResponseTime
	}
	if patch.ThinkingBudget != nil {
		ah.ThinkingBudget = *patch.ThinkingBudget
	}
	if patch.ContextBudget != nil {
		ah.ContextBudget = *patch.ContextBudget
	}
	ah.UpdatedAt = time.Now().Unix()

	if err := g.agentStore.UpsertAgentHarness(ah); err != nil {
		jsonError(w, "update failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Reload registry so changes take effect without restart
	g.reloadRegistry()

	jsonResponse(w, map[string]string{"status": "ok", "slug": patch.Slug})
}

// reloadRegistry refreshes the agent config from agent-store.
func (g *Server) reloadRegistry() {
	cfg, err := registry.LoadFromAgentStore("")
	if err != nil {
		logger.WithComponent("server").Warn("failed to reload registry", map[string]interface{}{"error": err})
		return
	}
	newAgents := make(map[string]AgentConfig, len(cfg.Agents))
	for name, rc := range cfg.Agents {
		newAgents[name] = AgentConfig{
			Name:      rc.Name,
			Project:   rc.Project,
			Projects:  rc.Projects,
			Workspace: rc.Workspace,
			Model:     rc.Model,
			Thinking:  rc.Thinking,
			Tools:     rc.Tools,
		}
	}
	g.config.Agents = newAgents
	if cfg.Default != "" {
		g.config.DefaultAgent = cfg.Default
	}
	logger.WithComponent("server").Info("reloaded agent registry", map[string]interface{}{"count": len(cfg.Agents)})
}
