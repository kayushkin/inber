package registry

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AgentConfig defines an agent's configuration
type AgentConfig struct {
	Name     string        `yaml:"name"`
	Role     string        `yaml:"role"`
	System   string        `yaml:"system"`
	Model    string        `yaml:"model"`
	Thinking int64         `yaml:"thinking"`
	Tools    []string      `yaml:"tools"`
	Context  ContextConfig `yaml:"context"`
}

// ContextConfig defines context settings for an agent
type ContextConfig struct {
	Tags         []string `yaml:"tags"`
	Budget       int      `yaml:"budget"`        // token budget for context
	InheritParent bool    `yaml:"inherit_parent"` // inherit parent's context
}

// LoadConfig loads an agent config from a YAML file
func LoadConfig(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	// Validate required fields
	if cfg.Name == "" {
		return nil, fmt.Errorf("agent name is required")
	}
	if cfg.System == "" {
		return nil, fmt.Errorf("agent system prompt is required")
	}
	if cfg.Model == "" {
		cfg.Model = "claude-sonnet-4-5" // default model
	}

	return &cfg, nil
}

// LoadConfigDir loads all agent configs from a directory
func LoadConfigDir(dir string) (map[string]*AgentConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	configs := make(map[string]*AgentConfig)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		cfg, err := LoadConfig(path)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", entry.Name(), err)
		}

		configs[cfg.Name] = cfg
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no agent configs found in %s", dir)
	}

	return configs, nil
}
