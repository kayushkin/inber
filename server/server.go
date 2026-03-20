// Package server provides the HTTP API server for inber, exposing endpoints
// for running agents, spawning isolated sessions, managing models, and
// interfacing with the message bus for distributed operation.
package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/bus"
	"github.com/kayushkin/inber/logger"
	modelstore "github.com/kayushkin/model-store"
	sessionMod "github.com/kayushkin/inber/session"
)

// Config and AgentConfig types moved to config.go

// Server manages agent sessions, routing, and sub-agent orchestration.
type Server struct {
	config     Config
	sessions   sync.Map          // sessionKey → *Session
	queue      *Queue
	store      *Store            // session/request persistence
	events     *EventPublisher   // bus event publisher (nil = disabled)
	bus        *bus.Client       // bus subscription client (nil = API-only mode)
	busManager *BusManager       // bus message handling
	modelStore *modelstore.Store
	forgeDB    WorkspaceManager             // workspace management
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
		logger.WithComponent("server").Warn("model store unavailable", map[string]interface{}{
			"error": err,
		})
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
		logger.WithComponent("server").Warn("failed to interrupt running requests", map[string]interface{}{
			"error": err,
		})
	} else if n > 0 {
		logger.WithComponent("server").Info("marked interrupted requests from previous run", map[string]interface{}{
			"count": n,
		})
	}

	q := NewQueue(map[string]int{
		"main":     cfg.MainConcurrency,
		"subagent": cfg.SubagentConcurrency,
	})

	events := NewEventPublisher(cfg.NatsURL, cfg.BusToken)

	// Open forge DB for workspace management.
	var forgeDB *forge.Forge
	home, _ := os.UserHomeDir()
	forgePath := filepath.Join(home, ".config", "forge", "forge.db")
	if _, err := os.Stat(forgePath); err == nil {
		if f, err := forge.Open(forgePath); err != nil {
			logger.WithComponent("server").Warn("forge unavailable", map[string]interface{}{
				"error": err,
				"path":  forgePath,
			})
		} else {
			forgeDB = f
			logger.WithComponent("server").Info("forge DB opened", map[string]interface{}{
				"path": forgePath,
			})
		}
	}

	// Create bus client for inbound subscription + outbound publishing.
	busClient := bus.NewClient(cfg.NatsURL, "inber-server")

	server := &Server{
		config:     cfg,
		queue:      q,
		store:      store,
		events:     events,
		bus:        busClient,
		modelStore: ms,
		forgeDB:    forgeDB,
		workspaces: make(map[string]*forge.Workspace),
	}

	// Create bus manager for handling bus messages.
	server.busManager = NewBusManager(server)

	return server, nil
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

// ListenBus subscribes to the bus and processes inbound messages.
// Delegates to the BusManager for actual implementation.
func (g *Server) ListenBus(ctx context.Context) error {
	return g.busManager.ListenBus(ctx)
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
	Text       string `json:"text,omitempty"` // alias for message (backward compat with bus-agent)
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
			logger.WithComponent("server").Debug("session busy, injecting message mid-turn", map[string]interface{}{
				"session_key": sessionKey,
			})
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

// Inject moved to session_management.go

// ---------------------------------------------------------------------------
// Session persistence moved to session_management.go
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Session listing moved to session_management.go
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Config loading moved to config.go

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
