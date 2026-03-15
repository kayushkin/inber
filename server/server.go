package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kayushkin/forge"
	modelstore "github.com/kayushkin/model-store"
	sessionMod "github.com/kayushkin/inber/session"
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

// Server manages agent sessions, routing, and sub-agent orchestration.
type Server struct {
	config     Config
	sessions   sync.Map          // sessionKey → *Session
	queue      *Queue
	store      *Store            // session/request persistence
	events     *EventPublisher   // bus event publisher (nil = disabled)
	modelStore *modelstore.Store
	forgeDB    *forge.Forge                // workspace management
	workspaces map[string]*forge.Workspace // active workspaces by ID
	mu         sync.RWMutex
}

// New creates a server.
func New(cfg Config) (*Server, error) {
	// Apply defaults.
	if cfg.MainConcurrency <= 0 {
		cfg.MainConcurrency = 4
	}
	if cfg.SubagentConcurrency <= 0 {
		cfg.SubagentConcurrency = 8
	}
	if cfg.MaxSpawnDepth <= 0 {
		cfg.MaxSpawnDepth = 2
	}
	if cfg.MaxChildrenPerAgent <= 0 {
		cfg.MaxChildrenPerAgent = 5
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8200"
	}
	if cfg.DataDir == "" {
		home, _ := os.UserHomeDir()
		// Use legacy dir if it exists, otherwise new name
		legacyDir := filepath.Join(home, ".inber", "gateway")
		if fileExists(legacyDir) {
			cfg.DataDir = legacyDir
		} else {
			cfg.DataDir = filepath.Join(home, ".inber", "server")
		}
	}
	if cfg.DefaultAgent == "" && len(cfg.Agents) > 0 {
		// Pick first agent as default.
		for name := range cfg.Agents {
			cfg.DefaultAgent = name
			break
		}
	}

	os.MkdirAll(cfg.DataDir, 0755)

	// Open shared model store.
	ms, err := modelstore.Open("")
	if err != nil {
		log.Printf("[server] warning: model store unavailable: %v", err)
	}

	// Open server store.
	// Check for legacy gateway.db first, fall back to server.db
	dbPath := filepath.Join(cfg.DataDir, "server.db")
	if legacyPath := filepath.Join(cfg.DataDir, "gateway.db"); fileExists(legacyPath) {
		dbPath = legacyPath // use existing data
	}
	store, err := NewStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open server store: %w", err)
	}

	// Mark any previously-running requests as interrupted.
	if n, err := store.InterruptRunning(); err != nil {
		log.Printf("[server] warning: failed to interrupt running requests: %v", err)
	} else if n > 0 {
		log.Printf("[server] marked %d interrupted requests from previous run", n)
	}

	q := NewQueue(map[string]int{
		"main":     cfg.MainConcurrency,
		"subagent": cfg.SubagentConcurrency,
	})

	events := NewEventPublisher(cfg.BusURL, cfg.BusToken)

	// Open forge DB for workspace management.
	var forgeDB *forge.Forge
	home, _ := os.UserHomeDir()
	forgePath := filepath.Join(home, ".config", "forge", "forge.db")
	if _, err := os.Stat(forgePath); err == nil {
		if f, err := forge.Open(forgePath); err != nil {
			log.Printf("[server] warning: forge unavailable: %v", err)
		} else {
			forgeDB = f
			log.Printf("[server] forge DB opened")
		}
	}

	return &Server{
		config:     cfg,
		queue:      q,
		store:      store,
		events:     events,
		modelStore: ms,
		forgeDB:    forgeDB,
		workspaces: make(map[string]*forge.Workspace),
	}, nil
}

// Close shuts down all sessions and releases resources.
func (g *Server) Close() error {
	g.sessions.Range(func(key, val any) bool {
		s := val.(*Session)
		s.close()
		return true
	})
	if g.store != nil {
		g.store.Close()
	}
	if g.modelStore != nil {
		g.modelStore.Close()
	}
	if g.forgeDB != nil {
		g.forgeDB.Close()
	}
	return nil
}

// Route resolves which agent handles a message.
// For now: returns the default agent. Routing rules can be added later.
func (g *Server) Route(channel, author string) string {
	return g.config.DefaultAgent
}

// AgentConfig returns config for a named agent.
func (g *Server) GetAgentConfig(name string) (AgentConfig, bool) {
	ac, ok := g.config.Agents[name]
	return ac, ok
}

// ---------------------------------------------------------------------------
// Running turns
// ---------------------------------------------------------------------------

// RunRequest is an inbound message to process.
type RunRequest struct {
	Agent      string `json:"agent"`
	Message    string `json:"message"`
	SessionKey string `json:"session_key"`
	Channel    string `json:"channel"`
	Author     string `json:"author"`
}

// RunResponse is the result of a turn.
type RunResponse struct {
	Text       string        `json:"text"`
	SessionKey string        `json:"session_key"`
	Tokens     TokenUsage    `json:"tokens"`
	Duration   time.Duration `json:"duration_ms"`
}

// TokenUsage tracks token consumption.
type TokenUsage struct {
	Input      int     `json:"input"`
	Output     int     `json:"output"`
	CacheRead  int     `json:"cache_read"`
	CacheWrite int     `json:"cache_write"`
	Cost       float64 `json:"cost"`
}

// StreamEvent is emitted during streaming.
type StreamEvent struct {
	Kind string `json:"kind"` // "delta", "thinking", "tool_call", "tool_result", "done"
	Text string `json:"text,omitempty"`
	Tool string `json:"tool,omitempty"`
	Data any    `json:"data,omitempty"`
}

// Run sends a message to an agent session. Creates the session if needed.
// Blocks until the turn completes.
func (g *Server) Run(ctx context.Context, req RunRequest) (*RunResponse, error) {
	return g.run(ctx, req, nil)
}

// Stream is like Run but calls onEvent for real-time output.
func (g *Server) Stream(ctx context.Context, req RunRequest, onEvent func(StreamEvent)) error {
	_, err := g.run(ctx, req, onEvent)
	return err
}

func (g *Server) run(ctx context.Context, req RunRequest, onEvent func(StreamEvent)) (*RunResponse, error) {
	// Resolve agent.
	agentName := req.Agent
	if agentName == "" {
		agentName = g.Route(req.Channel, req.Author)
	}
	ac, ok := g.GetAgentConfig(agentName)
	if !ok {
		return nil, fmt.Errorf("unknown agent: %s", agentName)
	}

	// Resolve session key.
	sessionKey := req.SessionKey
	if sessionKey == "" {
		sessionKey = fmt.Sprintf("agent:%s:main", agentName)
	}

	// Prepare input (prefix with author if present).
	input := req.Message
	if req.Author != "" {
		input = fmt.Sprintf("[%s] %s", req.Author, input)
	}

	// Check if session is already running — inject instead of queuing.
	if val, ok := g.sessions.Load(sessionKey); ok {
		sess := val.(*Session)
		sess.mu.Lock()
		isRunning := sess.Status == Running
		sess.mu.Unlock()

		if isRunning {
			log.Printf("[server] session %s busy, injecting message mid-turn", sessionKey)
			sess.inject(input)
			return &RunResponse{
				Text:       "[Message injected into running session — agent will see it during current work]",
				SessionKey: sessionKey,
			}, nil
		}
	}

	// Ensure session exists in DB.
	g.store.UpsertSession(sessionKey, agentName, "main")

	var resp *RunResponse

	// Enqueue the work (serialized by session, capped by lane).
	err := g.queue.Enqueue(ctx, "main", sessionKey, func(ctx context.Context) error {
		sess, err := g.getOrCreateSession(sessionKey, agentName, ac, onEvent)
		if err != nil {
			return fmt.Errorf("session %s: %w", sessionKey, err)
		}

		// Track request in DB.
		reqID, _ := g.store.CreateRequest(sessionKey, truncate(input, 500), nil)

		start := time.Now()
		result, err := sess.turn(ctx, input)
		if err != nil {
			g.store.CompleteRequest(reqID, "error", "", err.Error(), 0, 0, 0, 0, 0, 0)
			return err
		}

		tokens := TokenUsage{
			Input:      result.InputTokens,
			Output:     result.OutputTokens,
			CacheRead:  result.CacheReadTokens,
			CacheWrite: result.CacheCreationTokens,
		}

		cost := sessionMod.CalcCostWithCache("", tokens.Input, tokens.Output, tokens.CacheRead, tokens.CacheWrite)
		g.store.CompleteRequest(reqID, "completed", truncate(result.Text, 1000), "",
			result.ToolCalls, tokens.Input, tokens.Output, tokens.CacheRead, tokens.CacheWrite, cost)
		g.store.TouchSession(sessionKey, len(sess.Engine.Messages))

		// Persist messages.
		g.persistMessages(sess)

		if onEvent != nil {
			onEvent(StreamEvent{
				Kind: "done",
				Text: result.Text,
				Data: map[string]any{
					"tokens":      tokens,
					"duration_ms": time.Since(start).Milliseconds(),
				},
			})
		}

		resp = &RunResponse{
			Text:       result.Text,
			SessionKey: sessionKey,
			Tokens:     tokens,
			Duration:   time.Duration(time.Since(start).Milliseconds()),
		}
		return nil
	})

	return resp, err
}

// Inject sends a message into a session.
// If the session is running, injects mid-turn (agent sees it between tool calls).
// If idle, queues as pending (delivered as prefix on next turn).
func (g *Server) Inject(sessionKey, message string) error {
	val, ok := g.sessions.Load(sessionKey)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionKey)
	}
	s := val.(*Session)

	s.mu.Lock()
	isRunning := s.Status == Running
	s.mu.Unlock()

	if isRunning {
		s.inject(message)
	} else {
		s.queuePending(message)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Session persistence
// ---------------------------------------------------------------------------

func (g *Server) persistMessages(s *Session) {
	s.mu.Lock()
	msgs := s.Engine.Messages
	s.mu.Unlock()

	dir := filepath.Join(g.config.DataDir, "sessions", s.Key)
	os.MkdirAll(dir, 0755)

	data, err := json.Marshal(msgs)
	if err != nil {
		log.Printf("[server] persist %s: %v", s.Key, err)
		return
	}
	os.WriteFile(filepath.Join(dir, "messages.json"), data, 0644)
}

// ---------------------------------------------------------------------------
// Session listing
// ---------------------------------------------------------------------------

// SessionInfo is a summary of a session for listing.
type SessionInfo struct {
	Key        string        `json:"key"`
	Agent      string        `json:"agent"`
	Status     SessionStatus `json:"status"`
	SpawnDepth int           `json:"spawn_depth"`
	ParentKey  string        `json:"parent_key,omitempty"`
	Children   []string      `json:"children,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	LastActive time.Time     `json:"last_active"`
	Messages   int           `json:"messages"`
}

// ListSessions returns info about all sessions.
func (g *Server) ListSessions() []*SessionInfo {
	var result []*SessionInfo
	g.sessions.Range(func(key, val any) bool {
		s := val.(*Session)
		s.mu.Lock()
		info := &SessionInfo{
			Key:        s.Key,
			Agent:      s.AgentName,
			Status:     s.Status,
			SpawnDepth: s.SpawnDepth,
			ParentKey:  s.ParentKey,
			Children:   s.Children,
			CreatedAt:  s.CreatedAt,
			LastActive: s.LastActive,
			Messages:   len(s.Engine.Messages),
		}
		s.mu.Unlock()
		result = append(result, info)
		return true
	})
	return result
}

// StopSession aborts a running session and cascades to children.
func (g *Server) StopSession(key string) error {
	val, ok := g.sessions.Load(key)
	if !ok {
		return fmt.Errorf("session not found: %s", key)
	}
	s := val.(*Session)

	// Cascade to children first.
	s.mu.Lock()
	children := append([]string{}, s.Children...)
	s.mu.Unlock()

	for _, childKey := range children {
		g.StopSession(childKey)
	}

	s.stop()
	return nil
}

// ---------------------------------------------------------------------------
// Config loading
// ---------------------------------------------------------------------------

// LoadConfig loads server config from a JSON file.
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
