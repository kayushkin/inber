package server

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kayushkin/inber/engine"
)

// captureServerLog redirects the standard logger for one test and hands back a
// reader for what it collected.
//
// log.Printf is the only channel persistSessionStateLocked has. It returns
// nothing, and the writes it performs are the last thing a turn does, so a
// caller has no other way to learn that one failed. Asserting on the log is
// therefore asserting on the whole contract, not on a convenience.
//
// No test in this package calls t.Parallel(), so the global logger belongs to
// one test at a time. The assertions below match a substring rather than the
// whole buffer, so a stray line from a background goroutine can neither create
// nor destroy a result.
func captureServerLog(t *testing.T) func() string {
	t.Helper()
	var buf bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})
	return buf.String
}

// persistOneSession runs the real persist path for a session with an empty
// engine, which is all these tests need: the transcript write happens whatever
// the conversation holds.
func persistOneSession(server *Server, key string) {
	server.persistSessionState(&Session{Key: key, Engine: &engine.Engine{}})
}

// TestAFailedTranscriptWriteIsReported is the floor of noteboard card
// d347a2fe stated as the defect it closes.
//
// messages.json was written with os.WriteFile and its error discarded, while
// the two writes immediately below it in the same function both reported. So a
// full disk, a bad permission or a data directory that had gone away left the
// turn counter and the spend advancing against a transcript that was never
// written — the state session_creation.go names as the one to avoid, "a turn
// count without its transcript describes messages that are not there" — with no
// log line and no error to the caller.
//
// The failure is produced by putting a directory where messages.json goes.
// os.WriteFile then fails with EISDIR for a reason that does not depend on the
// test's uid, which a chmod would: running as root, an unwritable directory is
// still writable.
func TestAFailedTranscriptWriteIsReported(t *testing.T) {
	const key = "agent:brigid:main"
	server := &Server{config: Config{DataDir: t.TempDir()}}

	if err := os.MkdirAll(filepath.Join(dirForSessionKey(server, key), "messages.json"), 0755); err != nil {
		t.Fatalf("planting the blocker: %v", err)
	}

	readLog := captureServerLog(t)
	persistOneSession(server, key)

	if got := readLog(); !strings.Contains(got, "transcript not persisted for "+key) {
		t.Errorf("a transcript write that failed said nothing.\nlog was: %q", got)
	}
}

// TestATranscriptWriteThatSucceedsReportsNothing is the other side of the
// control, and it is why the test above means something.
//
// An assertion that a log line is present passes for two different reasons: the
// line was reported, or something in the package logs on every path. This arm
// separates them. It also stats the file afterwards, because an arm that
// silently failed to exercise the success path would satisfy the log assertion
// by doing nothing at all.
func TestATranscriptWriteThatSucceedsReportsNothing(t *testing.T) {
	const key = "agent:brigid:main"
	server := &Server{config: Config{DataDir: t.TempDir()}}

	readLog := captureServerLog(t)
	persistOneSession(server, key)

	if got := readLog(); strings.Contains(got, "transcript not persisted") {
		t.Errorf("a transcript write that succeeded reported a failure.\nlog was: %q", got)
	}
	written, err := os.ReadFile(filepath.Join(dirForSessionKey(server, key), "messages.json"))
	if err != nil {
		t.Fatalf("this arm never reached the success path, so it controls nothing: %v", err)
	}
	if len(written) == 0 {
		t.Fatal("messages.json is empty, so the write did not happen the way the other arm assumes")
	}
}

// TestAFailedSessionDirIsReported covers the second discarded error in the same
// function. MkdirAll's failure is the more total one — none of the three
// records can land after it — and it was equally silent.
//
// DataDir is placed underneath a regular file, so MkdirAll fails with ENOTDIR.
func TestAFailedSessionDirIsReported(t *testing.T) {
	const key = "agent:brigid:main"
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("planting the blocker: %v", err)
	}
	server := &Server{config: Config{DataDir: filepath.Join(blocker, "data")}}

	readLog := captureServerLog(t)
	persistOneSession(server, key)

	if got := readLog(); !strings.Contains(got, "session dir not created for "+key) {
		t.Errorf("a session directory that could not be created said nothing.\nlog was: %q", got)
	}
}
