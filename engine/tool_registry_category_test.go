package engine

import (
	"testing"

	"github.com/kayushkin/inber/agent"
	memorystore "github.com/kayushkin/memory-store"
)

// toolRegistryRecorder keeps what loadToolsIntoMemory hands the memory store.
type toolRegistryRecorder struct {
	emptyMemoryStore
	loaded []memorystore.ToolMetadata
}

func (r *toolRegistryRecorder) LoadToolRegistry(tools []memorystore.ToolMetadata) error {
	r.loaded = tools
	return nil
}

// The shell tool is named shell_commands; nothing produces a tool named
// "shell", so only shell_commands may land in the execution category.
func TestLoadToolsIntoMemoryPutsShellCommandsInExecution(t *testing.T) {
	recorder := &toolRegistryRecorder{}
	tools := []agent.Tool{
		{Name: "shell_commands"},
		{Name: "read_files"},
		{Name: "memory_search"},
		{Name: "web_fetch"},
	}
	if err := loadToolsIntoMemory(recorder, tools); err != nil {
		t.Fatalf("loadToolsIntoMemory: %v", err)
	}
	want := map[string]string{
		"shell_commands": "execution",
		"read_files":     "filesystem",
		"memory_search":  "memory",
		"web_fetch":      "general",
	}
	if len(recorder.loaded) != len(want) {
		t.Fatalf("loaded %d tools, want %d", len(recorder.loaded), len(want))
	}
	for _, tool := range recorder.loaded {
		if tool.Category != want[tool.Name] {
			t.Errorf("tool %q: category %q, want %q", tool.Name, tool.Category, want[tool.Name])
		}
	}
}
