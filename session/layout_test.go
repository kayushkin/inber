package session

import (
	"path/filepath"
	"strings"
	"testing"
)

// New appends the agent segment to the root it is handed. That is the whole
// layout contract, and until now it lived only in a doc comment — so when
// engine's setupSession appended the segment as well, nothing failed and every
// engine session on this box was written to logs/<agent>/<agent>/ instead.
//
// These tests pin the contract on the side that owns it. A future caller that
// pre-joins is then fixing the caller, not choosing between two defensible
// readings of an undocumented function.

func TestNewOwnsTheAgentSegment(t *testing.T) {
	root := t.TempDir()

	session, err := New(root, "", "testagent", "", nil)
	if err != nil {
		t.Fatalf("New failed on a writable root: %v", err)
	}
	defer session.Close()

	rel, err := filepath.Rel(root, session.FilePath())
	if err != nil {
		t.Fatalf("log file %q is not under the root %q: %v", session.FilePath(), root, err)
	}

	segments := strings.Split(rel, string(filepath.Separator))
	if len(segments) != 3 {
		t.Fatalf("want <root>/<agent>/<session>/session.jsonl, got %q", rel)
	}
	if segments[0] != "testagent" {
		t.Errorf("New did not put the log under its agent segment: %q", rel)
	}
	if segments[2] != "session.jsonl" {
		t.Errorf("unexpected log file name: %q", rel)
	}
	// The session segment is named for the session, not the agent again. A
	// caller that pre-joins produces exactly this shape one level up, so the
	// check that catches the doubling has to be the repeated name, not the
	// depth.
	if segments[1] == "testagent" {
		t.Errorf("agent name repeated as the session directory: %q", rel)
	}
}

// The counterweight: an empty agent name means no segment at all, so a caller
// with no agent is not silently given one. Without this, "always append
// something" passes the test above.
func TestNewAddsNoAgentSegmentWithoutAnAgentName(t *testing.T) {
	root := t.TempDir()

	session, err := New(root, "", "", "", nil)
	if err != nil {
		t.Fatalf("New failed on a writable root: %v", err)
	}
	defer session.Close()

	rel, err := filepath.Rel(root, session.FilePath())
	if err != nil {
		t.Fatalf("log file %q is not under the root %q: %v", session.FilePath(), root, err)
	}

	segments := strings.Split(rel, string(filepath.Separator))
	if len(segments) != 2 {
		t.Fatalf("want <root>/<session>/session.jsonl for an unnamed agent, got %q", rel)
	}
	if segments[1] != "session.jsonl" {
		t.Errorf("unexpected log file name: %q", rel)
	}
}

// Two agents under one root get one directory each. This is the property the
// root argument exists for, and it is what a caller destroys by pre-joining:
// pass logs/<agent> per agent and the roots stop being shared, which is why the
// doubling was invisible to everything except the paths on disk.
func TestNewSeparatesAgentsUnderOneRoot(t *testing.T) {
	root := t.TempDir()

	first, err := New(root, "", "claxon", "", nil)
	if err != nil {
		t.Fatalf("New failed for claxon: %v", err)
	}
	defer first.Close()

	second, err := New(root, "", "fionn", "", nil)
	if err != nil {
		t.Fatalf("New failed for fionn: %v", err)
	}
	defer second.Close()

	claxonDir := filepath.Join(root, "claxon")
	fionnDir := filepath.Join(root, "fionn")
	if !strings.HasPrefix(first.FilePath(), claxonDir+string(filepath.Separator)) {
		t.Errorf("claxon's log is not under %q: %q", claxonDir, first.FilePath())
	}
	if !strings.HasPrefix(second.FilePath(), fionnDir+string(filepath.Separator)) {
		t.Errorf("fionn's log is not under %q: %q", fionnDir, second.FilePath())
	}
}

// LogsRoot is the one join above the segment New owns, and it exists because
// the writer and the server's history reader both make it. Pinning it here
// pins the whole path a session is written to and read back from: a change to
// either half that this test survives is a change both halves made.
func TestLogsRootIsTheRootNewIsHandedForARepository(t *testing.T) {
	repoRoot := t.TempDir()

	session, err := New(LogsRoot(repoRoot), "", "claxon", "", nil)
	if err != nil {
		t.Fatalf("New failed under the logs root of a writable repository: %v", err)
	}
	defer session.Close()

	rel, err := filepath.Rel(repoRoot, session.FilePath())
	if err != nil {
		t.Fatalf("log file %q is not under the repository %q: %v", session.FilePath(), repoRoot, err)
	}

	segments := strings.Split(rel, string(filepath.Separator))
	if len(segments) != 4 {
		t.Fatalf("want <repo>/logs/<agent>/<session>/session.jsonl, got %q", rel)
	}
	if segments[0] != "logs" {
		t.Errorf("a session is not written under the repository's logs directory: %q", rel)
	}
	if segments[1] != "claxon" {
		t.Errorf("the agent segment is not directly under the logs root: %q", rel)
	}
}
