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
	path := filepath.Join(dir, "prompts", "turn-1.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read breakdown: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Turn 1") {
		t.Error("expected 'Turn 1' in breakdown")
	}
	if !strings.Contains(content, "system.md") {
		t.Error("expected link to system.md in breakdown")
	}
	if !strings.Contains(content, "Tokens") {
		t.Error("expected 'Tokens' in breakdown")
	}
	// Check system.md index was written
	sysPath := filepath.Join(dir, "prompts", "system.md")
	sysData, err := os.ReadFile(sysPath)
	if err != nil {
		t.Fatalf("system.md not written: %v", err)
	}
	if !strings.Contains(string(sysData), "System Prompt") {
		t.Error("expected 'System Prompt' in system.md")
	}
	// Check individual block file was written
	blockFiles, _ := filepath.Glob(filepath.Join(dir, "prompts", "system-01-*.md"))
	if len(blockFiles) == 0 {
		t.Error("expected system block file to be written")
	}
	// Check tools.md was written
	toolsPath := filepath.Join(dir, "prompts", "tools.md")
	if _, err := os.ReadFile(toolsPath); err != nil {
		t.Fatalf("tools.md not written: %v", err)
	}
}

func TestListPromptBreakdowns(t *testing.T) {
	dir := t.TempDir()
	promptsDir := filepath.Join(dir, "prompts")
	os.MkdirAll(promptsDir, 0755)

	// New format: prompts inside session dir
	sessDir := filepath.Join(dir, "sess1")
	sessPDir := filepath.Join(sessDir, "prompts")
	os.MkdirAll(sessPDir, 0755)
	os.WriteFile(filepath.Join(sessPDir, "turn-1.md"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(sessPDir, "turn-2.md"), []byte("test"), 0644)

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

	sessDir := filepath.Join(dir, "sess1")
	sessPDir := filepath.Join(sessDir, "prompts")
	os.MkdirAll(sessPDir, 0755)
	os.WriteFile(filepath.Join(sessPDir, "turn-1.md"), []byte("# Turn 1 content"), 0644)

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
