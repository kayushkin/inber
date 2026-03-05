package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AgentConfig defines an agent's configuration
type AgentConfig struct {
	Name     string        `json:"name"`
	Role     string        `json:"role"`
	System   string        `json:"-"` // loaded from markdown file
	Model    string        `json:"model"`
	Thinking int64         `json:"thinking"`
	Tools    []string      `json:"tools"`
	Context  ContextConfig `json:"context"`
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

// agentsFile is the JSON config file structure
type agentsFile struct {
	Default  string                     `json:"default"`            // default agent name
	Agents   map[string]*AgentConfig    `json:"agents"`
	Tiers    *TiersConfig               `json:"tiers,omitempty"`    // default model tiers
	OpenClaw *OpenClawConfig            `json:"openclaw,omitempty"` // OpenClaw gateway config
}

// RegistryConfig holds the loaded configuration including default agent
type RegistryConfig struct {
	Default  string
	Agents   map[string]*AgentConfig
	Tiers    *TiersConfig
	OpenClaw *OpenClawConfig
}

// LoadConfig loads an agent config from JSON + markdown files
// configPath should point to the agents.json file
// identityDir should point to the directory containing .md files
func LoadConfig(configPath, identityDir string) (*RegistryConfig, error) {
	// Read JSON config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var af agentsFile
	if err := json.Unmarshal(data, &af); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}

	if len(af.Agents) == 0 {
		return nil, fmt.Errorf("no agents defined in %s", configPath)
	}

	// Load identity (system prompt) from markdown files
	for name, cfg := range af.Agents {
		mdPath := filepath.Join(identityDir, name+".md")
		identityData, err := os.ReadFile(mdPath)
		if err != nil {
			return nil, fmt.Errorf("read identity for %s: %w", name, err)
		}

		cfg.System = string(identityData)

		// Validate required fields
		if cfg.Name == "" {
			return nil, fmt.Errorf("agent name is required")
		}
		if cfg.System == "" {
			return nil, fmt.Errorf("agent %s: system prompt is empty", name)
		}
		if cfg.Model == "" {
			cfg.Model = "claude-sonnet-4-5" // default model
		}
	}

	// Set default agent if specified
	defaultAgent := af.Default
	if defaultAgent != "" {
		if _, ok := af.Agents[defaultAgent]; !ok {
			return nil, fmt.Errorf("default agent %q not found in agents", defaultAgent)
		}
	}

	return &RegistryConfig{
		Default:  defaultAgent,
		Agents:   af.Agents,
		Tiers:    af.Tiers,
		OpenClaw: af.OpenClaw,
	}, nil
}

// LoadConfigDir loads agent configs from a directory
// Expects: agents.json and .md files in the same directory
func LoadConfigDir(dir string) (*RegistryConfig, error) {
	configPath := filepath.Join(dir, "..", "agents.json")
	return LoadConfig(configPath, dir)
}
