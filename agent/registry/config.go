package registry

import (
	"fmt"
	"strings"

	agentstore "github.com/kayushkin/agent-store"
)

// AgentConfig defines an agent's configuration
type AgentConfig struct {
	Name     string        `json:"name"`
	Role     string        `json:"role"`
	Project  string        `json:"project,omitempty"` // forge project name (e.g. "kayushkin.com")
	System   string        `json:"-"`                 // loaded from agent-store nature
	Model    string        `json:"model"`
	Thinking int64         `json:"thinking"`
	Tools    []string      `json:"tools"`
	Context  ContextConfig `json:"context"`
	Limits   *AgentLimits  `json:"limits,omitempty"`
}

// AgentLimits defines per-agent safety limits for token/turn usage
type AgentLimits struct {
	MaxTurns        int `json:"maxTurns,omitempty"`
	MaxInputTokens  int `json:"maxInputTokens,omitempty"`
	MaxResponseTime int `json:"maxResponseTime,omitempty"` // max seconds for orchestrator to respond/spawn
}

// ContextConfig defines context settings for an agent
type ContextConfig struct {
	Tags          []string `json:"tags"`
	Budget        int      `json:"budget"`         // token budget for context
	InheritParent bool     `json:"inherit_parent"` // inherit parent's context
}

// OpenClawConfig defines OpenClaw gateway configuration
type OpenClawConfig struct {
	URL    string   `json:"url"`    // WebSocket URL (e.g., ws://localhost:18789/ws)
	Token  string   `json:"token"`  // Auth token
	Agents []string `json:"agents"` // Agent names that route to OpenClaw
}

// TiersConfig defines default model tiers for racing/fallback.
type TiersConfig struct {
	High  []string `json:"high"`            // expensive models for planning (e.g. opus46, opus45, sonnet45)
	Low   []string `json:"low"`             // cheap models for execution (e.g. glm5, glm47, haiku)
	Delay int      `json:"delay,omitempty"` // seconds between staggered launches (default 4)
	Grace int      `json:"grace,omitempty"` // seconds to wait for better model after fallback responds (default 8)
}

// RegistryConfig holds the loaded configuration including default agent
type RegistryConfig struct {
	Default  string
	Agents   map[string]*AgentConfig
	Tiers    *TiersConfig
	OpenClaw *OpenClawConfig
}

// LoadFromAgentStore loads agent configs from the agent-store database.
// This is the only source of truth for agent configuration.
func LoadFromAgentStore(dbPath string) (*RegistryConfig, error) {
	store, err := agentstore.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open agent-store: %w", err)
	}
	defer store.Close()

	// Get orchestrator config for inber
	orch, err := store.GetOrchestrator("inber")
	if err != nil {
		return nil, fmt.Errorf("get inber orchestrator: %w", err)
	}

	// Get all agents
	agents, err := store.ListAgents()
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}

	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents in agent-store")
	}

	// Build config map
	configs := make(map[string]*AgentConfig)
	for _, a := range agents {
		cfg := &AgentConfig{
			Name: a.DisplayName,
			Role: a.Role,
		}

		// Get agent nature (identity, principles, values, user)
		natures, err := store.ListAgentNature(a.ID)
		if err == nil && len(natures) > 0 {
			var systemParts []string
			for _, n := range natures {
				if n.Content != "" {
					systemParts = append(systemParts, n.Content)
				}
			}
			cfg.System = strings.Join(systemParts, "\n\n")
		}

		// Get agent-orchestrator config for model/workspace
		aos, err := store.GetAgentOrchestrators(a.ID)
		if err == nil {
			for _, ao := range aos {
				if ao.OrchestratorID == "inber" {
					if ao.ModelPrimary != "" {
						cfg.Model = ao.ModelPrimary
					}
					if ao.WorkspacePath != "" {
						cfg.Project = ao.WorkspacePath
					}
					break
				}
			}
		}

		// Set default model if not specified
		if cfg.Model == "" {
			cfg.Model = "claude-sonnet-4-5"
		}

		configs[a.Slug] = cfg
	}

	// Resolve default agent from orchestrator's DefaultAgentID
	var defaultAgent string
	if orch.DefaultAgentID != nil {
		for _, a := range agents {
			if a.ID == *orch.DefaultAgentID {
				defaultAgent = a.Slug
				break
			}
		}
	}

	return &RegistryConfig{
		Default: defaultAgent,
		Agents:  configs,
	}, nil
}

// LoadConfigWithFallback loads agent config from agent-store.
// The configPath and identityDir parameters are no longer used but kept for API compatibility.
// Returns the config and true (always from agent-store now).
func LoadConfigWithFallback(configPath, identityDir string) (*RegistryConfig, bool) {
	cfg, err := LoadFromAgentStore("")
	if err != nil {
		return nil, false
	}
	return cfg, true
}
