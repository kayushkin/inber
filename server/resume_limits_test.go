package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/guard"
	sessionMod "github.com/kayushkin/inber/session"
)

// dirForSessionKey is where persistSessionState and restoreGuardState agree a
// session's state lives. The tests below write and read through the real
// functions, and only need this to place a record by hand.
func dirForSessionKey(g *Server, key string) string {
	return filepath.Join(g.config.DataDir, "sessions", key)
}

// TestResumingASessionKeepsTheCapItWasGiven is this fix stated as the defect it
// closes.
//
// Three callers of createSession pass a zero RunRequest, and every safety limit
// the server has is a field of that request. The one that matters most is the
// resume in handleBridgeResume: a session that is asked for and is no longer
// in memory is rebuilt from its persisted messages, with no request to copy
// limits from. Because guard.Config reads a zero cap as unlimited, a session
// created with a $5.00 cap came back from that rebuild with no cap, no log line
// and no error.
//
// It runs both real functions — persistSessionState writes and
// restoreGuardState reads — but stops short of the HTTP handler, because
// standing up an engine through NewEngine needs a workspace and a model client.
// What it does cover is every piece of state that crosses the process boundary.
func TestResumingASessionKeepsTheCapItWasGiven(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}
	const key = "agent:brigid:main"

	// A session created with limits, part way through spending them.
	original := guard.New(guard.Config{Mode: guard.Autonomous, MaxTurns: 7, MaxInputTokens: 90_000, MaxCost: 5.00})
	original.RecordTurn(400_000)
	original.RecordCost(4.80)
	server.persistSessionState(&Session{Key: key, Engine: &engine.Engine{Guard: original}})

	// The rebuild. A zero RunRequest configures no caps at all, so this is the
	// guard the resume path builds.
	rebuilt := guard.New(guard.Config{Mode: guard.Autonomous})
	server.restoreGuardState(key, rebuilt)

	state := rebuilt.State()
	if state.MaxCost != 5.00 {
		t.Errorf("the resumed session's cost cap is $%.2f, want $5.00 — a capped session came back uncapped", state.MaxCost)
	}
	if state.MaxTurns != 7 {
		t.Errorf("the resumed session's turn cap is %d, want 7", state.MaxTurns)
	}
	if state.MaxInputTokens != 90_000 {
		t.Errorf("the resumed session's input token cap is %d, want 90000", state.MaxInputTokens)
	}

	// And the cap has to still be the cap it was, not a fresh one: the $4.80
	// already spent under it comes back too.
	if state.Cost != 4.80 {
		t.Errorf("the resumed session has spent $%.2f, want $4.80 — the budget was handed back whole", state.Cost)
	}
	rebuilt.RecordCost(0.25)
	if exceeded, reason := rebuilt.CheckLimits(); !exceeded {
		t.Errorf("$5.05 spent against a $5.00 cap did not stop the resumed session (reason %q)", reason)
	}
}

// TestCreatingASessionThatHasNoRecordChangesNothing. restoreGuardState runs on
// every createSession, not only the resume, so a first-time creation and a
// spawned or forked child — neither of which has a record under its own key —
// must come out of it exactly as they were configured.
func TestCreatingASessionThatHasNoRecordChangesNothing(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}

	fresh := guard.New(guard.Config{Mode: guard.Autonomous, MaxCost: 3.00})
	server.restoreGuardState("agent:brigid:child-1", fresh)

	state := fresh.State()
	if state.MaxCost != 3.00 {
		t.Errorf("a session with no record came back with MaxCost $%.2f, want the $3.00 it was configured with", state.MaxCost)
	}
	if state.Turns != 0 || state.InputTokens != 0 || state.Cost != 0 {
		t.Errorf("a session with no record came back having already spent %+v", state)
	}
}

// TestACorruptRecordLeavesTheRebuildAsConfigured. A sidecar that will not parse
// must not take the configured caps down with it — an unreadable record is a
// reason to run under what this rebuild asked for, never a reason to run under
// nothing.
func TestACorruptRecordLeavesTheRebuildAsConfigured(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}
	const key = "agent:brigid:main"

	dir := dirForSessionKey(server, key)
	if err := sessionMod.SaveGuardState(dir, guard.State{MaxCost: 5.00}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionMod.GuardStatePath(dir), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	rebuilt := guard.New(guard.Config{Mode: guard.Autonomous, MaxCost: 2.00})
	server.restoreGuardState(key, rebuilt)

	if got := rebuilt.State().MaxCost; got != 2.00 {
		t.Errorf("MaxCost = $%.2f after an unreadable record, want the $2.00 this rebuild configured", got)
	}
}

// TestRestoringOntoASessionWithNoGuardDoesNotPanic. Engine.Guard is a field a
// caller can find nil, and the rebuild path hands it over unchecked.
func TestRestoringOntoASessionWithNoGuardDoesNotPanic(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}
	server.restoreGuardState("agent:brigid:main", nil)
}
