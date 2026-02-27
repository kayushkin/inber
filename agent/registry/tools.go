package registry

import (
	"fmt"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/tools"
)

// ToolRegistry maps tool names to tool constructors
type ToolRegistry struct {
	tools map[string]agent.Tool
}

// NewToolRegistry creates a registry with all built-in tools
// Note: Memory tools require a memory.Store instance which is not available at
// registry creation time. They must be registered separately when the store is available.
func NewToolRegistry() *ToolRegistry {
	r := &ToolRegistry{
		tools: make(map[string]agent.Tool),
	}

	// Register all built-in file and shell tools
	r.Register("shell", tools.Shell())
	r.Register("read_file", tools.ReadFile())
	r.Register("write_file", tools.WriteFile())
	r.Register("edit_file", tools.EditFile())
	r.Register("list_files", tools.ListFiles())

	return r
}

// RegisterMemoryTools adds memory tools to the registry using the given memory store
func (r *ToolRegistry) RegisterMemoryTools(store *memory.Store) {
	r.Register("memory_search", memory.SearchTool(store))
	r.Register("memory_save", memory.SaveTool(store))
	r.Register("memory_expand", memory.ExpandTool(store))
	r.Register("memory_forget", memory.ForgetTool(store))
}

// Register adds a tool to the registry
func (r *ToolRegistry) Register(name string, tool agent.Tool) {
	r.tools[name] = tool
}

// Get returns a tool by name
func (r *ToolRegistry) Get(name string) (agent.Tool, error) {
	tool, ok := r.tools[name]
	if !ok {
		return agent.Tool{}, fmt.Errorf("tool %q not registered", name)
	}
	return tool, nil
}

// List returns all registered tool names
func (r *ToolRegistry) List() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}
