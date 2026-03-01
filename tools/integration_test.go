package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kayushkin/inber/tools"
)

func TestToolsIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Test shell tool
	shellTool := tools.Shell()
	if shellTool.Name != "shell" {
		t.Errorf("expected name 'shell', got %s", shellTool.Name)
	}
	
	result, err := shellTool.Run(ctx, `{"command": "echo hello"}`)
	if err != nil {
		t.Fatalf("shell tool failed: %v", err)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("expected output to contain 'hello', got: %s", result)
	}

	// Test write_file tool
	writeTool := tools.WriteFile()
	testFile := filepath.Join(tmpDir, "test.txt")
	input := `{"path": "` + testFile + `", "content": "test content"}`
	
	result, err = writeTool.Run(ctx, input)
	if err != nil {
		t.Fatalf("write_file tool failed: %v", err)
	}
	if !strings.Contains(result, "wrote") {
		t.Errorf("expected write confirmation, got: %s", result)
	}

	// Verify file was created
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("expected 'test content', got: %s", string(content))
	}

	// Test read_file tool
	readTool := tools.ReadFile()
	input = `{"path": "` + testFile + `"}`
	
	result, err = readTool.Run(ctx, input)
	if err != nil {
		t.Fatalf("read_file tool failed: %v", err)
	}
	if !strings.Contains(result, "test content") {
		t.Errorf("expected to read 'test content', got: %s", result)
	}

	// Test edit_file tool
	editTool := tools.EditFile()
	input = `{"path": "` + testFile + `", "old_text": "test", "new_text": "edited"}`
	
	result, err = editTool.Run(ctx, input)
	if err != nil {
		t.Fatalf("edit_file tool failed: %v", err)
	}
	if !strings.Contains(result, "edited") {
		t.Errorf("expected edit confirmation, got: %s", result)
	}

	// Verify edit worked
	content, err = os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read edited file: %v", err)
	}
	if string(content) != "edited content" {
		t.Errorf("expected 'edited content', got: %s", string(content))
	}

	// Test list_files tool
	listTool := tools.ListFiles()
	input = `{"path": "` + tmpDir + `"}`
	
	result, err = listTool.Run(ctx, input)
	if err != nil {
		t.Fatalf("list_files tool failed: %v", err)
	}
	if !strings.Contains(result, "test.txt") {
		t.Errorf("expected listing to contain 'test.txt', got: %s", result)
	}

	// Test All() returns all basic tools
	allTools := tools.All()
	if len(allTools) != 5 {
		t.Errorf("expected 5 tools from All(), got %d", len(allTools))
	}
}
