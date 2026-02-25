// Package session provides conversation logging as JSONL files.
// Each session gets a timestamped file in the logs directory.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// Entry is a single log line in the session JSONL file.
type Entry struct {
	Timestamp    time.Time       `json:"ts"`
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
	totalIn   int
	totalOut  int
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
	sessionID := now.Format("2006-01-02_150405")
	
	// Add suffix for sub-agent sessions
	suffix := ""
	if parentID != "" {
		// Count existing sub-sessions to generate unique suffix
		entries, _ := os.ReadDir(agentDir)
		subCount := 0
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".jsonl" {
				subCount++
			}
		}
		suffix = fmt.Sprintf("-sub%d", subCount+1)
		sessionID += suffix
	}
	
	filename := filepath.Join(agentDir, sessionID+".jsonl")
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

// LogUser logs a user message.
func (s *Session) LogUser(text string) {
	s.write(Entry{
		Timestamp: time.Now(),
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
	s.mu.Unlock()

	s.write(Entry{
		Timestamp:    time.Now(),
		Role:         "assistant",
		Content:      text,
		Model:        s.model,
		InputTokens:  inTokens,
		OutputTokens: outTokens,
		TotalCost:    cost,
	})
}

// LogToolCall logs a tool invocation by the assistant.
func (s *Session) LogToolCall(toolID, name string, input json.RawMessage) {
	s.write(Entry{
		Timestamp: time.Now(),
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
		Role:      "thinking",
		Content:   text,
	})
}

// LogRequest logs the full API request payload (messages, tools, system prompt, model).
func (s *Session) LogRequest(payload json.RawMessage) {
	s.write(Entry{
		Timestamp: time.Now(),
		Role:      "request",
		Request:   payload,
	})
}

// Close finalizes the session log.
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
