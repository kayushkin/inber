package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests drive real git against a real bare remote in a temp directory.
// A fake would not do: every claim under test is about what git itself reports
// (which branch origin calls default, whether a ref reached the remote), and a
// fake would only re-state the assumption being checked.

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepoWithRemote builds a work tree on branch "main" whose origin is a bare
// repo that already has main, so origin's HEAD names main — the shape of every
// repository these hooks actually run in.
func newRepoWithRemote(t *testing.T) (repo string, remote string) {
	t.Helper()
	base := t.TempDir()
	remote = filepath.Join(base, "remote.git")
	repo = filepath.Join(base, "work")

	runGit(t, base, "init", "--bare", "--initial-branch=main", remote)
	runGit(t, base, "init", "--initial-branch=main", repo)
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "test")
	runGit(t, repo, "remote", "add", "origin", remote)

	if err := os.WriteFile(filepath.Join(repo, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "seed.txt")
	runGit(t, repo, "commit", "-m", "seed")
	runGit(t, repo, "push", "--set-upstream", "origin", "main")
	return repo, remote
}

// dirtyWorkTree leaves one uncommitted file, so finishSessionGit has something
// to commit and therefore something to try to push.
func dirtyWorkTree(t *testing.T, repo, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func remoteHas(t *testing.T, remote, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "-C", remote, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return cmd.Run() == nil
}

func remoteTip(t *testing.T, remote, branch string) string {
	t.Helper()
	return runGit(t, remote, "rev-parse", "refs/heads/"+branch)
}

// The defect this whole change exists for: a session that ended on main pushed
// to main, unattended, and a push cannot be taken back.
func TestSessionCloseRefusesToPushTheRemotesDefaultBranch(t *testing.T) {
	repo, remote := newRepoWithRemote(t)
	before := remoteTip(t, remote, "main")
	dirtyWorkTree(t, repo, "new.txt")

	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}
	summary := strings.Join(h.finishSessionGit(), "\n")

	if remoteTip(t, remote, "main") != before {
		t.Fatal("main was pushed to origin; the default-branch gate did not hold")
	}
	if !strings.Contains(summary, "Not pushed") {
		t.Fatalf("the refusal was silent, which reads as 'nothing to push'; summary was:\n%s", summary)
	}
	// The work must still be safe locally — refusing to publish is not
	// refusing to record.
	if out := runGit(t, repo, "status", "--porcelain"); out != "" {
		t.Fatalf("changes were left uncommitted as well as unpushed:\n%s", out)
	}
}

// The complement, and the assertion that matters most: a gate that refuses
// everything would pass the test above and break the feature outright.
func TestSessionClosePushesABranchThatIsNotTheDefault(t *testing.T) {
	repo, remote := newRepoWithRemote(t)
	runGit(t, repo, "checkout", "-b", "feature/work")
	// Establish the upstream first. finishSessionGit counts unpushed commits
	// with `rev-list @{u}..`, which fails outright on a branch that tracks
	// nothing — so a branch created during the session is never pushed at all,
	// however the gate answers. That is its own defect, recorded on the todo,
	// and this test is about the gate, not about that.
	runGit(t, repo, "push", "--set-upstream", "origin", "feature/work")
	dirtyWorkTree(t, repo, "new.txt")

	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}
	summary := strings.Join(h.finishSessionGit(), "\n")

	if !remoteHas(t, remote, "feature/work") {
		t.Fatalf("feature branch never reached origin; summary was:\n%s", summary)
	}
	if !strings.Contains(summary, "Pushed to feature/work") {
		t.Fatalf("summary does not report the push:\n%s", summary)
	}
}

// The opt-out has to work, or the gate is a removal rather than a gate.
func TestPushToDefaultBranchOptInPublishesMain(t *testing.T) {
	repo, remote := newRepoWithRemote(t)
	before := remoteTip(t, remote, "main")
	dirtyWorkTree(t, repo, "new.txt")

	h := &WorkflowHooks{repoRoot: repo, autoCommit: true, pushToDefaultBranch: true}
	summary := strings.Join(h.finishSessionGit(), "\n")

	if remoteTip(t, remote, "main") == before {
		t.Fatalf("PushToDefaultBranch was set and main still did not move; summary was:\n%s", summary)
	}
}

// 57 of 78 repositories on this host have no refs/remotes/origin/HEAD. Reading
// that ref instead of asking origin would call their default branch unknown.
func TestTheDefaultBranchIsAskedOfOriginNotOfTheLocalTrackingRef(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	// git init + git remote add never writes this ref; assert that rather than
	// assume it, so the test still means something if git starts writing it.
	if err := exec.Command("git", "-C", repo, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD").Run(); err == nil {
		runGit(t, repo, "symbolic-ref", "--delete", "refs/remotes/origin/HEAD")
	}

	h := &WorkflowHooks{repoRoot: repo}
	if got := h.remoteDefaultBranch(); got != "main" {
		t.Fatalf("remoteDefaultBranch() = %q, want \"main\" — it is reading the local tracking ref, which most clones here do not have", got)
	}
}

// Unknown must refuse, not wave through. A remote that names no default branch
// is not a remote with no shared branch — an empty remote, a transport error
// and a genuinely branchless repository are indistinguishable from here.
func TestPushIsRefusedWhenOriginNamesNoDefaultBranch(t *testing.T) {
	base := t.TempDir()
	remote := filepath.Join(base, "empty.git")
	repo := filepath.Join(base, "work")
	runGit(t, base, "init", "--bare", "--initial-branch=main", remote)
	runGit(t, base, "init", "--initial-branch=main", repo)
	runGit(t, repo, "remote", "add", "origin", remote)

	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}
	if got := h.remoteDefaultBranch(); got != "" {
		t.Fatalf("a remote carrying no refs reported default branch %q", got)
	}
	refusal := h.refuseDefaultBranchPush("some-branch", "3")
	if refusal == "" {
		t.Fatal("an unknown default branch was waved through; unknown is not evidence of safe")
	}
	if !strings.Contains(refusal, "3 commit(s)") {
		t.Fatalf("the refusal does not say what is being held back:\n%s", refusal)
	}
}

// A failed `git add` means what is staged is not the session's work. Committing
// and pushing it anyway puts a partial tree on the remote under a message that
// claims to be the whole session.
func TestAFailedStageStopsTheCommitAndThePush(t *testing.T) {
	repo, remote := newRepoWithRemote(t)
	runGit(t, repo, "checkout", "-b", "feature/work")
	runGit(t, repo, "push", "--set-upstream", "origin", "feature/work")
	before := remoteTip(t, remote, "feature/work")
	dirtyWorkTree(t, repo, "new.txt")

	// A held index lock is how staging fails in the wild: another git process
	// in the same work tree, which is exactly the case an unattended session
	// sharing a repo runs into.
	lock := filepath.Join(repo, ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(lock)

	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}
	summary := strings.Join(h.finishSessionGit(), "\n")

	if !strings.Contains(summary, "git add failed") {
		t.Fatalf("a failed stage was not reported:\n%s", summary)
	}
	if remoteTip(t, remote, "feature/work") != before {
		t.Fatal("pushed after staging failed")
	}
}

// rejectingPreCommitHook makes `git commit` fail while leaving `git add`
// working, which is the split these two tests turn on. A hook that refuses is
// how a commit fails in the wild; unset user identity, a missing signing key
// and an index lock taken after the stage all arrive at the same place.
func rejectingPreCommitHook(t *testing.T, repo string) {
	t.Helper()
	hook := filepath.Join(repo, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// A failed stage is reported and stops the push; a failed COMMIT was neither.
// The stage having succeeded is what makes it dangerous: the whole work tree is
// in the index, none of it is in a commit, and the push still ran and still
// reported "✅ Pushed" — over a remote carrying earlier commits and none of the
// session's work.
func TestAFailedCommitIsReportedAndStopsThePush(t *testing.T) {
	repo, remote := newRepoWithRemote(t)
	runGit(t, repo, "checkout", "-b", "feature/work")
	runGit(t, repo, "push", "--set-upstream", "origin", "feature/work")
	// An unpushed commit, so the push would otherwise run and this test can
	// tell "held back" apart from "there was nothing to push".
	dirtyWorkTree(t, repo, "earlier.txt")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-m", "earlier work")
	before := remoteTip(t, remote, "feature/work")

	dirtyWorkTree(t, repo, "session.txt")
	rejectingPreCommitHook(t, repo)

	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}
	summary := strings.Join(h.finishSessionGit(), "\n")

	if !strings.Contains(summary, "git commit failed") {
		t.Fatalf("the commit failed and the session said nothing about it; summary was:\n%s", summary)
	}
	if strings.Contains(summary, "Pushed") {
		t.Fatalf("reported a push that does not carry the session's work:\n%s", summary)
	}
	if remoteTip(t, remote, "feature/work") != before {
		t.Fatal("pushed after the commit failed, so origin moved without the session's work")
	}
	// Refusing to publish is not refusing to keep: the work stays staged.
	if out := runGit(t, repo, "status", "--porcelain"); !strings.Contains(out, "session.txt") {
		t.Fatalf("the session's work was dropped rather than left in place:\n%s", out)
	}
}

// The complement, and the one that decides the shape of the fix: treating every
// non-zero `git commit` as a failure would pass the test above and shout on a
// case where nothing is wrong. `git commit` also exits non-zero when there is
// nothing to commit — which a second agent in a shared checkout produces by
// committing in the window between this session's stage and its commit. The
// work is recorded, so there is nothing to report and nothing to hold back.
func TestWorkCommittedByAnotherProcessIsNotReportedAsAFailure(t *testing.T) {
	repo, remote := newRepoWithRemote(t)
	runGit(t, repo, "checkout", "-b", "feature/work")
	runGit(t, repo, "push", "--set-upstream", "origin", "feature/work")
	before := remoteTip(t, remote, "feature/work")
	dirtyWorkTree(t, repo, "session.txt")

	// The concurrent agent, made deterministic: it commits from the pre-commit
	// hook, i.e. after this session staged and before this session committed.
	hook := filepath.Join(repo, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ngit commit -q --no-verify -m 'the other agent'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	h := &WorkflowHooks{repoRoot: repo, autoCommit: true}
	summary := strings.Join(h.finishSessionGit(), "\n")

	if strings.Contains(summary, "git commit failed") {
		t.Fatalf("nothing was lost — the other agent committed this work — and the session reported a failure:\n%s", summary)
	}
	if remoteTip(t, remote, "feature/work") == before {
		t.Fatalf("the push was held back over a benign empty commit; summary was:\n%s", summary)
	}
	if out := runGit(t, repo, "status", "--porcelain"); out != "" {
		t.Fatalf("work was left uncommitted:\n%s", out)
	}
}

// autoCommit is the authority to write git history. finishSessionGit used to
// ignore it, so switching auto-commit off left the larger commit still armed.
func TestAutoCommitOffLeavesGitHistoryAlone(t *testing.T) {
	repo, _ := newRepoWithRemote(t)
	before := runGit(t, repo, "rev-parse", "HEAD")
	dirtyWorkTree(t, repo, "new.txt")

	h := &WorkflowHooks{repoRoot: repo, autoCommit: false}
	if parts := h.finishSessionGit(); len(parts) != 0 {
		t.Fatalf("reported work it should not have done: %v", parts)
	}
	if runGit(t, repo, "rev-parse", "HEAD") != before {
		t.Fatal("committed with autoCommit off")
	}
	if out := runGit(t, repo, "status", "--porcelain"); out == "" {
		t.Fatal("the working tree was swept up despite autoCommit being off")
	}
}
