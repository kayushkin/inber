package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReconstructTimelineFromJSONL(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "session.jsonl")

	// Create a simple session log
	entries := []Entry{
		{
			Timestamp: time.Date(2026, 2, 25, 21, 25, 0, 0, time.UTC),
			Role:      "system",
			Content:   "session started",
		},
		{
			Timestamp: time.Date(2026, 2, 25, 21, 25, 1, 0, time.UTC),
			Turn:      1,
			Role:      "request",
			Request: json.RawMessage(`{
				"messages": [
					{"role": "user", "content": [{"type": "text", "text": "hello world"}]}
				]
			}`),
		},
		{
			Timestamp: time.Date(2026, 2, 25, 21, 25, 2, 0, time.UTC),
			Turn:      1,
			Role:      "tool_call",
			ToolName:  "shell",
			ToolInput: json.RawMessage(`{"command":"echo hi"}`),
		},
		{
			Timestamp: time.Date(2026, 2, 25, 21, 25, 3, 0, time.UTC),
			Turn:      1,
			Role:      "tool_result",
			ToolName:  "shell",
			Content:   "hi\n",
			IsError:   false,
		},
		{
			Timestamp:    time.Date(2026, 2, 25, 21, 25, 4, 0, time.UTC),
			Turn:         1,
			Role:         "assistant",
			Content:      "Here's the output!",
			Model:        "claude-sonnet-4-20250514",
			InputTokens:  1000,
			OutputTokens: 200,
		},
	}

	f, _ := os.Create(logFile)
	enc := json.NewEncoder(f)
	for _, e := range entries {
		enc.Encode(e)
	}
	f.Close()

	events, startTime, err := ReconstructTimelineFromJSONL(logFile)
	if err != nil {
		t.Fatalf("ReconstructTimelineFromJSONL: %v", err)
	}

	if startTime.IsZero() {
		t.Error("expected non-zero start time")
	}

	// Should have: prompt, tool_call, tool_result, response, stats
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	if events[0].Type != "prompt" || events[0].TurnNumber != 1 {
		t.Errorf("event 0: expected prompt turn 1, got %s turn %d", events[0].Type, events[0].TurnNumber)
	}
	if !strings.Contains(events[0].UserMessage, "hello world") {
		t.Errorf("event 0: expected 'hello world' in message, got %s", events[0].UserMessage)
	}

	if events[1].Type != "tool_call" || events[1].ToolName != "shell" {
		t.Errorf("event 1: expected tool_call shell, got %s %s", events[1].Type, events[1].ToolName)
	}

	if events[2].Type != "tool_result" {
		t.Errorf("event 2: expected tool_result, got %s", events[2].Type)
	}

	if events[3].Type != "response" {
		t.Errorf("event 3: expected response, got %s", events[3].Type)
	}

	if events[4].Type != "stats" || events[4].InputTokens != 1000 {
		t.Errorf("event 4: expected stats with 1000 input tokens, got %s %d", events[4].Type, events[4].InputTokens)
	}
}

func TestFormatTimeline(t *testing.T) {
	startTime := time.Date(2026, 2, 25, 21, 25, 0, 0, time.UTC)
	events := []TimelineEvent{
		{
			Type:        "prompt",
			Timestamp:   startTime,
			TurnNumber:  1,
			UserMessage: "make a crab",
			PromptFile:  "prompts/turn-1.md",
		},
		{
			Type:      "tool_call",
			Timestamp: startTime.Add(time.Second),
			ToolName:  "shell",
			ToolInput: "`mkdir crab`",
			ToolCount: 1,
		},
		{
			Type:        "tool_result",
			Timestamp:   startTime.Add(2 * time.Second),
			ToolName:    "shell",
			ToolOutput:  "0 bytes",
			ToolIsError: false,
		},
		{
			Type:      "tool_call",
			Timestamp: startTime.Add(3 * time.Second),
			ToolName:  "write_file",
			ToolInput: "`crab/main.go`",
			ToolCount: 2,
		},
		{
			Type:        "tool_result",
			Timestamp:   startTime.Add(4 * time.Second),
			ToolName:    "write_file",
			ToolOutput:  "wrote 12 bytes",
			ToolIsError: false,
		},
		{
			Type:         "response",
			Timestamp:    startTime.Add(5 * time.Second),
			ResponseText: "Done! Created the crab repo.",
		},
		{
			Type:         "stats",
			Timestamp:    startTime.Add(6 * time.Second),
			InputTokens:  5000,
			OutputTokens: 300,
			ToolCalls:    2,
			Cost:         0.0150,
			Model:        "claude-sonnet-4-20250514",
		},
	}

	md := FormatTimeline(events, startTime)

	// Check header
	if !strings.Contains(md, "# Session Timeline — 2026-02-25 21:25") {
		t.Error("missing header")
	}
	if !strings.Contains(md, "## Turn 1") {
		t.Error("missing turn header")
	}
	if !strings.Contains(md, "📤 **Prompt:** \"make a crab\"") {
		t.Error("missing prompt")
	}
	if !strings.Contains(md, "⚡ **shell**") {
		t.Error("missing shell tool call")
	}
	if !strings.Contains(md, "⚡ **write_file**") {
		t.Error("missing write_file tool call")
	}
	if !strings.Contains(md, "💬 **Response:**") {
		t.Error("missing response")
	}
	if !strings.Contains(md, "📊 [in=5000 | out=300 | tools=2") {
		t.Error("missing stats")
	}
}

func TestReadTimelineFromJSONL(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "test-agent", "test-session-123")
	os.MkdirAll(sessionDir, 0755)
	logFile := filepath.Join(sessionDir, "session.jsonl")

	entries := []Entry{
		{
			Timestamp: time.Now(),
			Role:      "system",
			Content:   "session started",
		},
		{
			Timestamp: time.Now(),
			Turn:      1,
			Role:      "request",
			Request: json.RawMessage(`{
				"messages": [
					{"role": "user", "content": [{"type": "text", "text": "test message"}]}
				]
			}`),
		},
		{
			Timestamp:    time.Now(),
			Turn:         1,
			Role:         "assistant",
			Content:      "response text",
			Model:        "claude-sonnet-4-20250514",
			InputTokens:  100,
			OutputTokens: 50,
		},
	}

	f, _ := os.Create(logFile)
	enc := json.NewEncoder(f)
	for _, e := range entries {
		enc.Encode(e)
	}
	f.Close()

	// Read timeline
	content, err := ReadTimelineFromJSONL(dir, "test-session-123")
	if err != nil {
		t.Fatalf("ReadTimelineFromJSONL: %v", err)
	}

	if !strings.Contains(content, "test message") {
		t.Error("timeline missing prompt text")
	}
	if !strings.Contains(content, "response text") {
		t.Error("timeline missing response text")
	}
}

func TestFormatTerminalStats(t *testing.T) {
	ev := TimelineEvent{
		Type:         "stats",
		InputTokens:  1000,
		OutputTokens: 200,
		ToolCalls:    3,
		Cost:         0.0150,
	}
	out := FormatTerminalStats(ev)
	if !strings.Contains(out, "in=1000") {
		t.Error("missing input tokens")
	}
	if !strings.Contains(out, "tools=3") {
		t.Error("missing tool count")
	}
	if !strings.Contains(out, "$0.0150") {
		t.Error("missing cost")
	}
}

func TestCalcCost(t *testing.T) {
	// Just verify it returns 0 for unknown model
	if CalcCost("nonexistent-model", 1000, 1000) != 0 {
		t.Error("expected 0 for unknown model")
	}
	// If claude-sonnet-4-20250514 is in agent.Models, it should return > 0
	cost := CalcCost("claude-sonnet-4-20250514", 10000, 1000)
	// Don't assert exact value since model pricing may change
	_ = cost
}

func TestSummarizeToolInput(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    string
		want     string
	}{
		{
			name:     "shell command",
			toolName: "shell",
			input:    `{"command":"go build ./cmd/inber"}`,
			want:     "`go build ./cmd/inber`",
		},
		{
			name:     "read_file",
			toolName: "read_file",
			input:    `{"path":"session/timeline.go"}`,
			want:     "`session/timeline.go`",
		},
		{
			name:     "generic truncate",
			toolName: "other",
			input:    strings.Repeat("x", 100),
			want:     strings.Repeat("x", 80) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeToolInput(tt.toolName, tt.input)
			if got != tt.want {
				t.Errorf("summarizeToolInput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummarizeToolOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "empty",
			output: "",
			want:   "0 bytes",
		},
		{
			name:   "single line",
			output: "hello",
			want:   "5 bytes",
		},
		{
			name:   "multi-line",
			output: "line1\nline2\nline3",
			want:   "3 lines, 17 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeToolOutput(tt.output)
			if got != tt.want {
				t.Errorf("summarizeToolOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToolResultPairing(t *testing.T) {
	// Test that tool_call and tool_result are paired in the markdown
	startTime := time.Now()
	events := []TimelineEvent{
		{
			Type:       "prompt",
			TurnNumber: 1,
			Timestamp:  startTime,
		},
		{
			Type:      "tool_call",
			Timestamp: startTime.Add(time.Second),
			ToolName:  "shell",
			ToolInput: "`ls`",
		},
		{
			Type:       "tool_result",
			Timestamp:  startTime.Add(2 * time.Second),
			ToolName:   "shell",
			ToolOutput: "5 lines",
		},
	}

	md := FormatTimeline(events, startTime)
	
	// The shell tool_call and result should be on consecutive lines
	if !strings.Contains(md, "⚡ **shell** `ls`") {
		t.Error("missing tool call")
	}
	if !strings.Contains(md, "→ 5 lines") {
		t.Error("missing tool result")
	}
}
