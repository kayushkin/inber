package registry

import (
	"encoding/json"
	"fmt"
	"strconv"
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
	Tags         []string `json:"tags"`
	Budget       int      `json:"budget"`         // token budget for context
	InheritParent bool    `json:"inherit_parent"` // inherit parent's context
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
			Name: a.Name,
			Role: a.Role,
		}

		// Get agent nature (identity, principles, values, user)
		natures, err := store.GetAgentNature(a.ID)
		if err == nil && len(natures) > 0 {
			var systemParts []string
			for _, n := range natures {
				if n.Content != "" {
					systemParts = append(systemParts, n.Content)
				}
			}
			cfg.System = strings.Join(systemParts, "\n\n")
		}

		// Get agent-specific config for inber
		agentCfg, err := store.GetAgentConfig(a.ID, "inber")
		if err == nil {
			// Model
			if model, ok := agentCfg.Values["model"]; ok && model != "" {
				cfg.Model = model
			}
			// Thinking budget
			if thinking, ok := agentCfg.Values["thinking"]; ok {
				if t, err := strconv.ParseInt(thinking, 10, 64); err == nil {
					cfg.Thinking = t
				}
			}
			// Context budget
			if budget, ok := agentCfg.Values["context_budget"]; ok {
				if b, err := strconv.Atoi(budget); err == nil {
					cfg.Context.Budget = b
				}
			}
			// Project (forge workspace)
			if project, ok := agentCfg.Values["project"]; ok && project != "" {
				cfg.Project = project
			}
			// Tools
			for _, tc := range agentCfg.Tools {
				if tc.Enabled {
					cfg.Tools = append(cfg.Tools, tc.Tool)
				}
			}
			// Limits
			if len(agentCfg.Limits) > 0 {
				cfg.Limits = &AgentLimits{}
				if maxTurns, ok := agentCfg.Limits["max_turns"]; ok {
					cfg.Limits.MaxTurns = maxTurns
				}
				if maxInputTokens, ok := agentCfg.Limits["max_input_tokens"]; ok {
					cfg.Limits.MaxInputTokens = maxInputTokens
				}
				if maxResponseTime, ok := agentCfg.Limits["max_response_time"]; ok {
					cfg.Limits.MaxResponseTime = maxResponseTime
				}
			}
		}

		// Set default model if not specified
		if cfg.Model == "" {
			cfg.Model = "claude-sonnet-4-5"
		}

		configs[a.ID] = cfg
	}

	// Get orchestrator settings (tiers)
	var tiers *TiersConfig
	settings, err := store.GetOrchestratorSettings("inber")
	if err == nil && len(settings) > 0 {
		tiers = &TiersConfig{}
		if highJSON, ok := settings["tier_high"]; ok {
			var high []string
			if err := json.Unmarshal([]byte(highJSON), &high); err == nil {
				tiers.High = high
			}
		}
		if lowJSON, ok := settings["tier_low"]; ok {
			var low []string
			if err := json.Unmarshal([]byte(lowJSON), &low); err == nil {
				tiers.Low = low
			}
		}
		if delay, ok := settings["tier_delay"]; ok {
			if d, err := strconv.Atoi(delay); err == nil {
				tiers.Delay = d
			}
		}
		if grace, ok := settings["tier_grace"]; ok {
			if g, err := strconv.Atoi(grace); err == nil {
				tiers.Grace = g
			}
		}
	}

	return &RegistryConfig{
		Default:  orch.DefaultAgent,
		Agents:   configs,
		Tiers:    tiers,
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
