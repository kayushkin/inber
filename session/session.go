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

// Session tracks and logs a conversation.
type Session struct {
	mu        sync.Mutex
	file      *os.File
	enc       *json.Encoder
	start     time.Time
	model     string
	agentName string // agent name for multi-agent support
	parentID  string // parent session ID (empty for root)
	sessionID string // unique session ID
	turn      int    // current API round-trip number
	totalIn   int
	totalOut  int
	db        *DB    // session tracking DB (nil if unavailable)
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
func New(logsDir, model, agentName, parentID string) (*Session, error) {
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
		file:      f,
		enc:       json.NewEncoder(f),
		start:     now,
		model:     model,
		agentName: agentName,
		parentID:  parentID,
		sessionID: sessionID,
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

// AttachDB attaches a session tracking database and registers this session.
func (s *Session) AttachDB(db *DB, command string) {
	s.db = db
	if db != nil {
		db.InsertSession(&SessionRow{
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

// currentTurn returns the current turn number (0 before first request).
func (s *Session) currentTurn() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turn
}

// LogUser logs a user message.
func (s *Session) LogUser(text string) {
	s.write(Entry{
		Timestamp: time.Now(),
		Turn:      s.currentTurn(),
		Role:      "user",
		Content:   text,
	})
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
	
	if s.db != nil {
		cost := CalcCost(s.model, inTokens, outTokens)
		s.db.EndTurn(s.sessionID, turn, inTokens, outTokens, toolCalls, cost, stopReason, errMsg)
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

// LogToolResult logs a tool result.
func (s *Session) LogToolResult(toolID, name, output string, isError bool) {
	s.write(Entry{
		Timestamp: time.Now(),
		Turn:      s.currentTurn(),
		Role:      "tool_result",
		ToolID:    toolID,
		ToolName:  name,
		Content:   output,
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

	if s.db != nil {
		s.db.InsertTurn(&TurnRow{
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

	if s.db != nil {
		s.db.EndSession(s.sessionID, "completed", "")
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

	if s.db != nil {
		s.db.EndSession(s.sessionID, "error", errMsg)
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
}

func (s *Session) cost() float64 {
	info, ok := agent.Models[s.model]
	if !ok {
		return 0
	}
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
		return // best-effort; don't crash on timeline failure
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
