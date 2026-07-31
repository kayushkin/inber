package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

// pastInstant is a deadline that has already gone by.
func pastInstant() time.Time { return time.Now().Add(-time.Second) }

// TestRunTurnStopsWhenItsContextIsCancelled is the curative test for the
// finding that a turn in flight could not be stopped.
//
// RunTurn used to take no context and hand executeAgent a fresh
// context.Background(), so the one cancellation check in the whole execute
// path — Agent.Run's ctx.Err() — could never fire. A session interrupt, a stop
// and a sub-agent spawn timeout all cancelled a context that nothing observed,
// and the API call and any running tool ran to completion.
//
// The engine here is deliberately empty: it carries no client, so reaching the
// API at all is a panic. That is the point. Revert RunTurn to
// context.Background() and this test does not fail politely — it panics inside
// AnthropicProvider.CompleteStreaming, which is exactly the network call a
// cancelled turn is not supposed to make.
func TestRunTurnStopsWhenItsContextIsCancelled(t *testing.T) {
	e := &Engine{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := e.RunTurn(ctx, "do something expensive")

	if err == nil {
		t.Fatal("RunTurn returned no error for an already-cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunTurn error = %v, want context.Canceled — the turn ignored its context", err)
	}
}

// TestRunTurnReportsWhatItStoppedFor checks the turn distinguishes the two ways
// a caller stops it. A sub-agent spawn reports "timeout" separately from
// "error", and it can only tell them apart if the deadline reaches the turn as
// a deadline rather than as a bare cancellation.
func TestRunTurnReportsWhatItStoppedFor(t *testing.T) {
	e := &Engine{}

	ctx, cancel := context.WithDeadline(context.Background(), pastInstant())
	defer cancel()

	_, err := e.RunTurn(ctx, "do something expensive")

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunTurn error = %v, want context.DeadlineExceeded", err)
	}
}
