package engine

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// git runs a git command and returns combined output, trimmed. Never panics.
func (h *WorkflowHooks) git(args ...string) (string, error) {
	out, err := h.gitRaw(args...)
	return strings.TrimSpace(out), err
}

// gitRaw is git with the output left exactly as git wrote it.
//
// Machine-readable git output cannot be trimmed. `git status --porcelain`
// spends the first two columns on a status code whose unstaged half is a
// LEADING SPACE — " M path", " D path" — so trimming shifts every such line two
// characters left and reads the path as "ath". The trimmed helper above is for
// output meant for a human.
func (h *WorkflowHooks) gitRaw(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", h.repoRoot}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// commitFile handles auto-commit for a changed file.
//
// Both git commands carry the file as an explicit pathspec. `git commit` with
// no pathspec commits the whole index, so staging one file and then committing
// would still carry off anything a parallel agent had staged in the same work
// tree — under a message naming this session's file.
func (h *WorkflowHooks) commitFile(toolName, filePath string) string {
	var msg string
	if toolName == "write_file" || toolName == "write_files" {
		msg = fmt.Sprintf("Create %s", filepath.Base(filePath))
	} else {
		msg = fmt.Sprintf("Update %s", filepath.Base(filePath))
	}

	// Stage file
	out, err := h.git("add", "--", filePath)
	if err != nil {
		return fmt.Sprintf("⚠️ git add failed for %s:\n%s\nFix this before continuing.", filePath, out)
	}

	// Commit
	out, err = h.git("commit", "-m", msg, "--", filePath)
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

// changedPathsInWorkTree returns every path git reports as differing from HEAD,
// repository-relative: modified, staged, deleted, renamed (both ends) and
// untracked. Ignored paths are absent, which is what stopped inber's own
// session transcripts from being committed once `logs/` was gitignored.
//
// Untracked files are asked for individually. The default porcelain output
// collapses a wholly untracked directory to the directory alone, so a session
// that created a new package would have every file in it read as unattributable.
//
// The NUL-separated form is used because the default one quotes and escapes any
// path with a special character in it, and this set is matched against real
// paths.
func (h *WorkflowHooks) changedPathsInWorkTree() (map[string]bool, error) {
	out, err := h.gitRaw("status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("git status: %s", strings.TrimSpace(out))
	}

	changed := map[string]bool{}
	records := strings.Split(strings.Trim(out, "\x00"), "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 4 {
			continue
		}
		status, path := record[:2], record[3:]
		changed[path] = true
		// A rename or copy is followed by its source path as its own record.
		// The source is a change to that path too, and consuming it here keeps
		// it from being read as a status code.
		if (status[0] == 'R' || status[0] == 'C') && i+1 < len(records) {
			i++
			changed[records[i]] = true
		}
	}
	return changed, nil
}

// sessionPathspecs returns the repository-relative paths this session changed:
// the files it wrote, narrowed to those git agrees differ from HEAD.
//
// Narrowing to what git reports is the whole of the filtering, and it is doing
// two jobs at once:
//
//   - It drops a path that was recorded but no longer differs — written and
//     then reverted, or written and then removed. `git add -- <an untracked
//     path that is not there>` is a fatal pathspec error, so one such path would
//     take down a commit that had real work in it.
//   - It drops a path outside the repository. A session's file tools are not
//     confined to its workspace (todo d967400a), so a recorded path can name
//     anything on disk — and `git add` handed a path outside the repository is
//     the same fatal error. Every path in the changed set is repository-relative
//     by construction, so an outside path can never match one.
//
// A relative recorded path is read as relative to the repository root, which is
// how `git -C <root>` would have read it anyway.
func (h *WorkflowHooks) sessionPathspecs() ([]string, error) {
	changed, err := h.changedPathsInWorkTree()
	if err != nil {
		return nil, err
	}

	var paths []string
	seen := map[string]bool{}
	for _, recorded := range h.changedFiles {
		abs := recorded
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(h.repoRoot, abs)
		}
		rel, err := filepath.Rel(h.repoRoot, abs)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if !changed[rel] || seen[rel] {
			continue
		}
		seen[rel] = true
		paths = append(paths, rel)
	}
	return paths, nil
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

	// Commit the session's own writes — and only those.
	//
	// This used to be `git add -A` followed by a bare `git commit`, which is
	// two ways of saying "everything in the tree". It committed inber's own
	// session transcripts into three repositories on this host, and, worse, it
	// swept up whatever a parallel agent had edited or staged in the same work
	// tree and pushed it under a message claiming the work was this session's.
	// A commit can be amended; a push cannot be taken back.
	paths, err := h.sessionPathspecs()
	if err != nil {
		return append(parts, fmt.Sprintf("❌ Could not read git status, nothing committed or pushed:\n%s", err))
	}
	if len(paths) > 0 {
		if out, err := h.git(append([]string{"add", "--"}, paths...)...); err != nil {
			// Staging failed, so what is staged now is not the session's work.
			// Committing and pushing it anyway is how a partial tree reaches
			// the remote under a message claiming it is the session.
			return append(parts, fmt.Sprintf("❌ git add failed, nothing committed or pushed:\n%s", strings.TrimSpace(out)))
		}
		out, err := h.git(append([]string{"commit", "-m", "auto: session work", "--"}, paths...)...)
		if err == nil && !strings.Contains(out, "nothing to commit") {
			parts = append(parts, fmt.Sprintf("✅ Committed %d file%s changed by this session", len(paths), plural(len(paths))))
		}
	}

	// Anything still dirty is somebody else's, or was never attributed to a
	// tool call. Say so: a silent "not committed" is indistinguishable from
	// "there was nothing to commit", and the difference is whether work is
	// sitting in the tree waiting for a human.
	if leftover, err := h.git("status", "--porcelain"); err == nil && strings.TrimSpace(leftover) != "" {
		parts = append(parts, fmt.Sprintf("⏸️ Not committed: the working tree still has changes this session did not make. Left for whoever did:\n%s", strings.TrimSpace(leftover)))
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