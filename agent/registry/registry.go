package registry

import (
	"fmt"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/session"
	modelstore "github.com/kayushkin/model-store"
)

// Registry manages multiple agents with isolated sessions and contexts
type Registry struct {
	mu            sync.RWMutex
	client        *anthropic.Client
	modelClient   *agent.ModelClient // unified client (Anthropic or OpenAI)
	modelStore    *modelstore.Store  // model store for creating per-agent clients
	logsDir       string
	default_      string
	configs       map[string]*AgentConfig
	agents        map[string]*agent.Agent
	sessions      map[string]*session.Session
	tools         *ToolRegistry
	openclawURL   string   // OpenClaw gateway URL
	openclawToken string   // OpenClaw auth token
	openclawAgents []string // Agents that route to OpenClaw

	// listRegisteredAgents supplies the bus-agent registry entries that the
	// spawn tool validates a requested agent against and builds its parameter
	// descriptions from. Nil means the live registry over HTTP. It exists as a
	// field because the live call is a hardcoded GET against a fixed port, so
	// without it no test can reach the spawn tool's body for a stated reason —
	// only for whatever that endpoint happens to be serving on this box.
	listRegisteredAgents func() []RegistryAgent
}

// New creates a registry using agent-store as the source of truth.
func New(client *anthropic.Client, logsDir string) (*Registry, error) {
	cfg, err := LoadFromAgentStore("")
	if err != nil {
		return nil, fmt.Errorf("load from agent-store: %w", err)
	}

	r := &Registry{
		client:       client,
		logsDir:      logsDir,
		default_:     cfg.Default,
		configs:      cfg.Agents,
		agents:       make(map[string]*agent.Agent),
		sessions:     make(map[string]*session.Session),
		tools:        NewToolRegistry(),
	}

	// Apply OpenClaw configuration if present
	if cfg.OpenClaw != nil {
		r.openclawURL = cfg.OpenClaw.URL
		r.openclawToken = cfg.OpenClaw.Token
		r.openclawAgents = cfg.OpenClaw.Agents
	}

	// Register spawn_agent tool (requires registry reference)
	r.tools.RegisterSpawnTool(r.SpawnAgentTool())

	return r, nil
}


// List returns the names of all configured agents
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.configs))
	for name := range r.configs {
		names = append(names, name)
	}
	return names
}

// Default returns the default agent name (if configured)
func (r *Registry) Default() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.default_
}

// SetMemoryStore registers memory tools with the given memory store
func (r *Registry) SetMemoryStore(store memory.MemoryStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools.RegisterMemoryTools(store)
}

// SetModelClient sets the unified model client (Anthropic or OpenAI)
func (r *Registry) SetModelClient(mc *agent.ModelClient) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelClient = mc
}

// SetModelStore sets the model store for creating per-agent model clients
func (r *Registry) SetModelStore(store *modelstore.Store) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelStore = store
}


// SetOpenClawConfig configures OpenClaw gateway for sub-agent delegation
func (r *Registry) SetOpenClawConfig(url, token string, agents []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.openclawURL = url
	r.openclawToken = token
	r.openclawAgents = agents
}

// GetConfig returns the config for the named agent (case-insensitive lookup)
func (r *Registry) GetConfig(name string) (*AgentConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cfg, ok := r.configs[name]
	if ok {
		return cfg, nil
	}
	// Case-insensitive fallback
	lower := strings.ToLower(name)
	for k, v := range r.configs {
		if strings.ToLower(k) == lower {
			return v, nil
		}
	}
	return nil, fmt.Errorf("agent %q not found", name)
}

// Get returns an existing agent instance or creates one if not exists
func (r *Registry) Get(name string) (*agent.Agent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Return existing agent
	if a, ok := r.agents[name]; ok {
		return a, nil
	}

	// Create new agent
	cfg, ok := r.configs[name]
	if !ok {
		return nil, fmt.Errorf("agent %q not found", name)
	}

	a, err := r.createAgent(cfg)
	if err != nil {
		return nil, err
	}

	r.agents[name] = a
	return a, nil
}

// GetSession returns the session for the named agent (creates if needed)
func (r *Registry) GetSession(name string) (*session.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Return existing session
	if sess, ok := r.sessions[name]; ok {
		return sess, nil
	}

	// Create new session
	cfg, ok := r.configs[name]
	if !ok {
		return nil, fmt.Errorf("agent %q not found", name)
	}

	sess, err := session.New(r.logsDir, cfg.Model, name, "", r.modelStore)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	r.sessions[name] = sess
	return sess, nil
}

// CloseSession closes and removes the session for the named agent
func (r *Registry) CloseSession(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sess, ok := r.sessions[name]; ok {
		sess.Close()
		delete(r.sessions, name)
	}
}

// CloseAll closes all sessions
func (r *Registry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, sess := range r.sessions {
		sess.Close()
	}
	r.sessions = make(map[string]*session.Session)
}

// createAgent creates an agent instance from config
func (r *Registry) createAgent(cfg *AgentConfig) (*agent.Agent, error) {
	provider := agent.NewAnthropicProvider(r.client)
	a := agent.New(provider, cfg.System)

	// Propagate OAuth flag so the agent adds Claude Code identity to system prompt
	if r.modelClient != nil && r.modelClient.IsOAuth {
		a.SetOAuth(true)
	}

	// Set thinking budget if specified
	if cfg.Thinking > 0 {
		a.SetThinking(cfg.Thinking)
	}

	// Register tools
	for _, toolName := range cfg.Tools {
		tool, err := r.tools.Get(toolName)
		if err != nil {
			return nil, fmt.Errorf("get tool %q: %w", toolName, err)
		}
		a.AddTool(tool)
	}

	return a, nil
}
