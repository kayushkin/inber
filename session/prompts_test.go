package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestWritePromptBreakdown(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test-session.jsonl")

	params := anthropic.MessageNewParams{
		Model: "claude-sonnet-4-20250514",
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		},
		System: []anthropic.TextBlockParam{
			{Text: "You are a test assistant."},
		},
	}

	err := WritePromptBreakdown(logFile, "test-session", 1, &params, nil)
	if err != nil {
		t.Fatalf("WritePromptBreakdown failed: %v", err)
	}

	// Check file exists
	path := filepath.Join(dir, "prompts", "test-session-turn-1.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read breakdown: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Turn 1") {
		t.Error("expected 'Turn 1' in breakdown")
	}
	if !strings.Contains(content, "System Prompt") {
		t.Error("expected 'System Prompt' in breakdown")
	}
	if !strings.Contains(content, "Token Breakdown") {
		t.Error("expected 'Token Breakdown' in breakdown")
	}
}

func TestListPromptBreakdowns(t *testing.T) {
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")
	os.MkdirAll(promptsDir, 0755)

	os.WriteFile(filepath.Join(promptsDir, "sess1-turn-1.md"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(promptsDir, "sess1-turn-2.md"), []byte("test"), 0644)

	files, err := ListPromptBreakdowns(dir, "sess1")
	if err != nil {
		t.Fatalf("ListPromptBreakdowns failed: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestReadPromptBreakdown(t *testing.T) {
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")
	os.MkdirAll(promptsDir, 0755)

	os.WriteFile(filepath.Join(promptsDir, "sess1-turn-1.md"), []byte("# Turn 1 content"), 0644)

	content, err := ReadPromptBreakdown(dir, "sess1", 1)
	if err != nil {
		t.Fatalf("ReadPromptBreakdown failed: %v", err)
	}
	if content != "# Turn 1 content" {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestReadPromptBreakdown_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadPromptBreakdown(dir, "nonexistent", 1)
	if err == nil {
		t.Error("expected error for nonexistent breakdown")
	}
}
