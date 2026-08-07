package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The defect these tests exist for: the close-time commit ran `git add -A`,
// which stages the whole working tree rather than the session's own writes. Two
// things follow, and both are asserted here against a real git repository
// rather than a fake, because every claim is about what git itself records.
//
//  1. It swept files nobody asked it to commit into git — inber's own
//     `<workspace>/logs/<agent>/<session>/` transcripts ended up tracked in
//     three repositories on this host (kayushkin.com 275 files, downloadstack
//     95, si 42).
//  2. It committed a parallel agent's in-progress work under a message claiming
//     the work was this session's, and then pushed it.
//
// The second is the one that cannot be undone, so it gets the sharper tests.

// sessionWrote is the fixture for "this session changed this file": it writes
// the file and records the write the way OnToolResult does.
func sessionWrote(t *testing.T, h *WorkflowHooks, repo, name, content string) {
	t.Helper()
	full := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	h.changedFiles = append(h.changedFiles, full)
}

// committedPaths lists the paths a commit touched, which is the only honest way
// to ask "what did this commit actually take".
//
// It is deliberately not given a revision: every caller means "the commit the
// session just made", and asking for HEAD by name silently answers with the
// fixture's seed commit when no commit was made at all — which is how a test
// asserting `[seed.txt]` passed against a close-time commit that never ran.
func committedPaths(t *testing.T, repo, headBefore string) []string {
	t.Helper()
	if head := runGit(t, repo, "rev-parse", "HEAD"); head == headBefore {
		t.Fatalf("HEAD did not move — no commit was made at all")
	}
	out := runGit(t, repo, "show", "--name-only", "--pretty=format:", "HEAD")
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

// The headline case. A parallel agent's edit sits in the same work tree; the
// session must commit its own file and leave the other one exactly where it is.
func TestCloseTimeCommitLeavesAnotherAgentsUntrackedFileAlone(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	before := runGit(t, repo, "rev-parse", "HEAD")
	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}

	sessionWrote(t, h, repo, "mine.txt", "session work\n")
	dirtyWorkTree(t, repo, "theirs.txt") // another agent, mid-task

	h.finishSessionGit()

	got := committedPaths(t, repo, before)
	if len(got) != 1 || got[0] != "mine.txt" {
		t.Fatalf("close-time commit took %v, want exactly [mine.txt] — it is still sweeping the tree", got)
	}
	if status := runGit(t, repo, "status", "--porcelain"); !strings.Contains(status, "theirs.txt") {
		t.Fatalf("another agent's file is no longer uncommitted; status:\n%s", status)
	}
}

// The nastier half of the same defect: `git commit` with no pathspec commits
// whatever is in the index, so scoping only the `git add` would still carry off
// work a parallel agent had staged. Staging is exactly what an agent does
// mid-task, so this is not a hypothetical shape.
func TestCloseTimeCommitLeavesAnotherAgentsSTAGEDWorkAlone(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	before := runGit(t, repo, "rev-parse", "HEAD")
	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}

	sessionWrote(t, h, repo, "mine.txt", "session work\n")
	dirtyWorkTree(t, repo, "theirs.txt")
	runGit(t, repo, "add", "theirs.txt") // the other agent staged it

	h.finishSessionGit()

	got := committedPaths(t, repo, before)
	if len(got) != 1 || got[0] != "mine.txt" {
		t.Fatalf("close-time commit took %v, want exactly [mine.txt] — a bare `git commit` swept the index", got)
	}
	staged := runGit(t, repo, "diff", "--cached", "--name-only")
	if staged != "theirs.txt" {
		t.Fatalf("the other agent's staged work is no longer staged (diff --cached = %q)", staged)
	}
}

// With nothing attributable there is nothing to commit, and the session must
// say so rather than commit the tree or fall silent. A silent "no commit" is
// indistinguishable from "there was nothing to do", and the difference here is
// whether somebody else's work is sitting uncommitted.
func TestCloseTimeCommitRefusesWhenNothingIsAttributableToTheSession(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	before := runGit(t, repo, "rev-parse", "HEAD")

	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}
	dirtyWorkTree(t, repo, "theirs.txt")

	summary := strings.Join(h.finishSessionGit(), "\n")

	if runGit(t, repo, "rev-parse", "HEAD") != before {
		t.Fatalf("committed a tree it could not attribute to the session; summary:\n%s", summary)
	}
	if !strings.Contains(summary, "Not committed") {
		t.Fatalf("the refusal was silent, which reads as 'nothing to commit'; summary was:\n%s", summary)
	}
}

// A clean tree with nothing recorded is the ordinary read-only session. It must
// not produce the refusal above — that would put a warning on every session that
// simply had nothing to commit, and a warning everyone learns to ignore is worse
// than none.
func TestACleanTreeSaysNothingAtAll(t *testing.T) {
	repo, _ := newRepoWithRemote(t)

	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}
	if parts := h.finishSessionGit(); len(parts) != 0 {
		t.Fatalf("a clean tree reported %v, want nothing", parts)
	}
}

// A file the session deleted is still the session's change, and `git add` alone
// does not stage a deletion.
func TestASessionsDELETEIsCommitted(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	before := runGit(t, repo, "rev-parse", "HEAD")
	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}

	if err := os.Remove(filepath.Join(repo, "seed.txt")); err != nil {
		t.Fatal(err)
	}
	h.changedFiles = append(h.changedFiles, filepath.Join(repo, "seed.txt"))

	h.finishSessionGit()

	if got := committedPaths(t, repo, before); len(got) != 1 || got[0] != "seed.txt" {
		t.Fatalf("the deletion was not committed: %v", got)
	}
	if status := runGit(t, repo, "status", "--porcelain"); status != "" {
		t.Fatalf("tree still dirty after committing the deletion:\n%s", status)
	}
}

// An edit of a file that was already tracked. This is the ordinary case and it
// is here because it is the one that catches a subtle reading error: `git
// status --porcelain` spends two columns on a status code, and the unstaged
// half of that code is a LEADING SPACE (" M path"). Trimming the output — which
// the human-facing git helper does — shifts the line left and turns the path
// into "ath", so the file matches nothing and the session commits none of its
// own edits. An untracked file ("?? path") has no leading space and hides this
// completely, so a test suite made only of new files reads as passing.
func TestASessionsEDITOfATrackedFileIsCommitted(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	before := runGit(t, repo, "rev-parse", "HEAD")
	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}

	sessionWrote(t, h, repo, "seed.txt", "seed, edited by the session\n")

	summary := strings.Join(h.finishSessionGit(), "\n")

	if got := committedPaths(t, repo, before); len(got) != 1 || got[0] != "seed.txt" {
		t.Fatalf("commit took %v, want exactly [seed.txt]; summary:\n%s", got, summary)
	}
	if status := runGit(t, repo, "status", "--porcelain"); status != "" {
		t.Fatalf("tree still dirty after committing the session's own edit:\n%s", status)
	}
}

// A session's file tools are not confined to the repository, so the recorded
// list can name a path git has no business being handed. Passing it to
// `git add` is a hard error that would take the whole commit down with it —
// so the failure this catches is not "the outside file got committed", it is
// "the session's real work did not, because of a path beside it".
func TestAPathOUTSIDETheRepositoryIsDroppedNotPassedToGit(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	before := runGit(t, repo, "rev-parse", "HEAD")
	outside := filepath.Join(t.TempDir(), "elsewhere.txt")
	if err := os.WriteFile(outside, []byte("not in the repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}
	sessionWrote(t, h, repo, "mine.txt", "session work\n")
	h.changedFiles = append(h.changedFiles, outside)

	summary := strings.Join(h.finishSessionGit(), "\n")

	if got := committedPaths(t, repo, before); len(got) != 1 || got[0] != "mine.txt" {
		t.Fatalf("commit took %v, want exactly [mine.txt]; summary:\n%s", got, summary)
	}
}

// A recorded path the session wrote and then reverted has nothing to commit.
// It must not be handed to git either: `git add -- <untracked, absent path>` is
// a fatal pathspec error, and one such path would sink a commit that had real
// work in it.
func TestARecordedPathThatNoLongerDiffersDoesNotSinkTheCommit(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	before := runGit(t, repo, "rev-parse", "HEAD")
	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}

	sessionWrote(t, h, repo, "scratch.txt", "temporary\n")
	if err := os.Remove(filepath.Join(repo, "scratch.txt")); err != nil {
		t.Fatal(err)
	}
	sessionWrote(t, h, repo, "mine.txt", "session work\n")

	summary := strings.Join(h.finishSessionGit(), "\n")

	if got := committedPaths(t, repo, before); len(got) != 1 || got[0] != "mine.txt" {
		t.Fatalf("commit took %v, want exactly [mine.txt]; summary:\n%s", got, summary)
	}
}

// Untracked files inside a NEW directory are reported by `git status
// --porcelain` as the directory, not the files, unless untracked files are
// asked for individually. Matching the recorded paths against the collapsed
// form would drop every file in a newly created package.
func TestANewFileInANewDIRECTORYIsStillAttributed(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	before := runGit(t, repo, "rev-parse", "HEAD")
	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}

	sessionWrote(t, h, repo, "pkg/deep/new.go", "package deep\n")

	summary := strings.Join(h.finishSessionGit(), "\n")

	if got := committedPaths(t, repo, before); len(got) != 1 || got[0] != "pkg/deep/new.go" {
		t.Fatalf("commit took %v, want exactly [pkg/deep/new.go]; summary:\n%s", got, summary)
	}
}

// The per-write commit has the same bare `git commit` as the close-time one, so
// it carries off staged work the same way. It is unreachable today (the name
// gate of todo af237d64), which is exactly why it needs its own test: nothing
// else would catch this the day that gate is re-armed.
func TestThePerWriteCommitAlsoLeavesStagedWorkAlone(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	before := runGit(t, repo, "rev-parse", "HEAD")
	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}

	if err := os.WriteFile(filepath.Join(repo, "mine.txt"), []byte("session work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirtyWorkTree(t, repo, "theirs.txt")
	runGit(t, repo, "add", "theirs.txt")

	if msg := h.commitFile("write_files", filepath.Join(repo, "mine.txt")); msg != "" {
		t.Fatalf("commitFile reported a problem: %s", msg)
	}

	if got := committedPaths(t, repo, before); len(got) != 1 || got[0] != "mine.txt" {
		t.Fatalf("per-write commit took %v, want exactly [mine.txt]", got)
	}
	if staged := runGit(t, repo, "diff", "--cached", "--name-only"); staged != "theirs.txt" {
		t.Fatalf("the other agent's staged work is no longer staged (diff --cached = %q)", staged)
	}
}
