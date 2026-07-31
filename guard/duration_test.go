package guard

import (
	"testing"
	"time"
)

// MaxDuration was declared on Config, listed in this package's own doc comment
// as one of the four limits that "enforce hard stops when agents exceed
// budgets", and enforced by nothing: CheckLimits carried a TODO where the
// comparison belonged, and the field it was to compare against was never
// written by anyone. So a session given a wall-clock cap ran without one, and
// ran without one silently — guard.Config documents 0 as unlimited, so a cap
// that was never checked is indistinguishable from a caller who wanted none.
//
// These tests are the enforcement half. The wiring half — that a cap asked for
// over the API reaches this package at all — is pinned in
// engine/max_duration_wiring_test.go and server/max_duration_request_test.go.

func TestASessionIsRefusedOnceItPassesItsDurationCap(t *testing.T) {
	g := New(Config{Mode: Autonomous, MaxDuration: 1})

	if exceeded, reason := g.CheckLimits(); exceeded {
		t.Fatalf("a session capped at 1s was refused before any time passed: %s", reason)
	}

	time.Sleep(1100 * time.Millisecond)

	exceeded, reason := g.CheckLimits()
	if !exceeded {
		t.Fatalf("a session capped at 1s had been running %ds and was allowed to continue",
			g.ElapsedSeconds())
	}
	if reason != "max duration exceeded" {
		t.Errorf("the turn was refused for %q, want %q — a caller reading the reason "+
			"would be told the wrong limit stopped it", reason, "max duration exceeded")
	}
}

// TestAnUncappedSessionIsNeverRefusedForDuration is the complement, and it is
// the one that keeps the fix from being worse than the defect. Almost no
// session sets max_duration. If an unset cap were treated as zero seconds
// rather than as unlimited, every one of those sessions would be refused at its
// first turn.
func TestAnUncappedSessionIsNeverRefusedForDuration(t *testing.T) {
	g := New(Config{Mode: Autonomous})
	g.secondsBeforeThisRun = 100 * 365 * 24 * 3600 // a century of it

	if exceeded, reason := g.CheckLimits(); exceeded {
		t.Fatalf("a session with no duration cap was refused after %ds: %s", g.ElapsedSeconds(), reason)
	}
}

// TestTheDurationClockStartsWhenTheGuardIsBuilt pins the field the original
// TODO could not have worked without. A start time nobody writes leaves the
// zero instant in it, and every elapsed reading is then the age of the calendar
// — which trips any cap immediately, and would have made the check look
// enforced while refusing every session on its first turn.
func TestTheDurationClockStartsWhenTheGuardIsBuilt(t *testing.T) {
	g := New(Config{Mode: Autonomous, MaxDuration: 3600})

	if elapsed := g.ElapsedSeconds(); elapsed > 5 {
		t.Fatalf("a guard built just now reports %ds elapsed — the clock is not being started at New", elapsed)
	}
	if exceeded, reason := g.CheckLimits(); exceeded {
		t.Fatalf("a session capped at an hour was refused on its first turn: %s", reason)
	}
}

// TestARebuiltSessionKeepsTheTimeItAlreadySpent is the duration form of the
// rule the dollar total already follows: a cap whose running total restarts
// every time the session is rebuilt is a cap per rebuild, not per session.
func TestARebuiltSessionKeepsTheTimeItAlreadySpent(t *testing.T) {
	original := New(Config{Mode: Autonomous, MaxDuration: 600})
	original.secondsBeforeThisRun = 595

	rebuilt := New(Config{Mode: Autonomous})
	if err := rebuilt.RestoreState(ResumeState(original.State(), rebuilt.State())); err != nil {
		t.Fatalf("RestoreState: %v", err)
	}

	if got := rebuilt.State().MaxDuration; got != 600 {
		t.Errorf("the rebuilt session's duration cap is %ds, want 600s — a capped session came back uncapped", got)
	}
	if got := rebuilt.ElapsedSeconds(); got < 595 {
		t.Errorf("the rebuilt session reports %ds elapsed, want at least 595s — "+
			"a rebuild handed the whole ten minutes back", got)
	}
	if exceeded, _ := rebuilt.CheckLimits(); exceeded {
		t.Errorf("a session 595s into a 600s cap was refused on rebuild")
	}

	rebuilt.secondsBeforeThisRun = 600
	if exceeded, reason := rebuilt.CheckLimits(); !exceeded {
		t.Errorf("a rebuilt session past its restored duration cap was allowed to continue")
	} else if reason != "max duration exceeded" {
		t.Errorf("refused for %q, want %q", reason, "max duration exceeded")
	}
}

// TestRestoringElapsedOverwritesRatherThanAdds pins the contract RestoreState
// states for every other total: it puts a session back where it was, so the
// time this rebuild has been alive is not added on top of the record.
func TestRestoringElapsedOverwritesRatherThanAdds(t *testing.T) {
	g := New(Config{Mode: Autonomous, MaxDuration: 600})
	g.secondsBeforeThisRun = 400

	if err := g.RestoreState(State{Mode: "autonomous", MaxDuration: 600, ElapsedSeconds: 60}); err != nil {
		t.Fatalf("RestoreState: %v", err)
	}

	if got := g.ElapsedSeconds(); got > 65 {
		t.Errorf("after restoring a 60s record the guard reports %ds — the restore added to the "+
			"totals it was meant to replace", got)
	}
}
