// Package trace provides structured execution logging for agent sessions.
//
// Traces capture the full lifecycle of a turn — every tool call, its input/output,
// timing, token usage, and the final result. Unlike session JSONL logs (which are
// append-only for replay), traces are structured for analysis: comparing runs,
// identifying regressions, and feeding into optimization loops (cf. Meta-Harness).
//
// Each trace is a tree: Turn → Phases → Tool Calls → Results.
//
//	trace.StartTurn(turnID, input)
//	trace.RecordPhase("prepare", duration)
//	trace.RecordToolCall(toolID, name, input)
//	trace.RecordToolResult(toolID, output, duration, tokens)
//	trace.EndTurn(result, totalTokens, cost)
//
// Traces are written as structured JSON to a per-session directory.
// The filesystem layout enables a Meta-Harness-style proposer to read
// full execution history across sessions.
//
// Layout:
//
//	traces/{session_id}/
//	    turn_001.json
//	    turn_002.json
//	    summary.json   — session-level aggregates
package trace

import "time"

// Turn captures the full execution trace of a single turn.
type Turn struct {
	ID        string        `json:"id"`
	Number    int           `json:"number"`
	Input     string        `json:"input"`
	Output    string        `json:"output"`
	Model     string        `json:"model"`
	StartTime time.Time     `json:"start_time"`
	Duration  time.Duration `json:"duration_ms"`

	// Phases
	PrepareMs  int64 `json:"prepare_ms"`
	ContextMs  int64 `json:"context_ms"`
	ExecuteMs  int64 `json:"execute_ms"`
	ProcessMs  int64 `json:"process_ms"`

	// Token usage
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CacheRead    int     `json:"cache_read_tokens"`
	CacheWrite   int     `json:"cache_write_tokens"`
	Cost         float64 `json:"cost"`

	// Tool calls within this turn
	ToolCalls []ToolCall `json:"tool_calls"`

	// Outcome
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// ToolCall captures a single tool invocation within a turn.
type ToolCall struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Input    string        `json:"input"`
	Output   string        `json:"output"`
	Duration time.Duration `json:"duration_ms"`
	IsError  bool          `json:"is_error"`
}

// SessionSummary aggregates trace data across a full session.
type SessionSummary struct {
	SessionID    string    `json:"session_id"`
	Agent        string    `json:"agent"`
	StartTime    time.Time `json:"start_time"`
	TotalTurns   int       `json:"total_turns"`
	TotalTokens  int       `json:"total_tokens"`
	TotalCost    float64   `json:"total_cost"`
	ToolCallFreq map[string]int `json:"tool_call_frequency"` // tool name → count
	AvgTurnMs    int64     `json:"avg_turn_ms"`
}

// Recorder writes structured traces to the filesystem.
type Recorder struct {
	sessionID string
	agent     string
	dir       string // traces/{session_id}/
	turns     []Turn
}

// NewRecorder creates a trace recorder for a session.
// Pass empty dir to disable (no-op recorder).
func NewRecorder(dir, sessionID, agent string) *Recorder {
	if dir == "" {
		return nil
	}
	return &Recorder{
		sessionID: sessionID,
		agent:     agent,
		dir:       dir,
	}
}

// RecordTurn appends a completed turn trace.
// TODO: implement filesystem writes
func (r *Recorder) RecordTurn(t Turn) {
	if r == nil {
		return
	}
	r.turns = append(r.turns, t)
}

// WriteSummary generates and writes the session summary.
// TODO: implement
func (r *Recorder) WriteSummary() error {
	if r == nil {
		return nil
	}
	return nil
}
