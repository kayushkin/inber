// Package tools provides built-in tools for the inber agent.
// Each tool is a function returning an agent.Tool, so callers pick what to enable.
package tools

import (
	"github.com/kayushkin/agentkit"
	agentkittools "github.com/kayushkin/agentkit/tools"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/context"
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

// Re-export individual tools so existing callers don't break.
func Shell() agent.Tool     { return wrap(agentkittools.Shell()) }
func ReadFile() agent.Tool  { return wrap(agentkittools.ReadFile()) }
func WriteFile() agent.Tool { return wrap(agentkittools.WriteFile()) }
func EditFile() agent.Tool  { return wrap(agentkittools.EditFile()) }
func ListFiles() agent.Tool { return wrap(agentkittools.ListFiles()) }

// RepoMap returns the repository structure mapping tool.
// rootDir is the repository root, ignorePatterns are glob patterns to exclude.
func RepoMap(rootDir string, ignorePatterns []string) agent.Tool {
	return context.RepoMapTool(rootDir, ignorePatterns)
}

// All returns every built-in tool.
// Note: RepoMap is not included here since it requires configuration (rootDir, patterns).
// Callers should add it explicitly via RepoMap(rootDir, patterns).
func All() []agent.Tool {
	return []agent.Tool{
		Shell(),
		ReadFile(),
		WriteFile(),
		EditFile(),
		ListFiles(),
	}
}
