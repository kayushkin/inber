// Package server provides the HTTP API server for inber, exposing endpoints
// for running agents, spawning isolated sessions, managing models, and
// interfacing with the message bus for distributed operation.
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

	agentstore "github.com/kayushkin/agent-store"
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
	modelStore  *modelstore.Store
	agentStore  *agentstore.Store
	forgeDB     WorkspaceManager             // workspace management
	workspaces map[string]*forge.Workspace // active workspaces by ID
	mu         sync.RWMutex

	// Health tracking
	startedAt   time.Time
	lastError   string
	lastErrorAt time.Time
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
	// Credential management is handled by aiauth — model-store is metadata only.

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

	// Open agent-store for status queries.
	var as *agentstore.Store
	if a, err := agentstore.Open(""); err != nil {
		logger.WithComponent("server").Warn("agent-store unavailable", map[string]interface{}{
			"error": err,
		})
	} else {
		as = a
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
		agentStore: as,
		forgeDB:    forgeDB,
		workspaces: make(map[string]*forge.Workspace),
		startedAt:  time.Now(),
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
	if g.agentStore != nil {
		g.agentStore.Close()
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

	// Per-request overrides (thin CLI passes these through from flags).
	Model          string `json:"model,omitempty"`           // override agent's default model
	Thinking       int64  `json:"thinking,omitempty"`        // extended thinking token budget
	Raw            bool   `json:"raw,omitempty"`             // skip context/memory loading
	NoTools        bool   `json:"no_tools,omitempty"`        // disable all tools
	NoHooks        bool   `json:"no_hooks,omitempty"`        // skip post-request hooks
	System         string `json:"system,omitempty"`          // override system prompt
	NewSession     bool   `json:"new_session,omitempty"`     // start fresh session
	Detach         bool   `json:"detach,omitempty"`          // one-off session, don't persist
	MaxTurns       int     `json:"max_turns,omitempty"`        // safety limit: max API round-trips
	MaxInputTokens int     `json:"max_input_tokens,omitempty"` // safety limit: max cumulative input tokens
	MaxCost        float64 `json:"max_cost,omitempty"`         // safety limit: max dollar cost
	Mode           string  `json:"mode,omitempty"`             // execution mode: observe, assist, autonomous
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
	Turn int    `json:"turn,omitempty"` // API round-trip number from session
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
	if req.Detach {
		// One-off session: unique key, won't affect main session.
		sessionKey = fmt.Sprintf("agent:%s:detach-%d", agentName, time.Now().UnixMilli())
	} else if req.NewSession {
		// Fresh session: clear existing and use main key.
		sessionKey = fmt.Sprintf("agent:%s:main", agentName)
		g.sessions.Delete(sessionKey)
	} else if sessionKey == "" {
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
		sess, err := g.getOrCreateSession(ctx, sessionKey, agentName, ac, req, onEvent)
		if err != nil {
			return fmt.Errorf("session %s: %w", sessionKey, err)
		}

		// Track request in DB.
		reqID, _ := g.store.CreateRequest(sessionKey, truncate(input, 500), nil)

		start := time.Now()
		result, err := sess.turn(ctx, input)
		if err != nil {
			// The turn failed, but it may have produced text the user already
			// read and tokens that were already billed. Record both against the
			// request rather than writing an empty error row, and snapshot the
			// messages the engine kept.
			if result != nil {
				cost := sessionMod.CalcCostWithCache(sess.Engine.Model, result.InputTokens, result.OutputTokens,
					result.CacheReadTokens, result.CacheCreationTokens, g.modelStore)
				g.store.CompleteRequest(reqID, "error", truncate(result.Text, 1000), err.Error(),
					result.ToolCalls, result.InputTokens, result.OutputTokens,
					result.CacheReadTokens, result.CacheCreationTokens, cost)
				if result.Text != "" {
					g.store.TouchSession(sessionKey, len(sess.Engine.Messages))
					g.persistSessionState(sess)
				}
			} else {
				g.store.CompleteRequest(reqID, "error", "", err.Error(), 0, 0, 0, 0, 0, 0)
			}
			return err
		}

		tokens := TokenUsage{
			Input:      result.InputTokens,
			Output:     result.OutputTokens,
			CacheRead:  result.CacheReadTokens,
			CacheWrite: result.CacheCreationTokens,
		}

		// TokenUsage.Cost is what api_bridge.go reports to llm-bridge as the
		// turn's TotalUSD, and nothing ever assigned it, so every bridge
		// session billed $0.00 however much it spent. The cost belongs on the
		// tokens it prices; computing it into a local and leaving the field
		// zero is what let the two disagree.
		tokens.Cost = sessionMod.CalcCostWithCache(sess.Engine.Model, tokens.Input, tokens.Output,
			tokens.CacheRead, tokens.CacheWrite, g.modelStore)
		g.store.CompleteRequest(reqID, "completed", truncate(result.Text, 1000), "",
			result.ToolCalls, tokens.Input, tokens.Output, tokens.CacheRead, tokens.CacheWrite, tokens.Cost)
		g.store.TouchSession(sessionKey, len(sess.Engine.Messages))

		// Persist messages.
		g.persistSessionState(sess)

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

// ---------------------------------------------------------------------------
// Error logging
// ---------------------------------------------------------------------------

// logError writes structured error logs to logs/server-errors.jsonl
func (g *Server) logError(agent, session, input string, err error) {
	now := time.Now()

	g.mu.Lock()
	g.lastError = err.Error()
	g.lastErrorAt = now
	g.mu.Unlock()

	logEntry := map[string]interface{}{
		"ts":      now.Format("2006-01-02T15:04:05-07:00"),
		"level":   "error",
		"agent":   agent,
		"session": session,
		"input":   truncateInput(input, 200),
		"error":   err.Error(),
	}

	g.writeErrorLog(logEntry)
}

// logWarning writes structured warning logs to logs/server-errors.jsonl  
func (g *Server) logWarning(agent, session, input, message string) {
	logEntry := map[string]interface{}{
		"ts":      time.Now().Format("2006-01-02T15:04:05-07:00"),
		"level":   "warning",
		"agent":   agent, 
		"session": session,
		"input":   truncateInput(input, 200),
		"error":   message,
	}

	g.writeErrorLog(logEntry)
}

// writeErrorLog writes a log entry to logs/server-errors.jsonl
func (g *Server) writeErrorLog(logEntry map[string]interface{}) {
	// Create logs directory if it doesn't exist
	logsDir := "logs"
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		log.Printf("[server] failed to create logs directory: %v", err)
		return
	}

	// Open/create the error log file  
	logFile := filepath.Join(logsDir, "server-errors.jsonl")
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[server] failed to open error log file: %v", err)
		return
	}
	defer file.Close()

	// Write JSON line
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(logEntry); err != nil {
		log.Printf("[server] failed to write error log: %v", err)
	}
}

// truncateInput truncates input text to the specified length
func truncateInput(input string, maxLen int) string {
	if len(input) <= maxLen {
		return input
	}
	return input[:maxLen] + "..."
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DataDir returns the resolved data directory path.
func (g *Server) DataDir() string {
	return g.config.DataDir
}
