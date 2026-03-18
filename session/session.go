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
	"strings"
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
	TotalCost    float64         `json:"cost_usd,omitempty"`      // cumulative session cost
	Request      json.RawMessage `json:"request,omitempty"`       // full API request payload
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
	store            Store           // session tracking store (nil if unavailable)
	modelStore       *modelstore.Store // model store for cost calculation (nil if unavailable)
	truncateCfg      TruncateConfig  // truncation config for tool results
	truncateRefs     map[string]string // map of tool_id -> full output for references
	logstack         *LogstackAdapter // optional logstack adapter for centralized logging
	prevMessageCount int             // tracks message count for prompt breakdown diffs
}

// shortID generates a 4-character hex random string for session uniqueness.
func shortID() string {
	b := make([]byte, 2)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// New creates a session logger. Logs go to logsDir/agentName/YYYY-MM-DD_HHMMSS[-subN].jsonl.
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
		truncateRefs: make(map[string]string),
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

// GetFullToolResult retrieves the full (untruncated) output for a tool call.
// Returns empty string if tool result wasn't truncated or doesn't exist.
func (s *Session) GetFullToolResult(toolID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.truncateRefs[toolID]
}

// currentTurn returns the current turn number (0 before first request).
func (s *Session) currentTurn() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turn
}

// LogUser logs a user message.
func (s *Session) LogUser(text string) {
	turn := s.currentTurn()
	s.write(Entry{
		Timestamp: time.Now(),
		Turn:      turn,
		Role:      "user",
		Content:   text,
	})
	// Set task on first user message (turn is 0 before LogRequest increments it)
	if turn == 0 && s.store != nil {
		// Ignore error - this is a best-effort operation for task tracking
		s.store.SetTask(s.sessionID, text)
	}
}

// LogAssistant logs an assistant response with token counts.
func (s *Session) LogAssistant(text string, inTokens, outTokens, toolCalls int) {
	s.mu.Lock()
	s.totalIn += inTokens
	s.totalOut += outTokens
	cost := s.cost()
	turn := s.turn
	s.mu.Unlock()

	s.write(Entry{
		Timestamp:    time.Now(),
		Turn:         turn,
		Role:         "assistant",
		Content:      text,
		Model:        s.model,
		InputTokens:  inTokens,
		OutputTokens: outTokens,
		TotalCost:    cost,
	})
}

// EndTurn records turn completion in the DB (called after each API response).
func (s *Session) EndTurn(inTokens, outTokens, toolCalls int, stopReason, errMsg string) {
	s.mu.Lock()
	turn := s.turn
	s.mu.Unlock()
	
	if s.store != nil {
		// Calculate cost for just this turn
		info := agent.GetModelInfo(s.model, s.modelStore)
		cost := (float64(inTokens)*info.InputCostPer1M + float64(outTokens)*info.OutputCostPer1M) / 1_000_000
		s.store.EndTurn(s.sessionID, turn, inTokens, outTokens, toolCalls, cost, stopReason, errMsg)
	}
	
	// Append to timeline.md after each turn completes
	s.appendTimelineEntry(turn, inTokens, outTokens, toolCalls)
}

// LogToolCall logs a tool invocation by the assistant.
func (s *Session) LogToolCall(toolID, name string, input json.RawMessage) {
	s.write(Entry{
		Timestamp: time.Now(),
		Turn:      s.currentTurn(),
		Role:      "tool_call",
		ToolID:    toolID,
		ToolName:  name,
		ToolInput: input,
	})
}

// TruncateToolResult truncates a tool result according to session config.
// Returns truncated output, or empty string if no truncation needed.
func (s *Session) TruncateToolResult(name, output string, isError bool) string {
	s.mu.Lock()
	cfg := s.truncateCfg
	s.mu.Unlock()

	result := TruncateToolResult(name, output, cfg)
	if result.Truncated {
		return result.Displayed
	}
	return "" // No modification needed
}

// LogToolResult logs a tool result with automatic truncation.
func (s *Session) LogToolResult(toolID, name, output string, isError bool) {
	s.mu.Lock()
	cfg := s.truncateCfg
	s.mu.Unlock()

	// Truncate if needed
	result := TruncateToolResult(name, output, cfg)
	
	// Store full output as reference if truncated
	if result.Truncated {
		s.mu.Lock()
		s.truncateRefs[toolID] = output
		s.mu.Unlock()
	}

	s.write(Entry{
		Timestamp: time.Now(),
		Turn:      s.currentTurn(),
		Role:      "tool_result",
		ToolID:    toolID,
		ToolName:  name,
		Content:   result.Displayed,
		IsError:   isError,
	})
}

// LogThinking logs a thinking/reasoning block.
func (s *Session) LogThinking(text string) {
	s.write(Entry{
		Timestamp: time.Now(),
		Turn:      s.currentTurn(),
		Role:      "thinking",
		Content:   text,
	})
}

// LogRequest logs the full API request payload and advances the turn counter.
func (s *Session) LogRequest(payload json.RawMessage) {
	now := time.Now()
	s.mu.Lock()
	s.turn++
	turn := s.turn
	s.mu.Unlock()

	s.write(Entry{
		Timestamp: now,
		Turn:      turn,
		Role:      "request",
		Request:   payload,
	})

	if s.store != nil {
		s.store.InsertTurn(&TurnRow{
			SessionID: s.sessionID,
			Turn:      turn,
			StartedAt: now,
		})
	}
}

// LogCompaction logs a memory compaction event.
func (s *Session) LogCompaction(original []string, newID string, tags []string) {
	data := map[string]interface{}{
		"original_ids": original,
		"new_id":       newID,
		"tags":         tags,
	}
	raw, _ := json.Marshal(data)
	s.write(Entry{
		Timestamp: time.Now(),
		Role:      "compaction",
		Content:   fmt.Sprintf("compacted %d memories into %s", len(original), newID),
		Request:   json.RawMessage(raw),
	})
}

// LogSummarize logs a conversation summarization event.
func (s *Session) LogSummarize(turns int, summaryTokens int, keptMessages int, memoryID string) {
	data := map[string]interface{}{
		"summarized_turns": turns,
		"summary_tokens":   summaryTokens,
		"kept_messages":    keptMessages,
		"memory_id":        memoryID,
	}
	raw, _ := json.Marshal(data)
	s.write(Entry{
		Timestamp: time.Now(),
		Role:      "summarize",
		Content:   fmt.Sprintf("summarized %d turns → %d token summary (kept %d recent messages, memory: %s)", turns, summaryTokens, keptMessages, memoryID),
		Request:   json.RawMessage(raw),
	})
}

// LogStash logs a content stashing event.
func (s *Session) LogStash(messageType string, blockCount int, tokens int) {
	data := map[string]interface{}{
		"message_type": messageType,
		"block_count":  blockCount,
		"tokens":       tokens,
	}
	raw, _ := json.Marshal(data)
	s.write(Entry{
		Timestamp: time.Now(),
		Role:      "stash",
		Content:   fmt.Sprintf("stashed %d large blocks from %s message (%d tokens)", blockCount, messageType, tokens),
		Request:   json.RawMessage(raw),
	})
}

// LogPrune logs a conversation pruning event.
func (s *Session) LogPrune(removed int, tokensFreed int, strategy string) {
	data := map[string]interface{}{
		"removed":      removed,
		"tokens_freed": tokensFreed,
		"strategy":     strategy,
	}
	raw, _ := json.Marshal(data)
	s.write(Entry{
		Timestamp: time.Now(),
		Role:      "prune",
		Content:   fmt.Sprintf("pruned %d messages (%d tokens freed, strategy: %s)", removed, tokensFreed, strategy),
		Request:   json.RawMessage(raw),
	})
}

// Close finalizes the session log and marks it completed in the DB.
func (s *Session) Close() {
	s.mu.Lock()
	cost := s.cost()
	s.mu.Unlock()

	s.write(Entry{
		Timestamp:    time.Now(),
		Role:         "system",
		Content:      fmt.Sprintf("session ended — total tokens: in=%d out=%d — cost: $%.4f", s.totalIn, s.totalOut, cost),
		InputTokens:  s.totalIn,
		OutputTokens: s.totalOut,
		TotalCost:    cost,
	})
	s.file.Close()

	if s.store != nil {
		s.store.EndSession(s.sessionID, "completed", "")
	}
}

// CloseWithError finalizes the session log and marks it as errored in the DB.
func (s *Session) CloseWithError(err error) {
	s.mu.Lock()
	cost := s.cost()
	s.mu.Unlock()

	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	s.write(Entry{
		Timestamp:    time.Now(),
		Role:         "system",
		Content:      fmt.Sprintf("session error — %s — total tokens: in=%d out=%d — cost: $%.4f", errMsg, s.totalIn, s.totalOut, cost),
		InputTokens:  s.totalIn,
		OutputTokens: s.totalOut,
		TotalCost:    cost,
	})
	s.file.Close()

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

func (s *Session) cost() float64 {
	info := agent.GetModelInfo(s.model, s.modelStore)
	return (float64(s.totalIn) * info.InputCostPer1M / 1_000_000) +
		(float64(s.totalOut) * info.OutputCostPer1M / 1_000_000)
}

// appendTimelineEntry appends the current turn's timeline entry to timeline.md.
func (s *Session) appendTimelineEntry(turn, inTokens, outTokens, toolCalls int) {
	// Flush the JSONL file to ensure all entries are written
	s.file.Sync()

	// Reconstruct timeline events for this turn from the JSONL
	events, startTime, err := ReconstructTimelineFromJSONL(s.file.Name())
	if err != nil {
		// Log error to stderr (won't crash session, but visible in terminal)
		fmt.Fprintf(os.Stderr, "timeline generation failed (turn %d): %v\n", turn, err)
		return
	}

	timelinePath := filepath.Join(filepath.Dir(s.file.Name()), "timeline.md")
	
	// If it's turn 1, create file with header; otherwise append
	var f *os.File
	if turn == 1 {
		f, err = os.Create(timelinePath)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintf(f, "# Session Timeline — %s\n", startTime.Format("2006-01-02 15:04"))
	} else {
		f, err = os.OpenFile(timelinePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer f.Close()
	}

	// Format the full timeline and extract just this turn's section
	fullTimeline := FormatTimeline(events, startTime)
	lines := strings.Split(fullTimeline, "\n")
	
	var turnLines []string
	inCurrentTurn := false
	turnHeader := fmt.Sprintf("## Turn %d", turn)
	
	for _, line := range lines {
		if strings.HasPrefix(line, turnHeader) {
			inCurrentTurn = true
		}
		if inCurrentTurn {
			// Stop at the next turn header
			if strings.HasPrefix(line, "## Turn ") && !strings.HasPrefix(line, turnHeader) {
				break
			}
			turnLines = append(turnLines, line)
		}
	}
	
	if len(turnLines) > 0 {
		fmt.Fprintln(f, strings.Join(turnLines, "\n"))
	}
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
