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
	if toolName == "write_file" || toolName == "write_files" {
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

// remoteDefaultBranch asks origin which branch its HEAD points at — the branch
// a push publishes to every other clone of the repository.
//
// It asks the remote rather than reading the local `refs/remotes/origin/HEAD`
// tracking ref, because that ref is written by `git clone` and by
// `git remote set-head`, and by nothing else: 57 of the 78 repositories on this
// host do not have it. Reading it would answer "unknown" for repositories whose
// default branch is perfectly well defined, which is the wrong answer in the
// direction that matters. It is one round-trip to a remote we are about to push
// to anyway.
//
// Returns "" when origin does not name one (an empty remote, or a transport
// error). That is "unknown", not "there isn't one", and callers must treat it
// as such.
func (h *WorkflowHooks) remoteDefaultBranch() string {
	out, err := h.git("ls-remote", "--symref", "origin", "HEAD")
	if err != nil {
		return ""
	}
	// The symref line reads: "ref: refs/heads/main\tHEAD"
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != "ref:" || fields[2] != "HEAD" {
			continue
		}
		return strings.TrimPrefix(fields[1], "refs/heads/")
	}
	return ""
}

// finishSessionGit handles git operations when finishing a session.
//
// The session is ending, so nothing downstream will read what this returns
// except the human looking at the summary. Anything it declines to do must
// therefore say so out loud — a silent "not pushed" is indistinguishable from
// "nothing to push".
func (h *WorkflowHooks) finishSessionGit() []string {
	// autoCommit is the session's authority to write git history on its own.
	// The per-write commit in OnToolResult honours it; this one used to run
	// whatever the config said, so switching auto-commit off left the larger,
	// less selective commit still armed.
	if !h.autoCommit {
		return nil
	}

	var parts []string

	// Ensure any uncommitted changes are committed
	status, err := h.git("status", "--porcelain")
	if err == nil && strings.TrimSpace(status) != "" {
		if out, err := h.git("add", "-A"); err != nil {
			// Staging failed, so what is staged now is not the session's work.
			// Committing and pushing it anyway is how a partial tree reaches
			// the remote under a message claiming it is the session.
			return append(parts, fmt.Sprintf("❌ git add failed, nothing committed or pushed:\n%s", strings.TrimSpace(out)))
		}
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
			ahead = strings.TrimSpace(ahead)
			if refusal := h.refuseDefaultBranchPush(branch, ahead); refusal != "" {
				return append(parts, refusal)
			}
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

// refuseDefaultBranchPush returns the reason the end-of-session push must not
// run, or "" if it may. The session ends unattended, so the branch everyone
// else tracks is the one place it must not publish to on its own: a commit can
// be amended or dropped, a push cannot be taken back.
//
// Unknown is refused as well as known-default. The whole point of the gate is
// that nobody is watching, and "origin did not tell me which branch is shared"
// is not evidence that this one isn't.
func (h *WorkflowHooks) refuseDefaultBranchPush(branch, ahead string) string {
	if h.pushToDefaultBranch {
		return ""
	}
	switch h.remoteDefaultBranch() {
	case "":
		return fmt.Sprintf("⏸️ Not pushed: origin did not say which branch it treats as default, so '%s' cannot be shown to be safe to publish unattended. %s commit(s) are waiting locally.", branch, ahead)
	case branch:
		return fmt.Sprintf("⏸️ Not pushed: '%s' is origin's default branch. %s commit(s) are waiting locally — push them yourself, or set AutoWorkflowConfig.PushToDefaultBranch.", branch, ahead)
	}
	return ""
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