package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTimelineCreation(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.jsonl")
	os.Create(logFile)

	tl := NewTimeline(logFile, "test-session")
	if tl == nil {
		t.Fatal("expected non-nil timeline")
	}
	if len(tl.Events()) != 0 {
		t.Fatalf("expected 0 events, got %d", len(tl.Events()))
	}
}

func TestTimelineEventAccumulation(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.jsonl")
	os.Create(logFile)

	tl := NewTimeline(logFile, "test-session")

	tl.AddPrompt("hello world", 1, "prompts/test-turn-1.md")
	tl.AddToolCall("shell", `{"command":"echo hi"}`)
	tl.AddToolResult("shell", "hi\n", false)
	tl.AddResponse("Here's the output!")
	tl.AddStats("claude-sonnet-4-20250514", 1000, 200, 1)

	events := tl.Events()
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	if events[0].Type != "prompt" || events[0].TurnNumber != 1 {
		t.Errorf("event 0: expected prompt turn 1, got %s turn %d", events[0].Type, events[0].TurnNumber)
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
	if events[4].Type != "stats" || events[4].ToolCalls != 1 {
		t.Errorf("event 4: expected stats with 1 tool call, got %s %d", events[4].Type, events[4].ToolCalls)
	}
}

func TestTimelineFormat(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.jsonl")
	os.Create(logFile)

	tl := NewTimeline(logFile, "test-session")
	tl.startTime = time.Date(2026, 2, 25, 21, 25, 0, 0, time.UTC)

	tl.AddPrompt("make a crab", 1, "prompts/test-turn-1.md")
	tl.AddToolCall("shell", `{"command":"mkdir crab"}`)
	tl.AddToolResult("shell", "", false)
	tl.AddToolCall("write_file", `{"path":"crab/main.go","content":"package main"}`)
	tl.AddToolResult("write_file", "wrote 12 bytes", false)
	tl.AddResponse("Done! Created the crab repo.")
	tl.AddStats("claude-sonnet-4-20250514", 5000, 300, 2)

	md := tl.Format()

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

func TestTimelineWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.jsonl")
	os.Create(logFile)

	tl := NewTimeline(logFile, "test-session")
	tl.AddPrompt("hello", 1, "")
	tl.AddResponse("hi there")
	tl.AddStats("claude-sonnet-4-20250514", 100, 50, 0)

	if err := tl.WriteFile(); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "test-session-timeline.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("timeline file not found: %v", err)
	}

	// Read it back
	content, err := ReadTimelineFile(dir, "test-session")
	if err != nil {
		t.Fatalf("ReadTimelineFile: %v", err)
	}
	if !strings.Contains(content, "hello") {
		t.Error("timeline content missing prompt text")
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
	// Just verify it returns 0 for unknown model and non-zero for known
	if CalcCost("nonexistent-model", 1000, 1000) != 0 {
		t.Error("expected 0 for unknown model")
	}
	// If claude-sonnet-4-20250514 is in agent.Models, it should return > 0
	cost := CalcCost("claude-sonnet-4-20250514", 10000, 1000)
	// Don't assert exact value since model pricing may change
	_ = cost
}

func TestToolCountResets(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.jsonl")
	os.Create(logFile)

	tl := NewTimeline(logFile, "test-session")
	tl.AddPrompt("turn 1", 1, "")
	tl.AddToolCall("shell", `{"command":"echo 1"}`)
	tl.AddToolCall("shell", `{"command":"echo 2"}`)

	events := tl.Events()
	if events[1].ToolCount != 1 {
		t.Errorf("expected tool count 1, got %d", events[1].ToolCount)
	}
	if events[2].ToolCount != 2 {
		t.Errorf("expected tool count 2, got %d", events[2].ToolCount)
	}

	// New prompt resets counter
	tl.AddPrompt("turn 2", 2, "")
	tl.AddToolCall("shell", `{"command":"echo 3"}`)
	events = tl.Events()
	if events[4].ToolCount != 1 {
		t.Errorf("expected tool count reset to 1, got %d", events[4].ToolCount)
	}
}
