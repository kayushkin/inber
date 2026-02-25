package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kayushkin/inber/agent"
)

// AgentConfig defines an agent's configuration
type AgentConfig struct {
	Name     string               `json:"name"`
	Role     string               `json:"role"`
	System   string               `json:"-"` // loaded from markdown file
	Model    string               `json:"model"`
	Thinking int64                `json:"thinking"`
	Tools    []string             `json:"tools"`
	Context  ContextConfig        `json:"context"`
	Hooks    *agent.HookConfig    `json:"hooks,omitempty"`
}

// ContextConfig defines context settings for an agent
type ContextConfig struct {
	Tags         []string `json:"tags"`
	Budget       int      `json:"budget"`         // token budget for context
	InheritParent bool    `json:"inherit_parent"` // inherit parent's context
}

// agentsFile is the JSON config file structure
type agentsFile struct {
	Agents map[string]*AgentConfig `json:"agents"`
}

// LoadConfig loads an agent config from JSON + markdown files
// configPath should point to the agents.json file
// identityDir should point to the directory containing .md files
func LoadConfig(configPath, identityDir string) (map[string]*AgentConfig, error) {
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

	return af.Agents, nil
}

// LoadConfigDir loads agent configs from a directory
// Expects: agents.json and .md files in the same directory
func LoadConfigDir(dir string) (map[string]*AgentConfig, error) {
	configPath := filepath.Join(dir, "..", "agents.json")
	return LoadConfig(configPath, dir)
}
