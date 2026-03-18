// Package server configuration types and loading functions.
package server

import (
	"encoding/json"
	"os"
)

// Config defines the server's runtime configuration.
type Config struct {
	// Agent definitions: name → config.
	Agents map[string]AgentConfig `json:"agents"`

	// Default agent for unrouted messages.
	DefaultAgent string `json:"default_agent"`

	// Queue concurrency.
	MainConcurrency     int `json:"main_concurrency"`     // default 4
	SubagentConcurrency int `json:"subagent_concurrency"` // default 8

	// Sub-agent limits.
	MaxSpawnDepth       int `json:"max_spawn_depth"`        // default 2
	MaxChildrenPerAgent int `json:"max_children_per_agent"` // default 5

	// API server.
	ListenAddr string `json:"listen_addr"` // default ":8200"

	// Data directory for session persistence.
	DataDir string `json:"data_dir"` // default ~/.inber/server

	// Bus integration for dashboard events.
	BusURL   string `json:"bus_url,omitempty"`
	BusToken string `json:"bus_token,omitempty"`

	// OpenClaw proxy — forward bus messages where orchestrator=openclaw.
	OpenClawURL   string `json:"openclaw_url,omitempty"`   // e.g. "http://localhost:18789"
	OpenClawToken string `json:"openclaw_token,omitempty"` // bearer token
}

// AgentConfig defines one agent.
type AgentConfig struct {
	Name      string   `json:"name"`
	Project   string   `json:"project,omitempty"`  // primary project name
	Projects  []string `json:"projects,omitempty"` // all repos for workspace isolation
	Workspace string   `json:"workspace"`          // repo root / cwd
	Model     string   `json:"model"`
	Thinking  int64    `json:"thinking"`
	Tools     []string `json:"tools"`             // tool allowlist (empty = all)
}

// LoadConfig loads a server configuration from a JSON file.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

// ConfigFromAgents builds a Config from agent registry data.
// This bridges the existing agents.json / agent-store system.
func ConfigFromAgents(agents map[string]AgentConfig, defaultAgent string) Config {
	return Config{
		Agents:       agents,
		DefaultAgent: defaultAgent,
	}
}