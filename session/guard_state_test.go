package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kayushkin/inber/guard"
)

// TestGuardStateSurvivesTheRoundTrip. The whole point of the sidecar is that a
// process that did not run the session can read back what the session was
// allowed to spend and what it had spent.
func TestGuardStateSurvivesTheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	written := guard.State{
		MaxTurns: 7, MaxInputTokens: 90_000, MaxCost: 5.00,
		Turns: 12, InputTokens: 400_000, Cost: 4.80,
	}

	if err := SaveGuardState(dir, written); err != nil {
		t.Fatalf("SaveGuardState: %v", err)
	}
	read, err := LoadGuardState(dir)
	if err != nil {
		t.Fatalf("LoadGuardState: %v", err)
	}

	if read != written {
		t.Errorf("read back %+v, want %+v", read, written)
	}
}

// TestGuardStateWritesADirectoryThatIsNotThereYet — the first thing persisted
// for a session may be persisted before anything has created its directory.
func TestGuardStateWritesADirectoryThatIsNotThereYet(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions", "agent:brigid:main")

	if err := SaveGuardState(dir, guard.State{MaxCost: 5.00}); err != nil {
		t.Fatalf("SaveGuardState into a missing directory: %v", err)
	}
	if _, err := LoadGuardState(dir); err != nil {
		t.Fatalf("LoadGuardState: %v", err)
	}
}

// TestNoGuardStateFileIsNotAnError. A session that has never been persisted has
// no sidecar, and neither has one written before this file existed. Both mean
// nothing is known, which is the zero State.
func TestNoGuardStateFileIsNotAnError(t *testing.T) {
	state, err := LoadGuardState(t.TempDir())
	if err != nil {
		t.Fatalf("a missing sidecar reported an error: %v", err)
	}
	if state != (guard.State{}) {
		t.Errorf("a missing sidecar produced %+v, want the zero State", state)
	}
}

// TestAnUnreadableGuardStateIsAnError is the half that must not be quiet. A
// sidecar that exists and will not parse is a cap that was recorded and then
// lost, and returning the zero State for it is exactly the "resumed with no cap
// and no error" behaviour this file was written to end.
func TestAnUnreadableGuardStateIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, guardStateFileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadGuardState(dir); err == nil {
		t.Error("a corrupt sidecar loaded without complaint — a lost cap has to be loud")
	}
}
