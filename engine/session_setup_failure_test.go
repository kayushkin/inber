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
	// Assert on the full logs directory, agent segment included. The wrapped
	// error from sessionMod.New already names the parent it failed to mkdir,
	// so asserting on <root>/logs alone passes even when setupSession stops
	// reporting a path at all — the agent segment is the part only this
	// function knows.
	if !strings.Contains(err.Error(), filepath.Join(root, "logs", "testagent")) {
		t.Errorf("error does not name the path that failed: %v", err)
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
