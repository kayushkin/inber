package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// persistSessionState rewrites messages.json at the end of every turn. So a
// read failure that is swallowed does not merely lose the transcript for this
// turn — the next save writes a conversation that starts empty over the file we
// could not read, and the session's history is gone. loadPersistedSession used
// to return (nil, 0) for every failure, including a corrupt file, which made
// "this session has no history yet" and "this session's history is unreadable"
// the same answer to its caller.

func TestASessionWithNothingPersistedIsNotAnError(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}

	msgs, turnCounter, err := server.loadPersistedSession("agent:brigid:main")

	if err != nil {
		t.Fatalf("a session on its first turn has no transcript, and that is ordinary: %v", err)
	}
	if len(msgs) != 0 || turnCounter != 0 {
		t.Fatalf("want no messages and turn 0, got %d messages at turn %d", len(msgs), turnCounter)
	}
}

func TestAnUnreadableTranscriptIsReportedRatherThanTreatedAsEmpty(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}
	const key = "agent:brigid:main"

	dir := dirForSessionKey(server, key)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("make session dir: %v", err)
	}
	// A transcript that exists and is not JSON — a truncated write, a half-
	// finished copy, a disk that filled mid-save.
	if err := os.WriteFile(filepath.Join(dir, "messages.json"), []byte(`[{"role":"user"`), 0644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	msgs, _, err := server.loadPersistedSession(key)

	if err == nil {
		t.Fatal("a corrupt transcript read as an empty one; the next save would overwrite it")
	}
	if len(msgs) != 0 {
		t.Errorf("no messages should come back from a failed read, got %d", len(msgs))
	}
	if !strings.Contains(err.Error(), key) {
		t.Errorf("the error should name the session it is about: %v", err)
	}
	if !strings.Contains(err.Error(), "messages.json") {
		t.Errorf("the error should name the file to move aside: %v", err)
	}
}

func TestAPersistedTranscriptIsReadBack(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}
	const key = "agent:brigid:main"

	dir := dirForSessionKey(server, key)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("make session dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "messages.json"),
		[]byte(`[{"role":"user","content":[{"type":"text","text":"hello"}]}]`), 0644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	msgs, _, err := server.loadPersistedSession(key)

	if err != nil {
		t.Fatalf("loadPersistedSession: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want the one persisted message, got %d", len(msgs))
	}
}
