package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kayushkin/inber/engine"
	sessionMod "github.com/kayushkin/inber/session"
)

// TestTurnContextDropsCallerCancellation pins the half of the policy that is
// easy to lose by threading the caller's context straight through. The caller
// is an HTTP request, so its context dies when the browser tab closes or a
// proxy hits its read timeout. Neither of those is a reason to abandon a turn
// the session would otherwise finish and keep.
func TestTurnContextDropsCallerCancellation(t *testing.T) {
	caller, cancelCaller := context.WithCancel(context.Background())
	turnCtx, cancelTurn := withoutCallerCancellation(caller)
	defer cancelTurn()

	cancelCaller()

	select {
	case <-turnCtx.Done():
		t.Fatal("the caller hanging up cancelled the turn")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestTurnContextKeepsCallerDeadline pins the other half. A sub-agent spawn
// bounds its child with context.WithTimeout and then reports "timeout"
// separately from "error"; that only means anything if the bound survives into
// the turn and actually stops it.
func TestTurnContextKeepsCallerDeadline(t *testing.T) {
	want := time.Now().Add(time.Hour)
	caller, cancelCaller := context.WithDeadline(context.Background(), want)
	defer cancelCaller()

	turnCtx, cancelTurn := withoutCallerCancellation(caller)
	defer cancelTurn()

	got, ok := turnCtx.Deadline()
	if !ok {
		t.Fatal("the turn context carries no deadline — a spawn timeout would never fire")
	}
	if !got.Equal(want) {
		t.Fatalf("turn deadline = %v, want the caller's %v", got, want)
	}
}

// TestInterruptCancelsTheTurnContext checks the verb the session exposes for
// stopping a turn reaches the context the turn runs under.
func TestInterruptCancelsTheTurnContext(t *testing.T) {
	turnCtx, cancel := withoutCallerCancellation(context.Background())
	defer cancel()

	s := &Session{Key: "test"}
	s.mu.Lock()
	s.cancel = cancel
	s.Status = Running
	s.mu.Unlock()

	s.interrupt()

	select {
	case <-turnCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("interrupt did not cancel the turn context")
	}

	s.mu.Lock()
	status := s.Status
	s.mu.Unlock()
	if status != Idle {
		t.Errorf("status after interrupt = %v, want Idle", status)
	}
}

// TestStopCancelsTheTurnContext is the same check for the terminal verb.
func TestStopCancelsTheTurnContext(t *testing.T) {
	turnCtx, cancel := withoutCallerCancellation(context.Background())
	defer cancel()

	s := &Session{Key: "test"}
	s.mu.Lock()
	s.cancel = cancel
	s.Status = Running
	s.mu.Unlock()

	s.stop()

	select {
	case <-turnCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel the turn context")
	}
}

// TestInterruptStopsATurnAlreadyInFlight is the curative test for the finding
// this whole file exists for: interrupt used to cancel a context nothing
// observed, so a runaway turn could not be stopped.
//
// It interrupts from inside the turn. The engine emits "Running agent..."
// through the display hooks at the top of executeAgent, immediately before it
// hands the conversation to Agent.Run — so an interrupt raised there lands on a
// turn that is past every early return and about to call the API.
//
// The engine carries no client, so calling the API is a panic. Revert
// Engine.RunTurn to context.Background() and that is how this test fails: not
// on an assertion, but on the network call an interrupted turn made anyway.
func TestInterruptStopsATurnAlreadyInFlight(t *testing.T) {
	engineSession, err := sessionMod.New(t.TempDir(), "claude-sonnet-4-5-20250929", "test", "", nil)
	if err != nil {
		t.Fatalf("building the engine's session: %v", err)
	}

	s := &Session{Key: "test", Engine: &engine.Engine{Session: engineSession}}

	interrupted := false
	s.onEvent = func(event StreamEvent) {
		if event.Kind == "status" && event.Text == "Running agent..." {
			interrupted = true
			s.interrupt()
		}
	}

	_, err = s.turn(context.Background(), "do something expensive")

	if !interrupted {
		t.Fatal(`the turn never reported "Running agent..." — the test never interrupted anything`)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("turn error = %v, want context.Canceled", err)
	}
}

// TestTurnHonorsAnExpiredCallerDeadline drives the whole path a sub-agent
// spawn timeout takes — Session.turn, turnContext, Engine.RunTurn,
// executeAgent, Agent.Run — against a deadline that has already gone by, and
// checks the turn stops instead of calling the API.
//
// The engine carries no client, so reaching the API is a panic rather than a
// failed assertion. That is deliberate: before the fix this path did reach it.
func TestTurnHonorsAnExpiredCallerDeadline(t *testing.T) {
	s := &Session{Key: "test", Engine: &engine.Engine{}}

	caller, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, err := s.turn(caller, "do something expensive")

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("turn error = %v, want context.DeadlineExceeded", err)
	}

	s.mu.Lock()
	cancelAfter := s.cancel
	s.mu.Unlock()
	if cancelAfter != nil {
		t.Error("turn left its cancel function behind; a later interrupt would cancel nothing")
	}
}
