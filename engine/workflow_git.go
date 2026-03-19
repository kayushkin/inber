package engine

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// git runs a git command and returns combined output. Never panics.
func (h *WorkflowHooks) git(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", h.repoRoot}, args...)...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// commitFile handles auto-commit for a changed file.
func (h *WorkflowHooks) commitFile(toolName, filePath string) string {
	var msg string
	if toolName == "write_file" {
		msg = fmt.Sprintf("Create %s", filepath.Base(filePath))
	} else {
		msg = fmt.Sprintf("Update %s", filepath.Base(filePath))
	}

	// Stage file
	out, err := h.git("add", filePath)
	if err != nil {
		return fmt.Sprintf("⚠️ git add failed for %s:\n%s\nFix this before continuing.", filePath, out)
	}

	// Commit
	out, err = h.git("commit", "-m", msg)
	if err != nil {
		outStr := strings.TrimSpace(out)
		if strings.Contains(outStr, "nothing to commit") ||
			strings.Contains(outStr, "no changes added") {
			return ""
		}
		return fmt.Sprintf("⚠️ git commit failed:\n%s\nFix this git issue before continuing.", outStr)
	}

	return ""
}

// finishSessionGit handles git operations when finishing a session.
func (h *WorkflowHooks) finishSessionGit() []string {
	var parts []string

	// Ensure any uncommitted changes are committed
	status, err := h.git("status", "--porcelain")
	if err == nil && strings.TrimSpace(status) != "" {
		h.git("add", "-A")
		out, err := h.git("commit", "-m", "auto: session work")
		if err == nil && !strings.Contains(out, "nothing to commit") {
			parts = append(parts, "✅ Committed uncommitted changes")
		}
	}

	// Push if there are unpushed commits
	branch, err := h.git("rev-parse", "--abbrev-ref", "HEAD")
	if err == nil && strings.TrimSpace(branch) != "" {
		branch = strings.TrimSpace(branch)
		ahead, err := h.git("rev-list", "--count", "@{u}..")
		if err == nil && strings.TrimSpace(ahead) != "" && strings.TrimSpace(ahead) != "0" {
			out, err := h.git("push", "--set-upstream", "origin", branch)
			if err != nil {
				parts = append(parts, fmt.Sprintf("❌ Push failed: %s", strings.TrimSpace(out)))
			} else {
				parts = append(parts, fmt.Sprintf("✅ Pushed to %s", branch))
			}
		}
	}

	return parts
}

// checkGitStatus returns git issues for deployment verification.
func (h *WorkflowHooks) checkGitStatus() []string {
	var issues []string

	// Check 1: Uncommitted changes?
	status, err := h.git("status", "--porcelain")
	if err == nil && status != "" {
		issues = append(issues, fmt.Sprintf("Uncommitted changes:\n%s", status))
	}

	// Check 2: Unpushed commits?
	currentBranch, err := h.git("rev-parse", "--abbrev-ref", "HEAD")
	if err == nil && currentBranch != "" {
		ahead, err := h.git("rev-list", "--count", "@{u}..")
		if err == nil && ahead != "" && ahead != "0" {
			issues = append(issues, fmt.Sprintf("Branch '%s' has %s unpushed commits", currentBranch, ahead))
		}
	}

	return issues
}