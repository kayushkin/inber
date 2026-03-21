package registry

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

func TestToolRegistry(t *testing.T) {
	tr := NewToolRegistry()

	// Test List
	names := tr.List()
	if len(names) < 5 {
		t.Errorf("expected at least 5 built-in tools, got %d", len(names))
	}

	// Test Get
	tool, err := tr.Get("read_file")
	if err != nil {
		t.Errorf("Get(read_file) failed: %v", err)
	}
	if tool.Name != "read_file" {
		t.Errorf("expected tool name read_file, got %s", tool.Name)
	}

	// Test unknown tool
	_, err = tr.Get("unknown_tool")
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestToolRegistry_Register(t *testing.T) {
	tr := NewToolRegistry()
	
	// Create a simple test tool (just using empty schema for simplicity)
	testTool := agent.Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: anthropic.ToolInputSchemaParam{},
		Run: func(ctx context.Context, input string) (string, error) {
			return "test result", nil
		},
	}

	// Register the test tool
	tr.Register("test_tool", testTool)

	// Verify it was registered
	retrieved, err := tr.Get("test_tool")
	if err != nil {
		t.Errorf("Get(test_tool) failed: %v", err)
	}
	if retrieved.Name != "test_tool" {
		t.Errorf("expected tool name test_tool, got %s", retrieved.Name)
	}

	// Verify it's in the list
	names := tr.List()
	found := false
	for _, name := range names {
		if name == "test_tool" {
			found = true
			break
		}
	}
	if !found {
		t.Error("test_tool not found in tool list")
	}
}

func TestToolRegistry_RegisterMemoryTools(t *testing.T) {
	tr := NewToolRegistry()
	mockStore := newMockMemoryStore()

	// Register memory tools
	tr.RegisterMemoryTools(mockStore)

	// Test that memory tools were registered
	memoryTools := []string{"memory_search", "memory_save", "memory_expand", "memory_forget"}
	for _, toolName := range memoryTools {
		_, err := tr.Get(toolName)
		if err != nil {
			t.Errorf("Memory tool %s not registered: %v", toolName, err)
		}
	}
}

func TestToolRegistry_RegisterSpawnTool(t *testing.T) {
	tr := NewToolRegistry()
	
	// Create a mock spawn tool (simplified schema)
	spawnTool := agent.Tool{
		Name:        "spawn_agent",
		Description: "Spawn a sub-agent",
		InputSchema: anthropic.ToolInputSchemaParam{},
		Run: func(ctx context.Context, input string) (string, error) {
			return "spawned", nil
		},
	}

	// Register spawn tool
	tr.RegisterSpawnTool(spawnTool)

	// Verify it was registered
	retrieved, err := tr.Get("spawn_agent")
	if err != nil {
		t.Errorf("Get(spawn_agent) failed: %v", err)
	}
	if retrieved.Name != "spawn_agent" {
		t.Errorf("expected tool name spawn_agent, got %s", retrieved.Name)
	}
}