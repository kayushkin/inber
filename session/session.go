// Package session provides conversation logging as JSONL files.
// Each session gets a timestamped file in the logs directory.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	modelstore "github.com/kayushkin/model-store"
)

// Entry is a single log line in the session JSONL file.
type Entry struct {
	Timestamp    time.Time       `json:"ts"`
	Turn         int             `json:"turn,omitempty"`          // API round-trip number (increments on each request)
	Role         string          `json:"role"`                    // "user", "assistant", "tool_call", "tool_result", "system", "request"
	Content      string          `json:"content,omitempty"`       // text content
	Model        string          `json:"model,omitempty"`         // model used (assistant entries)
	ToolName     string          `json:"tool_name,omitempty"`     // for tool_call / tool_result
	ToolID       string          `json:"tool_id,omitempty"`       // tool use ID
	ToolInput    json.RawMessage `json:"tool_input,omitempty"`    // raw JSON input for tool_call
	IsError      bool            `json:"is_error,omitempty"`      // tool_result was an error
	InputTokens  int             `json:"in_tokens,omitempty"`     // cumulative for this turn
	OutputTokens int             `json:"out_tokens,omitempty"`    // cumulative for this turn
	CacheRead    int             `json:"cache_read_tokens,omitempty"`  // cumulative for this turn
	CacheWrite   int             `json:"cache_write_tokens,omitempty"` // cumulative for this turn
	TotalCost    float64         `json:"cost_usd,omitempty"`      // cumulative session cost
	Request      json.RawMessage `json:"request,omitempty"`       // full API request payload
}

// TurnTokens is one turn's usage as the provider reported it. The four counts
// are disjoint — Input is the part of the prompt the cache did not cover — so
// the prompt is their sum and no one of them can stand in for the whole.
//
// They travel together as a value because they were being carried apart. Every
// producer here had all four and every consumer needed all four, but the two
// logging entry points took only Input and Output, so the cache counts were
// dropped at the doorway and the session's own dollar figure was left pricing a
// twentieth of the prompt it had just sent. A struct is what stops the next
// count added upstream from being lost the same way.
type TurnTokens struct {
	Input      int
	Output     int
	CacheRead  int
	CacheWrite int
}

// toLogstackEntry moved to logstack.go

// Session tracks and logs a conversation.
type Session struct {
	mu               sync.Mutex
	file             *os.File
	enc              *json.Encoder
	start            time.Time
	model            string
	agentName        string          // agent name for multi-agent support
	parentID         string          // parent session ID (empty for root)
	sessionID        string          // unique session ID
	turn             int             // current API round-trip number
	totalIn          int
	totalOut         int
	totalCacheRead   int
	totalCacheWrite  int
	store            Store           // session tracking store (nil if unavailable)
	modelStore       *modelstore.Store // model store for cost calculation (nil if unavailable)
	truncateCfg      TruncateConfig  // truncation config for tool results
	logstack         *LogstackAdapter // optional logstack adapter for centralized logging
	prevMessageCount int             // tracks message count for prompt breakdown diffs
}

// shortID generates a 4-character hex random string for session uniqueness.
func shortID() string {
	b := make([]byte, 2)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// New creates a session logger. It owns the layout under logsDir and writes to
// logsDir/agentName/YYYY-MM-DD_HHMMSS_xxxx[-sub]/session.jsonl, creating every
// directory on that path.
//
// logsDir is therefore the root shared by every agent, NOT this agent's own
// directory: New appends the agent segment itself, so a caller that appends it
// too gets logs/<agent>/<agent>/. Pass <repo root>/logs.
//
// agentName identifies the agent (for multi-agent support).
// parentID is the parent session ID (empty string for root sessions).
// modelStore provides model cost information (can be nil).
func New(logsDir, model, agentName, parentID string, modelStore *modelstore.Store) (*Session, error) {
	// Create agent-specific subdirectory
	agentDir := logsDir
	if agentName != "" {
		agentDir = filepath.Join(logsDir, agentName)
	}
	
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return nil, fmt.Errorf("create logs dir: %w", err)
	}

	now := time.Now()
	sessionID := now.Format("2006-01-02_150405") + "_" + shortID()

	// Add suffix for sub-agent sessions
	if parentID != "" {
		sessionID += "-sub"
	}

	sessionDir := filepath.Join(agentDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	filename := filepath.Join(sessionDir, "session.jsonl")
	f, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	s := &Session{
		file:         f,
		enc:          json.NewEncoder(f),
		start:        now,
		model:        model,
		agentName:    agentName,
		parentID:     parentID,
		sessionID:    sessionID,
		modelStore:   modelStore,
		truncateCfg:  DefaultTruncateConfig(),
	}

	// Initialize logstack adapter if URL is configured
	if url := os.Getenv("LOGSTACK_URL"); url != "" {
		s.logstack = NewLogstackAdapter(url, agentName, s.sessionID)
	}

	// Log session start
	msg := fmt.Sprintf("session started — model: %s", model)
	if agentName != "" {
		msg += fmt.Sprintf(" — agent: %s", agentName)
	}
	if parentID != "" {
		msg += fmt.Sprintf(" — parent: %s", parentID)
	}
	
	s.write(Entry{
		Timestamp: now,
		Role:      "system",
		Content:   msg,
	})

	return s, nil
}

// SessionID returns the unique session ID
func (s *Session) SessionID() string {
	return s.sessionID
}

// AgentName returns the agent name for this session
func (s *Session) AgentName() string {
	return s.agentName
}

// AttachStore attaches a session tracking store and registers this session.
// Store returns the attached session store, or nil.
func (s *Session) Store() Store {
	return s.store
}

// AttachDB is a deprecated alias for AttachStore that accepts a *DB (which is an alias for *SQLiteStore).
// Deprecated: Use AttachStore with a Store interface instead.
func (s *Session) AttachDB(db *DB, command string) {
	s.AttachStore(db, command)
}

// DB returns the attached session store cast as *DB for backwards compatibility.
// Returns nil if no store is attached or if the store is not a *SQLiteStore.
// Deprecated: Use Store() instead.
func (s *Session) DB() *DB {
	if store, ok := s.store.(*SQLiteStore); ok {
		return store
	}
	return nil
}

func (s *Session) AttachStore(store Store, command string) {
	s.store = store
	if store != nil {
		store.InsertSession(&SessionRow{
			ID:        s.sessionID,
			Agent:     s.agentName,
			Model:     s.model,
			Command:   command,
			ParentID:  s.parentID,
			PID:       os.Getpid(),
			StartedAt: s.start,
			LogFile:   s.FilePath(),
		})
	}
}

// SetTruncateConfig updates the truncation configuration.
func (s *Session) SetTruncateConfig(cfg TruncateConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.truncateCfg = cfg
}

// CurrentTurn returns the current turn number (0 before first request).
func (s *Session) CurrentTurn() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turn
}













// Close finalizes the session log and marks it completed in the DB.
func (s *Session) Close() {
	s.closeJSONLWithCompletion()

	if s.store != nil {
		s.store.EndSession(s.sessionID, "completed", "")
	}
}

// CloseWithError finalizes the session log and marks it as errored in the DB.
func (s *Session) CloseWithError(err error) {
	s.closeJSONLWithError(err)

	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	if s.store != nil {
		s.store.EndSession(s.sessionID, "error", errMsg)
	}
}

// FilePath returns the path to the log file.
func (s *Session) FilePath() string {
	return s.file.Name()
}

func (s *Session) write(e Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enc.Encode(e) // best-effort; don't crash on log failure

	// Send copy to logstack if available (async, best-effort)
	s.logstack.Log(e)
}

// cost calculates the total session cost in USD based on token usage.
//
// It prices through CalcCostWithCache rather than multiplying out the input and
// output rates here. This function and its per-turn twin were a second
// implementation of the package's own pricing, and the copy had no cache terms
// at all: it charged the uncached remainder of the prompt and left the cache
// reads and cache writes out entirely. On the traffic in the server's store
// that is 781k tokens counted against 18.5M ignored, and cache writes bill at
// 125% of the input rate, so the figure the session log reported and the figure
// the server reported for the same turns disagreed by roughly five times.
func (s *Session) cost() float64 {
	return CalcCostWithCache(s.model, s.totalIn, s.totalOut, s.totalCacheRead, s.totalCacheWrite, s.modelStore)
}

// Hooks returns agent.Hooks wired to this session's logging methods.
func (s *Session) Hooks() *agent.Hooks {
	return &agent.Hooks{
		OnRequest: func(params *anthropic.MessageNewParams) {
			if data, err := json.Marshal(params); err == nil {
				s.LogRequest(json.RawMessage(data))
			}
		},
		OnThinking: func(text string) {
			s.LogThinking(text)
		},
		OnToolCall: func(toolID, name string, input []byte) {
			s.LogToolCall(toolID, name, json.RawMessage(input))
		},
		OnToolResult: func(toolID, name, output string, isError bool) {
			s.LogToolResult(toolID, name, output, isError)
		},
	}
}
