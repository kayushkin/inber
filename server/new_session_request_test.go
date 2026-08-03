package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kayushkin/inber/engine"
	sessionMod "github.com/kayushkin/inber/session"
)

// A fresh session is the third field RunRequest advertised and the server threw
// away before the engine could act on it — after max_cost and mode. It is the
// one whose symptom is invisible, because the caller gets a 200 and a working
// session; it is just the wrong conversation.
//
// Two links carry `new_session` from the API to a session that is actually new,
// and both were broken. The engine link decides whether setupSession clears the
// workspace transcript or loads it. The server link decides whether the
// key-scoped transcript on disk is read back onto the new engine. Fixing either
// alone leaves the conversation in the other.

func TestAFreshSessionRequestReachesTheEngine(t *testing.T) {
	var cfg engine.EngineConfig

	applyRequestOverrides(&cfg, RunRequest{NewSession: true})

	if !cfg.NewSession {
		t.Error("a request asking for a fresh session produced an engine config that continues " +
			"the old one — setupSession loads the workspace transcript instead of clearing it")
	}
}

func TestAnOrdinaryRequestDoesNotAskForAFreshSession(t *testing.T) {
	var cfg engine.EngineConfig

	applyRequestOverrides(&cfg, RunRequest{})

	if cfg.NewSession {
		t.Error("an empty request asked for a fresh session, which would clear the workspace " +
			"transcript of every ordinary turn")
	}
}

// TestAFreshSessionDoesNotOpenWithThePreviousTranscript is the server half. The
// key is deliberately the one `run` reuses for a fresh session: the whole defect
// is that the replacement lands on the same key as the conversation it replaced.
func TestAFreshSessionDoesNotOpenWithThePreviousTranscript(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}
	key := mainSessionKey("brigid")
	writePersistedTranscript(t, server, key)

	msgs, turnCounter, err := server.transcriptToStartSessionFrom(key, RunRequest{NewSession: true})

	if err != nil {
		t.Fatalf("transcriptToStartSessionFrom: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("a fresh session opened holding %d message(s) of the conversation it replaced", len(msgs))
	}
	if turnCounter != 0 {
		t.Errorf("a fresh session opened at turn %d, carrying the replaced conversation's count", turnCounter)
	}
}

// The control. Resuming is the ordinary path and by far the common one, so the
// fix has to be readable as "new_session and nothing else": a test that only
// asserted the fresh case would pass just as well against a server that had
// stopped resuming altogether.
func TestAnOrdinarySessionStillOpensWithItsPersistedTranscript(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}
	key := mainSessionKey("brigid")
	writePersistedTranscript(t, server, key)

	msgs, turnCounter, err := server.transcriptToStartSessionFrom(key, RunRequest{})

	if err != nil {
		t.Fatalf("transcriptToStartSessionFrom: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("want the one persisted message, got %d", len(msgs))
	}
	if turnCounter != 7 {
		t.Errorf("want the persisted turn count 7, got %d", turnCounter)
	}
}

// A fresh session skips the read, so it must also skip the read's failure. An
// unreadable transcript is a reason to refuse to resume; it is not a reason to
// refuse to start something new that will never look at it.
func TestAFreshSessionIsNotBlockedByAnUnreadableTranscript(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}
	key := mainSessionKey("brigid")
	dir := dirForSessionKey(server, key)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("make session dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "messages.json"), []byte(`[{"role":"user"`), 0644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	if _, _, err := server.transcriptToStartSessionFrom(key, RunRequest{NewSession: true}); err != nil {
		t.Errorf("a fresh session was refused over a transcript it does not read: %v", err)
	}
	if _, _, err := server.transcriptToStartSessionFrom(key, RunRequest{}); err == nil {
		t.Error("the control: a resume over the same unreadable transcript must still be refused")
	}
}

func writePersistedTranscript(t *testing.T, server *Server, key string) {
	t.Helper()
	dir := dirForSessionKey(server, key)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("make session dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "messages.json"),
		[]byte(`[{"role":"user","content":[{"type":"text","text":"the conversation being replaced"}]}]`), 0644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	if err := sessionMod.SaveTurnCounter(dir, 7); err != nil {
		t.Fatalf("write turn counter: %v", err)
	}
}
