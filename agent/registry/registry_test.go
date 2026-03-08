package registry

import (
	"testing"
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

// Note: LoadConfig and LoadConfigDir tests removed - agent-store is now the only config source
// Registry tests would require a test agent-store database setup
