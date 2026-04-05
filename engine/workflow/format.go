package workflow

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// formatFile handles auto-formatting for a changed file.
func (h *WorkflowHooks) formatFile(filePath string) string {
	absPath := filepath.Join(h.repoRoot, filePath)

	var cmd *exec.Cmd
	switch h.projectType {
	case "go":
		cmd = exec.Command("gofmt", "-w", absPath)
	case "node":
		cmd = exec.Command("npx", "prettier", "--write", absPath)
	case "rust":
		cmd = exec.Command("rustfmt", absPath)
	default:
		return ""
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("⚠️ format failed for %s:\n%s", filePath, strings.TrimSpace(string(out)))
	}
	return ""
}