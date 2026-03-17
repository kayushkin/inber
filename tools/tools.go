// Package tools provides built-in tools for the inber agent.
// Tools are now interface-based for modularity and swappability.
// Legacy functions still work for backward compatibility.
package tools

import (
	"context"
	"encoding/json"

	"github.com/kayushkin/agentkit"
	agentkittools "github.com/kayushkin/agentkit/tools"
	"github.com/kayushkin/inber/agent"
)

// wrap converts an agentkit.Tool to an agent.Tool
func wrap(t agentkit.Tool) agent.Tool {
	return agent.Tool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema,
		Run:         t.Run,
	}
}

// Default registry for built-in tools
var DefaultRegistry = NewRegistry()

// init populates the default registry with built-in tools
func init() {
	// Register all built-in tools in the default registry
	DefaultRegistry.Register(NewAgentToolAdapter(Shell()))
	DefaultRegistry.Register(NewAgentToolAdapter(ReadFile()))
	DefaultRegistry.Register(NewAgentToolAdapter(WriteFile()))
	DefaultRegistry.Register(NewAgentToolAdapter(EditFile()))
	DefaultRegistry.Register(NewAgentToolAdapter(ListFiles()))
	DefaultRegistry.Register(NewAgentToolAdapter(Browser()))
	DefaultRegistry.Register(NewAgentToolAdapter(WebSearch()))
	DefaultRegistry.Register(NewAgentToolAdapter(WebFetch()))
}

// File system tools
func Shell() agent.Tool     { return wrap(agentkittools.Shell()) }

// ShellInDir returns a shell tool that defaults to the given directory.
func ShellInDir(dir string) agent.Tool {
	t := agentkittools.Shell()
	origRun := t.Run
	t.Run = func(ctx context.Context, raw string) (string, error) {
		// Inject default workdir if not specified.
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			if _, ok := parsed["workdir"]; !ok || parsed["workdir"] == "" {
				parsed["workdir"] = dir
				if newRaw, err := json.Marshal(parsed); err == nil {
					raw = string(newRaw)
				}
			}
		}
		return origRun(ctx, raw)
	}
	return wrap(t)
}
func ReadFile() agent.Tool  { return wrap(agentkittools.ReadFile()) }
func WriteFile() agent.Tool { return wrap(agentkittools.WriteFile()) }
func EditFile() agent.Tool  { return wrap(agentkittools.EditFile()) }
func ListFiles() agent.Tool { return wrap(agentkittools.ListFiles()) }

// Code introspection tools (require configuration)
func RepoMap(rootDir string, ignorePatterns []string) agent.Tool {
	return wrap(agentkittools.RepoMap(rootDir, ignorePatterns))
}

func RecentFiles(rootDir string) agent.Tool {
	return wrap(agentkittools.RecentFiles(rootDir))
}

// Browser returns a tool that controls a browser via PinchTab.
func Browser() agent.Tool { return wrap(agentkittools.Browser()) }

// WebSearch returns a tool that searches the web via Brave Search API.
func WebSearch() agent.Tool { return wrap(agentkittools.WebSearch()) }

// WebFetch returns a tool that fetches a URL and extracts readable text.
func WebFetch() agent.Tool { return wrap(agentkittools.WebFetch()) }

// All returns standard file system tools.
// Note: RepoMap and RecentFiles require configuration (rootDir, patterns) and must be added explicitly.
func All() []agent.Tool {
	return []agent.Tool{
		Shell(),
		ReadFile(),
		WriteFile(),
		EditFile(),
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
