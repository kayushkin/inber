package guard

import (
	"sync"
	"testing"
)

// TestTheSpendCeilingRecordSurvivesConcurrentAccess drives the two goroutines
// that reach a live Guard in production and asserts, under -race, that they do
// not collide.
//
// The pair is not hypothetical. The turn goroutine records against the totals
// with no session lock held — server.Session.turn releases s.mu before
// engine.Engine.RunTurn, so RecordTurn and RecordCost run outside every lock
// the server has — while an HTTP goroutine reads the same fields through
// State(), from persistSessionState, and writes a cap through
// SetMaxInputTokens from the session-update handler.
//
// What makes it worth a test rather than a comment: State() is the record the
// spend ceiling is rebuilt from. It is persisted per session and installed back
// on every rebuild so that a rebuilt session cannot get its budget back. A torn
// read writes a total the session never held, and a session handed back part of
// its budget is the failure the ceiling exists to prevent.
//
// Without the mutex on Guard this fails on every run under -race. It asserts no
// ordering: which turn's cost a concurrent State() catches is genuinely up to
// the scheduler. It asserts only that the totals are read and written whole,
// and that the final total is every dollar recorded rather than whatever
// survived the last lost update.
func TestTheSpendCeilingRecordSurvivesConcurrentAccess(t *testing.T) {
	g := New(Config{Mode: Assist, MaxCost: 1_000_000, MaxInputTokens: 1_000_000})

	const turns = 500
	var writers, readers sync.WaitGroup
	stop := make(chan struct{})

	// The turn goroutine: engine.RunTurn's record step, then the cost that
	// turn_postprocess adds after it.
	writers.Add(1)
	go func() {
		defer writers.Done()
		for i := 0; i < turns; i++ {
			g.RecordTurn(10)
			g.RecordCost(0.01)
			g.CheckLimits()
		}
	}()

	// The HTTP goroutines: persistSessionState reading the record the ceiling
	// is rebuilt from, and the session-update handler lowering a cap mid-turn.
	for i := 0; i < 3; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				state := g.State()
				// A turn that has been counted must never come back with a
				// cost of zero: the two are written by the same goroutine one
				// after the other, so the only way to read a positive turn
				// count beside no cost at all is a read that tore.
				if state.Turns > 1 && state.Cost == 0 {
					t.Errorf("read %d turns recorded but $0 spent, so the record tore",
						state.Turns)
					return
				}
				g.CostSoFar()
				g.ElapsedSeconds()
				g.Mode()
				g.SetMaxInputTokens(1_000_000)
			}
		}()
	}

	writers.Wait()
	close(stop)
	readers.Wait()

	if got := g.State(); got.Turns != turns {
		t.Errorf("recorded %d turns, want %d — updates were lost", got.Turns, turns)
	}
	if got := g.CostSoFar(); got < 0.01*turns-1e-6 {
		t.Errorf("recorded $%.4f, want $%.4f — updates were lost", got, 0.01*turns)
	}
}

// TestCheckToolDoesNotHoldTheLockWhileAskingTheApprover pins the one place the
// mutex could deadlock a session instead of protecting it.
//
// ApprovalFunc is caller code. It is entitled to ask the guard what mode it is
// enforcing, or how much the session has spent, before answering — and if
// CheckTool asked it while holding mu, that question would block on the lock
// its own caller holds, wedging the turn for good on a non-reentrant Mutex.
func TestCheckToolDoesNotHoldTheLockWhileAskingTheApprover(t *testing.T) {
	var g *Guard
	g = New(Config{
		Mode: Assist,
		ApprovalFunc: func(tool, input string) bool {
			// Both of these take mu. Reaching the return means CheckTool let
			// go of it first.
			return g.Mode() == Assist && g.CostSoFar() == 0
		},
	})

	if verdict := g.CheckTool("shell_commands", "ls"); verdict != Allowed {
		t.Errorf("CheckTool = %v, want Allowed", verdict)
	}
}

// TestRestoreStateRacingAReadIsSafe covers the duration cap, whose two fields
// are written by RestoreState on a rebuild and read by ElapsedSeconds from
// wherever the record is being taken. It is the same defect as a torn total,
// on the cap that bounds how long a session may go on running.
func TestRestoreStateRacingAReadIsSafe(t *testing.T) {
	g := New(Config{MaxDuration: 1_000_000})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			if err := g.RestoreState(State{MaxDuration: 1_000_000, ElapsedSeconds: 590}); err != nil {
				t.Errorf("RestoreState: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			if got := g.ElapsedSeconds(); got < 0 {
				t.Errorf("ElapsedSeconds = %d, which no clock can produce", got)
				return
			}
			g.CheckLimits()
		}
	}()
	wg.Wait()
}
