// Package tools provides built-in tools for the inber agent.
// Each tool is a function returning an agent.Tool, so callers pick what to enable.
package tools

import (
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/tools/fs"
	"github.com/kayushkin/inber/tools/shell"
)

// Re-export individual tools so existing callers don't break.
func Shell() agent.Tool    { return shell.Shell() }
func ReadFile() agent.Tool { return fs.ReadFile() }
func WriteFile() agent.Tool { return fs.WriteFile() }
func EditFile() agent.Tool { return fs.EditFile() }
func ListFiles() agent.Tool { return fs.ListFiles() }

// All returns every built-in tool.
func All() []agent.Tool {
	return []agent.Tool{
		Shell(),
		ReadFile(),
		WriteFile(),
		EditFile(),
		ListFiles(),
	}
}
