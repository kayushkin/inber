// Package server configuration types and loading functions.
package server

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/engine"
	toolstoretools "github.com/kayushkin/tool-store/tools"
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

	// NATS bus integration.
	NatsURL  string `json:"nats_url,omitempty"`  // default nats://localhost:4222
	BusToken string `json:"bus_token,omitempty"` // kept for event publisher compat

	// OpenClaw proxy — forward bus messages where orchestrator=openclaw.
	OpenClawURL   string `json:"openclaw_url,omitempty"`   // e.g. "http://localhost:18789"
	OpenClawToken string `json:"openclaw_token,omitempty"` // bearer token

	// BridgeSessionTTL is how long a bridge- session can be idle before the
	// reaper removes it. Defaults to 1 hour. Set to 0 to disable.
	BridgeSessionTTL time.Duration `json:"bridge_session_ttl,omitempty"`

	// RequireChecks is the allowlist of selftest check names that are
	// fatal at startup. Any check not in this list is demoted to WARN.
	// If nil/empty, the legacy behavior applies (agent-store is the only
	// fatal check). Valid names: "nats", "agent-store", "workspace",
	// "anthropic-key".
	RequireChecks []string `json:"require_checks,omitempty"`

	// AgentStorePath is the agent-store database agents are loaded from, and
	// the one status queries read. Empty means agent-store's default path. The
	// command sets it from AGENT_STORE_PATH; a config file cannot.
	AgentStorePath string `json:"-"`

	// LogstackURL is where every session's log is also sent. Empty sends it
	// nowhere else. The command sets it from LOGSTACK_URL.
	LogstackURL string `json:"-"`

	// ToolConnections is where every session's browser, web search and
	// scheduler tools reach PinchTab, Brave and the scheduler. The command sets
	// it from PINCHTAB_URL, PINCHTAB_TOKEN, BRAVE_API_KEY, SCHEDULER_URL and
	// SCHEDULER_TOKEN.
	ToolConnections toolstoretools.OutsideServiceConnections `json:"-"`

	// ProviderAPIKeys are the provider credentials every session tries before
	// aiauth's, and the Anthropic one is what POST /api/oneshot sends with. The
	// command sets them from ANTHROPIC_API_KEY, OPENAI_API_KEY, GOOGLE_API_KEY
	// and OPENROUTER_API_KEY, or the Anthropic one from auth-store.
	ProviderAPIKeys agent.ProviderAPIKeys `json:"-"`

	// Blueprint turns on prompt blueprint diffs for every session. The command
	// sets it from INBER_BLUEPRINT.
	Blueprint bool `json:"-"`

	// SettingsHandler serves GET /settings: the environment variables the
	// command declared, as llm-bridge's servicesettings describes them. The
	// command builds it; Serve refuses to start without one.
	SettingsHandler http.Handler `json:"-"`
}

// AgentConfig defines one agent.
type AgentConfig struct {
	Name      string   `json:"name"`
	Project   string   `json:"project,omitempty"`  // primary project name
	Projects  []string `json:"projects,omitempty"` // all repos for workspace isolation
	Workspace string   `json:"workspace"`          // repo root / cwd
	Model     string   `json:"model"`
	Thinking  int64    `json:"thinking"`
	Tools     []string `json:"tools"` // tool allowlist (empty = all)

	// WorkspaceRoots is every repository of the forge workspace this agent was
	// spawned into, filled in at spawn time alongside Workspace and never read
	// from stored configuration — which is why it is not serialized. A session
	// outside a workspace leaves it empty.
	WorkspaceRoots []engine.WorkspaceRoot `json:"-"`
}

// BridgeSessionTTL is the default TTL for idle bridge sessions.
const defaultBridgeSessionTTL = time.Hour

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
// This bridges the agent-store system.
func ConfigFromAgents(agents map[string]AgentConfig, defaultAgent string) Config {
	return Config{
		Agents:       agents,
		DefaultAgent: defaultAgent,
	}
}
