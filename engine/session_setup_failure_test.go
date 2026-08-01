package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sessionMod "github.com/kayushkin/inber/session"
)

// setupSession used to answer an unwritable logs directory with a zero-value
// Session and a warning. That object is not a degraded logger, it is a booby
// trap: its json encoder and its file handle are both nil, so the first
// LogUser — which runs on every turn — dereferences nil and takes the process
// down, several frames away from the failure that caused it.
//
// These two tests pin the pair of facts that make the loud failure the only
// honest answer: the fallback panics, and setupSession no longer builds one.

// blockedLogsRepoRoot returns a repo root whose logs path is occupied by a
// regular file, so creating the log directory fails with ENOTDIR. A permission
// bit would not do: the test suite can run as a user that ignores it.
func blockedLogsRepoRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "logs"), []byte("not a directory"), 0644); err != nil {
		t.Fatalf("seed blocked logs path: %v", err)
	}
	return root
}

func TestZeroValueSessionPanicsOnItsCommonestCall(t *testing.T) {
	// Guards the reasoning, not the caller: if Session ever becomes safe at its
	// zero value, this fails and the comment in setupSession needs rereading.
	for _, tc := range []struct {
		name string
		call func(*sessionMod.Session)
	}{
		{"LogUser", func(s *sessionMod.Session) { s.LogUser("hello") }},
		{"LogToolResult", func(s *sessionMod.Session) { s.LogToolResult("id", "shell", "out", true) }},
		{"FilePath", func(s *sessionMod.Session) { _ = s.FilePath() }},
		{"Close", func(s *sessionMod.Session) { s.Close() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("a zero-value Session survived %s; it is safe now, so setupSession's comment is stale", tc.name)
				}
			}()
			tc.call(&sessionMod.Session{})
		})
	}
}

func TestSetupSessionFailsLoudlyWhenTheLogDirectoryCannotBeCreated(t *testing.T) {
	root := blockedLogsRepoRoot(t)

	session, _, _, _, _, err := setupSession(root, "testagent", "chat", true, true)
	if err == nil {
		t.Fatalf("setupSession succeeded against an unwritable logs path; it returned session %#v", session)
	}
	if session != nil {
		t.Errorf("setupSession returned a session alongside its error: %#v", session)
	}
	if !strings.Contains(err.Error(), "create session logger") {
		t.Errorf("error does not name what failed: %v", err)
	}
	// Assert on the message and the path as ONE CONTIGUOUS STRING, because a
	// bare path is not enough: os.MkdirAll reports the first component that is
	// not a directory rather than the path it was asked for, so the wrapped
	// error from sessionMod.New says "mkdir <root>/logs" however deep the
	// target was, and an assertion on <root>/logs alone is satisfied by the
	// callee even when setupSession reports no path at all. The "create session
	// logger in " prefix is the part only this function writes.
	//
	// This assertion used to be the bare filepath.Join(root, "logs", "testagent"),
	// which pinned the outer frame correctly for as long as setupSession
	// appended the agent segment itself — that was the one part of the path the
	// callee's error could not contain. Now that sessionMod.New owns the
	// segment, setupSession reports <root>/logs and no frame names the agent,
	// so the old assertion does not go vacuous, it goes red.
	if want := "create session logger in " + filepath.Join(root, "logs"); !strings.Contains(err.Error(), want) {
		t.Errorf("error does not name the path that failed, want %q in: %v", want, err)
	}
}

// TestSetupSessionWritesExactlyOneAgentSegment pins the layout itself, which no
// test covered while it was wrong: setupSession appended the agent name to the
// logs root and sessionMod.New appended it again, so every engine session on
// this box landed in logs/<agent>/<agent>/. Both joins read as correct alone,
// and the sessions on disk are the only place the pair was visible.
func TestSetupSessionWritesExactlyOneAgentSegment(t *testing.T) {
	root := t.TempDir()

	session, _, _, _, _, err := setupSession(root, "testagent", "chat", true, true)
	if err != nil {
		t.Fatalf("setupSession failed on a writable root: %v", err)
	}
	defer session.Close()

	// <root>/logs/testagent/<session id>/session.jsonl — one agent segment, and
	// the session directory named for the session rather than the agent again.
	got := session.FilePath()
	rel, relErr := filepath.Rel(root, got)
	if relErr != nil {
		t.Fatalf("session log %q is not under the repo root %q: %v", got, root, relErr)
	}
	segments := strings.Split(rel, string(filepath.Separator))
	if len(segments) != 4 {
		t.Fatalf("want <root>/logs/<agent>/<session>/session.jsonl, got %q", rel)
	}
	if segments[0] != "logs" || segments[1] != "testagent" || segments[3] != "session.jsonl" {
		t.Errorf("unexpected log layout: %q", rel)
	}
	if segments[2] == "testagent" {
		t.Errorf("the agent name is joined twice — logs/%s/%s: %q", segments[1], segments[2], rel)
	}

	// Belt and braces, because the segment check above passes if the doubling
	// ever moves to a different depth: the agent name must appear once.
	if n := strings.Count(rel, "testagent"); n != 1 {
		t.Errorf("agent name appears %d times in %q, want 1", n, rel)
	}
}

func TestSetupSessionStillSucceedsOnAWritableRoot(t *testing.T) {
	// The counterweight: the failure above must be caused by the blocked path,
	// not by setupSession refusing every temp root.
	session, _, _, _, _, err := setupSession(t.TempDir(), "testagent", "chat", true, true)
	if err != nil {
		t.Fatalf("setupSession failed on a writable root: %v", err)
	}
	if session == nil {
		t.Fatal("setupSession returned no session and no error")
	}
	session.Close()
}
