package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentstore "github.com/kayushkin/agent-store"
)

// AgentConfig defines an agent's configuration
type AgentConfig struct {
	Name      string        `json:"name"`
	Role      string        `json:"role"`
	Project   string        `json:"project,omitempty"`   // primary project name
	Projects  []string      `json:"projects,omitempty"`  // all assigned projects
	System    string        `json:"-"`                   // system prompt (soul + shared identity)
	Model     string        `json:"model"`
	Thinking  int64         `json:"thinking"`
	Tools     []string      `json:"tools"`
	Context   ContextConfig `json:"context"`
	Limits    *AgentLimits  `json:"limits,omitempty"`
	Workspace string        `json:"workspace,omitempty"` // workspace path
	Shelved   bool          `json:"shelved,omitempty"`
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

// agentStoreDBPath returns the path to the agent-store database.
// Uses AGENT_STORE_PATH env var, or defaults to ~/.config/agent-store/agents.db.
func agentStoreDBPath(override string) string {
	if override != "" {
		return override
	}
	if p := os.Getenv("AGENT_STORE_PATH"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "agent-store", "agents.db")
}

// LoadFromAgentStore loads agent configs from the agent-store database.
// This is the only source of truth for agent configuration.
func LoadFromAgentStore(dbPath string) (*RegistryConfig, error) {
	dbPath = agentStoreDBPath(dbPath)

	store, err := agentstore.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open agent-store at %s: %w", dbPath, err)
	}
	defer store.Close()

	allConfigs, defaultSlug, err := store.GetAllAgentConfigs("inber")
	if err != nil {
		return nil, fmt.Errorf("get all agent configs: %w", err)
	}

	if len(allConfigs) == 0 {
		return nil, fmt.Errorf("no agents registered with inber orchestrator in agent-store")
	}

	configs := make(map[string]*AgentConfig, len(allConfigs))
	for _, fc := range allConfigs {
		// Build system prompt from nature content
		var systemParts []string
		if fc.Soul != "" {
			systemParts = append(systemParts, fc.Soul)
		}
		if fc.Principles != "" {
			systemParts = append(systemParts, fc.Principles)
		}
		if fc.Values != "" {
			systemParts = append(systemParts, fc.Values)
		}
		if fc.UserContext != "" {
			systemParts = append(systemParts, fc.UserContext)
		}
		// If agent has a custom system prompt override, use that instead
		systemPrompt := strings.Join(systemParts, "\n\n")
		if fc.SystemPrompt != "" {
			systemPrompt = fc.SystemPrompt
		}

		// Parse projects from comma-separated string
		var projects []string
		if fc.Projects != "" {
			for _, p := range strings.Split(fc.Projects, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					projects = append(projects, p)
				}
			}
		}

		cfg := &AgentConfig{
			Name:      fc.DisplayName,
			Role:      fc.Role,
			Project:   fc.Project,
			Projects:  projects,
			System:    systemPrompt,
			Model:     fc.Model,
			Thinking:  int64(fc.Thinking),
			Tools:     fc.Tools,
			Workspace: fc.Workspace,
			Shelved:   fc.Shelved,
			Context: ContextConfig{
				Tags:   fc.ContextTags,
				Budget: fc.ContextBudget,
			},
		}

		// Set limits if any are non-zero
		if fc.MaxTurns > 0 || fc.MaxInputTokens > 0 || fc.MaxResponseTime > 0 {
			cfg.Limits = &AgentLimits{
				MaxTurns:        fc.MaxTurns,
				MaxInputTokens:  fc.MaxInputTokens,
				MaxResponseTime: fc.MaxResponseTime,
			}
		}

		// Default model
		if cfg.Model == "" {
			cfg.Model = "claude-sonnet-4-5"
		}

		configs[fc.Slug] = cfg
	}

	return &RegistryConfig{
		Default: defaultSlug,
		Agents:  configs,
	}, nil
}

// LoadConfig loads agent configuration from agent-store.
// This is the primary entry point for loading config.
func LoadConfig() (*RegistryConfig, error) {
	return LoadFromAgentStore("")
}
