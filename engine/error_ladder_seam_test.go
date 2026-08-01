package engine

import (
	"testing"

	sessionMod "github.com/kayushkin/inber/session"
)

// The error-recovery ladder in contextBudget reads Turn.ConsecutiveErrors, and
// that counter has exactly one writer: the OnToolResult hook built here. This
// pins the seam between the two — one error-flagged tool result is one rung.
//
// The other half of the chain is in agent/chain_error_count_test.go, which pins
// that one tool_use block produces at most one error-flagged result however
// much rode along inside its arguments. Together they are what stops an
// ordinary policy refusal from recalling memory as if the model were stuck: a
// block that failed once widens recall from 6,000 tokens to 20,000, not to
// 35,000, and does not rewrite the cached prompt prefix a second time.
func engineWithSession(t *testing.T) *Engine {
	t.Helper()
	s, err := sessionMod.New(t.TempDir(), "test-model", "tester", "", nil)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	t.Cleanup(s.Close)
	return &Engine{Session: s}
}

func TestOneErrorFlaggedToolResultIsOneRungOfTheRecoveryLadder(t *testing.T) {
	e := engineWithSession(t)
	hooks := e.buildHooks()
	if hooks.OnToolResult == nil {
		t.Fatal("buildHooks left OnToolResult unwired, so the recovery ladder can never move")
	}

	if e.Turn.ConsecutiveErrors != 0 {
		t.Fatalf("a fresh engine starts at %d errors, want 0", e.Turn.ConsecutiveErrors)
	}

	hooks.OnToolResult("t1", "read_files", "ok", false)
	if e.Turn.ConsecutiveErrors != 0 {
		t.Errorf("a succeeding tool result moved the counter to %d, want 0", e.Turn.ConsecutiveErrors)
	}

	hooks.OnToolResult("t2", "write_files", "refused: policy", true)
	if e.Turn.ConsecutiveErrors != 1 {
		t.Errorf("one failing tool result moved the counter to %d, want 1", e.Turn.ConsecutiveErrors)
	}

	hooks.OnToolResult("t3", "write_files", "refused: policy", true)
	if e.Turn.ConsecutiveErrors != 2 {
		t.Errorf("two failing tool results moved the counter to %d, want 2", e.Turn.ConsecutiveErrors)
	}
}

// The rungs and what each one costs, read off contextBudget itself, so a change
// to the ladder has to be deliberate about how much a miscount is worth. These
// are the numbers the agent-side fix is defending.
func TestTheRecoveryLadderChargesMoreForEveryRung(t *testing.T) {
	cases := []struct {
		errors int
		budget int
	}{
		{0, 6000},
		{1, 20000},
		{2, 20000},
		{3, 35000},
		{4, 35000},
		{5, 50000},
		{6, 50000},
	}

	for _, tc := range cases {
		e := &Engine{Turn: TurnState{Counter: 5, ConsecutiveErrors: tc.errors}}
		_, budget := e.contextBudget("do the thing")
		if budget != tc.budget {
			t.Errorf("%d consecutive errors recalls %d tokens, want %d",
				tc.errors, budget, tc.budget)
		}
	}
}
