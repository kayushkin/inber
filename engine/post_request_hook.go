package engine

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// PostRequestHook ensures agent work is committed and pushed after a session.
// No LLM involvement — just deterministic git operations.
type PostRequestHook struct {
	repoRoot string
}

// NewPostRequestVerifier creates a post-request hook.
func NewPostRequestVerifier(repoRoot string, _ interface{}, _ interface{}) *PostRequestHook {
	return &PostRequestHook{repoRoot: repoRoot}
}

// VerifyResult contains the outcome of post-request verification.
type VerifyResult struct {
	Clean       bool     // true if nothing to do
	Committed   bool     // true if we committed changes
	Pushed      bool     // true if we pushed
	Branch      string   // current branch
	CommitCount int      // number of unpushed commits
	Errors      []string // any errors encountered
}

// Verify ensures all changes are committed and pushed to the current branch.
func (h *PostRequestHook) Verify(ctx context.Context) (*VerifyResult, error) {
	result := &VerifyResult{}

	// Get current branch
	result.Branch = strings.TrimSpace(h.git("rev-parse", "--abbrev-ref", "HEAD"))

	// Check for uncommitted changes
	status := strings.TrimSpace(h.git("status", "--porcelain"))
	if status != "" {
		// Stage and commit everything
		h.git("add", "-A")
		out := h.git("commit", "-m", "auto: session work")
		if strings.Contains(out, "nothing to commit") {
			// Already committed
		} else {
			result.Committed = true
		}
	}

	// Check for unpushed commits
	unpushed := strings.TrimSpace(h.git("log", "--oneline", "@{u}.."))
	if unpushed == "" {
		// Try without upstream (new branch)
		unpushed = strings.TrimSpace(h.git("log", "--oneline", "-5"))
	}
	if unpushed != "" {
		result.CommitCount = len(strings.Split(unpushed, "\n"))

		// Push to current branch
		out := h.git("push", "--set-upstream", "origin", result.Branch)
		if strings.Contains(out, "error") || strings.Contains(out, "fatal") {
			result.Errors = append(result.Errors, "push failed: "+out)
		} else {
			result.Pushed = true
		}
	}

	result.Clean = !result.Committed && !result.Pushed && len(result.Errors) == 0
	return result, nil
}

// Format returns a human-readable summary.
func (r *VerifyResult) Format() string {
	if r.Clean {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n┌─ Session Wrap-Up ─────────────\n")
	sb.WriteString(fmt.Sprintf("│ Branch: %s\n", r.Branch))

	if r.Committed {
		sb.WriteString("│ ✅ Committed uncommitted changes\n")
	}
	if r.Pushed {
		sb.WriteString(fmt.Sprintf("│ ✅ Pushed %d commit(s)\n", r.CommitCount))
	}
	for _, e := range r.Errors {
		sb.WriteString(fmt.Sprintf("│ ❌ %s\n", e))
	}

	sb.WriteString("└───────────────────────────────\n")
	return sb.String()
}

// git runs a git command in the repo root.
func (h *PostRequestHook) git(args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = h.repoRoot
	out, _ := cmd.CombinedOutput()
	return string(out)
}
