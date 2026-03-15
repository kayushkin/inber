// Package gateway orchestrates agent sessions, routing, spawning, and concurrency.
// It is the entry point for all external interactions with inber agents.
//
// Start with: inber serve
//
// This file is a design reference — not compilable code yet.
package gateway

import (
	"context"
	"sync"
	"time"

	"github.com/kayushkin/inber/engine"
	modelstore "github.com/kayushkin/model-store"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// Config defines the gateway's runtime configuration.
type Config struct {
	// Agent definitions: name → config.
	// Each agent has its own workspace, model, tools, and identity.
	Agents map[string]AgentConfig

	// Default agent for unrouted messages.
	DefaultAgent string

	// Queue concurrency limits per lane.
	MainConcurrency     int // inbound messages (default 4)
	SubagentConcurrency int // spawned sub-agents (default 8)

	// Sub-agent limits.
	MaxSpawnDepth       int // max nesting: main→sub→sub (default 2)
	MaxChildrenPerAgent int // max active children per session (default 5)

	// API server.
	ListenAddr string // default ":8200"

	// Optional: subscribe directly to bus (skip bus-agent).
	BusURL   string
	BusToken string
}

// AgentConfig defines one agent's configuration.
type AgentConfig struct {
	Name      string   // agent name (e.g. "claxon", "ogma")
	Workspace string   // repo root / cwd for this agent
	Model     string   // default model (e.g. "claude-opus-4-6")
	Thinking  int64    // thinking budget (0 = off)
	Tools     []string // tool allowlist (empty = all tools)
	Identity  string   // path to identity/soul files, or inline system prompt
}

// ---------------------------------------------------------------------------
// Gateway
// ---------------------------------------------------------------------------

// Gateway manages agent sessions, routing, and sub-agent orchestration.
type Gateway struct {
	config     Config
	sessions   sync.Map              // sessionKey → *Session
	queue      *Queue                // lane-based work queue
	modelStore *modelstore.Store     // shared across all engines
	agents     map[string]AgentConfig // agent registry
}

// New creates a gateway from config.
func New(cfg Config) (*Gateway, error)

// Serve starts the HTTP/WebSocket API server. Blocks until ctx is cancelled.
func (g *Gateway) Serve(ctx context.Context) error

// Close shuts down all sessions and releases resources.
func (g *Gateway) Close() error

// ---------------------------------------------------------------------------
// Routing
// ---------------------------------------------------------------------------

// Route resolves which agent handles a message based on channel and metadata.
func (g *Gateway) Route(channel, author string) string // returns agent name

// AgentConfig returns the config for a named agent.
func (g *Gateway) AgentConfig(name string) (AgentConfig, bool)

// ---------------------------------------------------------------------------
// Running turns
// ---------------------------------------------------------------------------

// RunRequest is an inbound message to process.
type RunRequest struct {
	Agent      string // agent name (resolved by Route, or explicit)
	Message    string // user message text
	SessionKey string // session to continue (empty = auto-assign main session)
	Channel    string // source channel (e.g. "websocket", "telegram")
	Author     string // who sent the message
}

// RunResponse is the result of processing a message.
type RunResponse struct {
	Text      string     // agent's response text
	SessionKey string    // session that handled it
	Tokens    TokenUsage // token consumption
	Duration  time.Duration
}

// Run sends a message to an agent session. Creates the session if needed.
// Blocks until the turn completes.
func (g *Gateway) Run(ctx context.Context, req RunRequest) (*RunResponse, error)

// Stream is like Run but calls onEvent for real-time streaming.
func (g *Gateway) Stream(ctx context.Context, req RunRequest, onEvent func(StreamEvent)) error

// StreamEvent is emitted during a streaming run.
type StreamEvent struct {
	Kind string // "delta", "thinking", "tool_call", "tool_result", "done"
	Text string // text content (delta, thinking, final response)
	Tool string // tool name (for tool_call/tool_result)
	Data any    // structured data (tool input/output, final tokens)
}

// Inject sends a message into a running session mid-turn.
// If the session is idle, it queues for the next turn.
func (g *Gateway) Inject(sessionKey string, message string) error

// ---------------------------------------------------------------------------
// Session lifecycle
// ---------------------------------------------------------------------------

// Session represents one ongoing conversation with an agent.
type Session struct {
	Key         string
	AgentName   string
	Engine      *engine.Engine
	Status      SessionStatus // Idle, Running, Completed, Error
	SpawnDepth  int           // 0 = root, 1 = sub-agent, 2 = sub-sub-agent
	ParentKey   string        // empty for root sessions
	Children    []string      // child session keys
	CreatedAt   time.Time
	LastActive  time.Time
	mu          sync.Mutex
	cancel      context.CancelFunc
}

type SessionStatus int

const (
	Idle SessionStatus = iota
	Running
	Completed
	Error
)

// GetSession returns a session by key, or nil.
func (g *Gateway) GetSession(key string) *Session

// ListSessions returns all sessions matching the filter.
func (g *Gateway) ListSessions(filter SessionFilter) []*SessionInfo

// StopSession aborts a running session and cascades to children.
func (g *Gateway) StopSession(key string) error

// ---------------------------------------------------------------------------
// Session creation and persistence
// ---------------------------------------------------------------------------

// getOrCreateSession returns an existing session or creates one with a fresh engine.
func (g *Gateway) getOrCreateSession(key string, agent AgentConfig) (*Session, error)

// createEngine builds an engine.Engine configured for this agent.
func (g *Gateway) createEngine(agent AgentConfig, opts engineOpts) (*engine.Engine, error)

// persistMessages saves a session's messages to disk for restart recovery.
func (g *Gateway) persistMessages(s *Session) error

// loadMessages restores a session's messages from disk.
func (g *Gateway) loadMessages(key string) ([]any, error) // returns anthropic message params

// repairMessages fixes interrupted sessions (dangling tool_use, alternation).
func (g *Gateway) repairMessages(messages []any) []any

// ---------------------------------------------------------------------------
// Spawning
// ---------------------------------------------------------------------------

// SpawnRequest is the input for creating a sub-agent.
type SpawnRequest struct {
	ParentKey string // parent session key
	Agent     string // target agent name
	Task      string // task description
	Model     string // model override (empty = agent default)
	Fork      bool   // if true, child inherits parent's conversation history
}

// SpawnResponse is returned immediately when a sub-agent is accepted.
type SpawnResponse struct {
	Status   string // "accepted"
	ChildKey string // child session key
}

// Spawn creates a child session and enqueues its work.
// Returns immediately. Result delivered to parent async via deliverResult.
func (g *Gateway) Spawn(ctx context.Context, req SpawnRequest) (*SpawnResponse, error)

// ForkAndSpawn forks the parent session N times, one per task.
// All children start with the same conversation history.
func (g *Gateway) ForkAndSpawn(ctx context.Context, parentKey string, tasks []SpawnRequest) ([]*SpawnResponse, error)

// SpawnResult is delivered to the parent when a child completes.
type SpawnResult struct {
	ChildKey string
	Agent    string
	Task     string
	Status   string        // "success", "error", "timeout"
	Summary  string        // child's final response
	Tokens   TokenUsage
	Duration time.Duration
	Error    string
}

// deliverResult injects the child's result into the parent session.
func (g *Gateway) deliverResult(parentKey string, result SpawnResult)

// ---------------------------------------------------------------------------
// Forking
// ---------------------------------------------------------------------------

// forkSession creates a child session with a deep copy of the parent's messages.
// The child gets its own engine but starts with the parent's conversation state.
func (g *Gateway) forkSession(parent *Session, childKey string, agent AgentConfig) (*Session, error)

// ---------------------------------------------------------------------------
// Queue
// ---------------------------------------------------------------------------

// Queue manages lane-based concurrent work with per-session serialization.
type Queue struct {
	lanes    map[string]*lane
	sessions sync.Map // sessionKey → per-session mutex
}

// Enqueue runs work in the specified lane.
// Same session key = serialized. Different sessions = parallel up to lane cap.
func (q *Queue) Enqueue(ctx context.Context, lane string, sessionKey string, work func(ctx context.Context) error) error

// ---------------------------------------------------------------------------
// Token usage
// ---------------------------------------------------------------------------

type TokenUsage struct {
	Input      int
	Output     int
	CacheRead  int
	CacheWrite int
	Cost       float64
}

// ---------------------------------------------------------------------------
// API endpoints (served by Serve)
// ---------------------------------------------------------------------------
//
// POST   /api/run              — Run(RunRequest)
// POST   /api/spawn            — Spawn(SpawnRequest)
// POST   /api/fork-spawn       — ForkAndSpawn
// GET    /api/sessions          — ListSessions
// GET    /api/sessions/:key     — GetSession
// POST   /api/sessions/:key/inject — Inject
// DELETE /api/sessions/:key     — StopSession
// GET    /api/models            — model health dashboard
// POST   /api/models/test       — test a model
// WS     /ws/stream             — real-time streaming
