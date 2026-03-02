package session

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSession_TruncateLargeToolResult(t *testing.T) {
	tmpDir := t.TempDir()
	
	sess, err := New(tmpDir, "test-model", "test-agent", "")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	
	// Configure aggressive truncation
	sess.SetTruncateConfig(TruncateConfig{
		Threshold:  100,
		HeadTokens: 50,
		TailTokens: 20,
		Strategy:   StrategyHeadTail,
		CreateRef:  true,
	})
	
	// Simulate large shell output (like your Go error example)
	largeOutput := strings.Repeat("router_X.go:123: cannot use *ResponseX\n", 100)
	
	// Log the tool result
	sess.LogToolResult("tool-123", "shell", largeOutput, true)
	
	// Read back the logged entry
	sess.Close()
	
	logFile := sess.FilePath()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	
	// Parse JSONL entries and find tool_result
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var entry Entry
	found := false
	
	for _, line := range lines {
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if e.Role == "tool_result" {
			entry = e
			found = true
			break
		}
	}
	
	if !found {
		t.Fatal("no tool_result entry found in log")
	}
	
	// Content should be truncated (much shorter than original)
	originalLen := len(largeOutput)
	displayedLen := len(entry.Content)
	
	if displayedLen >= originalLen {
		t.Errorf("expected truncation: original=%d, displayed=%d", originalLen, displayedLen)
	}
	
	// Should contain truncation markers
	if !strings.Contains(entry.Content, "...") {
		t.Error("expected truncation marker '...'")
	}
	
	t.Logf("✓ Truncated %d chars → %d chars (%.1f%% reduction)",
		originalLen, displayedLen,
		100.0*(float64(originalLen-displayedLen)/float64(originalLen)))
}

func TestSession_SmallToolResultNotTruncated(t *testing.T) {
	tmpDir := t.TempDir()
	
	sess, err := New(tmpDir, "test-model", "test-agent", "")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	
	// Small output should pass through unchanged
	smallOutput := "File created successfully"
	
	sess.LogToolResult("tool-456", "write_file", smallOutput, false)
	
	// Read back the logged entry
	sess.Close()
	
	logFile := sess.FilePath()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	
	// Parse JSONL entries and find tool_result
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var entry Entry
	found := false
	
	for _, line := range lines {
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if e.Role == "tool_result" {
			entry = e
			found = true
			break
		}
	}
	
	if !found {
		t.Fatal("no tool_result entry found in log")
	}
	
	// Content should be unchanged
	if entry.Content != smallOutput {
		t.Errorf("small output was modified: got %q, want %q", entry.Content, smallOutput)
	}
}

func TestSession_GetFullToolResult(t *testing.T) {
	tmpDir := t.TempDir()
	
	sess, err := New(tmpDir, "test-model", "test-agent", "")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	
	// Configure truncation
	sess.SetTruncateConfig(TruncateConfig{
		Threshold:  50,
		HeadTokens: 20,
		TailTokens: 10,
		Strategy:   StrategyHeadTail,
	})
	
	// Large output
	original := strings.Repeat("error line\n", 100)
	
	sess.LogToolResult("tool-789", "shell", original, true)
	
	// Retrieve full output
	full := sess.GetFullToolResult("tool-789")
	
	if full != original {
		t.Errorf("GetFullToolResult returned wrong content: got %d chars, want %d chars",
			len(full), len(original))
	}
	
	// Non-existent tool should return empty
	missing := sess.GetFullToolResult("tool-999")
	if missing != "" {
		t.Errorf("GetFullToolResult for missing tool should return empty, got %q", missing)
	}
}
