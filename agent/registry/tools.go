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
	r.Register(tools.ShellCommands())
	r.Register(tools.ReadFiles())
	r.Register(tools.WriteFiles())
	r.Register(tools.EditFiles())
	r.Register(tools.ListFiles())
	r.Register(tools.Browser())
	r.Register(tools.WebSearch())
	r.Register(tools.WebFetch())

	return r
}

// RegisterMemoryTools adds memory tools to the registry using the given memory store
func (r *ToolRegistry) RegisterMemoryTools(store memory.MemoryStore) {
	r.Register(memory.SearchTool(store))
	r.Register(memory.SaveTool(store))
	r.Register(memory.ExpandTool(store))
	r.Register(memory.ForgetTool(store))
}

// RegisterSpawnTool adds the spawn_agent tool to the registry.
// Must be called after registry creation since it needs a reference to the registry itself.
func (r *ToolRegistry) RegisterSpawnTool(spawnTool agent.Tool) {
	r.Register(spawnTool)
}

// Register adds a tool to the registry under the name the tool declares.
// The tool's own Name is the only key: registering under a separately-supplied
// string let the two drift apart silently, which is how the registry ended up
// serving tool-store's post-rename names while callers still asked for the
// pre-rename ones.
func (r *ToolRegistry) Register(tool agent.Tool) {
	r.tools[tool.Name] = tool
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
