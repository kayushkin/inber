package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTurnCounterRoundTrip(t *testing.T) {
	dir := t.TempDir()

	if err := SaveTurnCounter(dir, 47); err != nil {
		t.Fatalf("SaveTurnCounter: %v", err)
	}
	got, err := LoadTurnCounter(dir)
	if err != nil {
		t.Fatalf("LoadTurnCounter: %v", err)
	}
	if got != 47 {
		t.Fatalf("counter = %d, want 47", got)
	}
}

// A session that has never been resumed has no sidecar, and neither does one
// persisted before the sidecar existed. Both mean no turns are known to have
// run, which is what a fresh engine already assumes.
func TestLoadTurnCounterMissingFileIsZeroNotAnError(t *testing.T) {
	got, err := LoadTurnCounter(t.TempDir())
	if err != nil {
		t.Fatalf("missing file reported as an error: %v", err)
	}
	if got != 0 {
		t.Fatalf("counter = %d, want 0", got)
	}
}

// A count that was recorded and then became unreadable is a different thing
// from one that was never recorded, and the caller has to be able to say so.
func TestLoadTurnCounterUnreadableFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, turnCounterFileName), []byte("{"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := LoadTurnCounter(dir)
	if err == nil {
		t.Fatal("a truncated sidecar loaded silently as turn 0")
	}
	if got != 0 {
		t.Fatalf("counter = %d, want 0 alongside the error", got)
	}
}

// The count only means anything as a description of a particular transcript.
// Left behind by a cleared one it tells the next session it is deep into a
// conversation that has no messages in it.
func TestClearMessagesAlsoClearsTheTurnCount(t *testing.T) {
	ws := &Workspace{Dir: t.TempDir()}
	if err := ws.SaveMessages([]byte("[]")); err != nil {
		t.Fatalf("SaveMessages: %v", err)
	}
	if err := SaveTurnCounter(ws.Dir, 47); err != nil {
		t.Fatalf("SaveTurnCounter: %v", err)
	}

	ws.ClearMessages()

	got, err := LoadTurnCounter(ws.Dir)
	if err != nil {
		t.Fatalf("LoadTurnCounter: %v", err)
	}
	if got != 0 {
		t.Fatalf("counter = %d after ClearMessages, want 0 — the count outlived its transcript", got)
	}
}
