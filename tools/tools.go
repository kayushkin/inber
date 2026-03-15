// Package tools provides built-in tools for the inber agent.
// Each tool is a function returning an agent.Tool, so callers pick what to enable.
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
