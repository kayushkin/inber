package registry

import (
	"fmt"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/session"
)

// Registry manages multiple agents with isolated sessions and contexts
type Registry struct {
	mu       sync.RWMutex
	client   *anthropic.Client
	logsDir  string
	default_ string
	configs  map[string]*AgentConfig
	agents   map[string]*agent.Agent
	contexts map[string]*context.Store
	sessions map[string]*session.Session
	tools    *ToolRegistry
}

// New creates a registry and loads agent configs from the given directory
func New(client *anthropic.Client, configDir, logsDir string) (*Registry, error) {
	cfg, err := LoadConfigDir(configDir)
	if err != nil {
		return nil, fmt.Errorf("load configs: %w", err)
	}

	r := &Registry{
		client:   client,
		logsDir:  logsDir,
		default_: cfg.Default,
		configs:  cfg.Agents,
		agents:   make(map[string]*agent.Agent),
		contexts: make(map[string]*context.Store),
		sessions: make(map[string]*session.Session),
		tools:    NewToolRegistry(),
	}

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
func (r *Registry) SetMemoryStore(store *memory.Store) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools.RegisterMemoryTools(store)
}

// GetConfig returns the config for the named agent
func (r *Registry) GetConfig(name string) (*AgentConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cfg, ok := r.configs[name]
	if !ok {
		return nil, fmt.Errorf("agent %q not found", name)
	}
	return cfg, nil
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

// GetContext returns the context store for the named agent
func (r *Registry) GetContext(name string) (*context.Store, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Return existing context
	if ctx, ok := r.contexts[name]; ok {
		return ctx, nil
	}

	// Create new context store
	_, ok := r.configs[name]
	if !ok {
		return nil, fmt.Errorf("agent %q not found", name)
	}

	ctx := context.NewStore()
	r.contexts[name] = ctx
	return ctx, nil
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

	sess, err := session.New(r.logsDir, cfg.Model, name, "")
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
	a := agent.New(r.client, cfg.System)

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
