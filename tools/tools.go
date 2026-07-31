// Package tools provides built-in tools for the inber agent.
// Tools are now interface-based for modularity and swappability.
// Legacy functions still work for backward compatibility.
package tools

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// wrap converts a tool-store Impl to an inber agent.Tool. The only structural
// difference is the schema type — tool-store uses plain JSON Schema, inber's
// agent.Tool uses the Anthropic SDK's ToolInputSchemaParam. Properties and
// Required pass through unchanged.
func wrap(t toolstoretools.Impl) agent.Tool {
	return agent.Tool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: t.InputSchema.Properties,
			Required:   t.InputSchema.Required,
		},
		Run: t.Run,
	}
}

// Default registry for built-in tools
var DefaultRegistry = NewRegistry()

// init populates the default registry with built-in tools
func init() {
	// Register all built-in tools in the default registry
	DefaultRegistry.Register(NewAgentToolAdapter(ShellCommands()))
	DefaultRegistry.Register(NewAgentToolAdapter(ReadFiles()))
	DefaultRegistry.Register(NewAgentToolAdapter(WriteFiles()))
	DefaultRegistry.Register(NewAgentToolAdapter(EditFiles()))
	DefaultRegistry.Register(NewAgentToolAdapter(ListFiles()))
	// ripgrep removed as dedicated tool — encourages grep-then-read two-turn
	// pattern when reading the file directly is one turn. Still available via
	// shell_commands ("rg ...") when truly needed for large-scale searches.
	DefaultRegistry.Register(NewAgentToolAdapter(Browser()))
	DefaultRegistry.Register(NewAgentToolAdapter(WebSearch()))
	DefaultRegistry.Register(NewAgentToolAdapter(WebFetch()))
	DefaultRegistry.Register(NewAgentToolAdapter(Scheduler()))
}

// File system tools
func ShellCommands() agent.Tool { return wrap(toolstoretools.Shell()) }

func ReadFiles() agent.Tool  { return wrap(toolstoretools.ReadFile()) }
func WriteFiles() agent.Tool { return wrap(toolstoretools.WriteFile()) }
func EditFiles() agent.Tool  { return wrap(toolstoretools.EditFile()) }
func ListFiles() agent.Tool  { return wrap(toolstoretools.ListFiles()) }
func Ripgrep() agent.Tool          { return wrap(toolstoretools.Grep()) }
func EndTurn() agent.Tool          { return wrap(toolstoretools.EndTurn()) }
func TaskPlan(repoRoot string) agent.Tool                    { return wrap(toolstoretools.TaskPlanTool(repoRoot)) }
func Scratchpad(repoRoot, agentName string) agent.Tool { return wrap(toolstoretools.ScratchpadTool(repoRoot, agentName)) }

// Code introspection tools (require configuration)
func RepoMap(rootDir string, ignorePatterns []string) agent.Tool {
	return wrap(toolstoretools.RepoMap(rootDir, ignorePatterns))
}

func RecentFiles(rootDir string) agent.Tool {
	return wrap(toolstoretools.RecentFiles(rootDir))
}

// Browser returns a tool that controls a browser via PinchTab.
func Browser() agent.Tool { return wrap(toolstoretools.Browser()) }

// WebSearch returns a tool that searches the web via Brave Search API.
func WebSearch() agent.Tool { return wrap(toolstoretools.WebSearch()) }

// WebFetch returns a tool that fetches a URL and extracts readable text.
func WebFetch() agent.Tool { return wrap(toolstoretools.WebFetch()) }

// Scheduler returns a tool that interacts with the scheduler HTTP API.
func Scheduler() agent.Tool { return wrap(toolstoretools.Scheduler()) }

// All returns standard file system tools.
// Note: RepoMap and RecentFiles require configuration (rootDir, patterns) and must be added explicitly.
func All() []agent.Tool {
	return []agent.Tool{
		ShellCommands(),
		ReadFiles(),
		WriteFiles(),
		EditFiles(),
		ListFiles(),
	}
}

// Registry-based functions for the new interface approach

// GetTool returns a tool by name from the default registry.
func GetTool(name string) Tool {
	return DefaultRegistry.Get(name)
}

// RegisterTool adds a tool to the default registry.
func RegisterTool(tool Tool) {
	DefaultRegistry.Register(tool)
}

// AllFromRegistry returns all tools from the default registry as agent.Tool slice.
// This provides a bridge between the new interface and existing engine code.
func AllFromRegistry() []agent.Tool {
	tools := DefaultRegistry.List()
	result := make([]agent.Tool, len(tools))
	for i, tool := range tools {
		result[i] = ToAgentTool(tool)
	}
	return result
}

// ByNames returns tools by their names from the default registry.
// Unknown tool names are silently ignored.
func ByNames(names []string) []agent.Tool {
	var result []agent.Tool
	for _, name := range names {
		if tool := DefaultRegistry.Get(name); tool != nil {
			result = append(result, ToAgentTool(tool))
		}
	}
	return result
}

// ListToolNames returns the names of all tools in the default registry.
func ListToolNames() []string {
	return DefaultRegistry.Names()
}
