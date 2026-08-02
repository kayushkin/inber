package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/kayushkin/inber/agent"
	modelstore "github.com/kayushkin/model-store"
)

// engineWithTempModelStore builds an Engine backed by a real model-store on a
// throwaway path. The store is the thing under test — health is persistent,
// cross-process state and the defect these tests pin is a write to it — so a
// fake that only counts calls would pass while the rows on disk stayed wrong.
func engineWithTempModelStore(t *testing.T) (*Engine, *modelstore.Store) {
	t.Helper()
	store, err := modelstore.Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("opening a temp model store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return &Engine{modelStore: store}, store
}

// healthyModel puts a model in the state selectModel wants to find: a recent
// success and no error after it.
func healthyModel(t *testing.T, e *Engine, model string) {
	t.Helper()
	e.recordModelHealth(model, 1200, nil)
	if !e.modelStore.GetHealth(model).IsHealthy(healthWindow) {
		t.Fatalf("setup failed: %s is not healthy after a recorded success", model)
	}
}

// TestPressingStopDoesNotMarkTheModelUnhealthy is the regression test for the
// defect. A user cancel reaches recordModelHealth as context.Canceled, and used
// to be written to the host-shared store as a provider fault — so one stop press
// in one session failed every other session on the box over to a fallback model.
func TestPressingStopDoesNotMarkTheModelUnhealthy(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "claude-sonnet-4-5"
	healthyModel(t, e, model)
	before := store.GetHealth(model)

	// What Agent.Run returns when Session.cancel fires: the bare ctx.Err().
	e.recordModelHealth(model, 40, context.Canceled)

	after := store.GetHealth(model)
	if !after.IsHealthy(healthWindow) {
		t.Fatalf("a stop press marked %s unhealthy (last error %q) — selectModel now fails every "+
			"session on this host over to a fallback", model, after.LastError)
	}
	if !after.LastErrorAt.IsZero() {
		t.Fatalf("a cancel stamped LastErrorAt (%s, %q); it is not evidence about the model",
			after.LastErrorAt, after.LastError)
	}
	if after.LastSuccessAt != before.LastSuccessAt || after.AvgResponseMs != before.AvgResponseMs {
		t.Fatalf("a cancel was recorded as a success: LastSuccessAt %s→%s, AvgResponseMs %d→%d. "+
			"The 40ms is how long until the user hit stop, not a response time, and AvgResponseMs "+
			"feeds timeoutFromHealth", before.LastSuccessAt, after.LastSuccessAt,
			before.AvgResponseMs, after.AvgResponseMs)
	}
}

// TestPolicyDeadlineDoesNotMarkTheModelUnhealthy covers the other context error.
// Every deadline a turn runs under is inber's own — a spawn timeout, a session
// max_duration, the bus cap — because selectModel's timeoutHint is discarded by
// executeAgent and never applied.
func TestPolicyDeadlineDoesNotMarkTheModelUnhealthy(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "claude-opus-4-6"
	healthyModel(t, e, model)

	e.recordModelHealth(model, 600_000, context.DeadlineExceeded)

	// Assert on the stamp, not on IsHealthy: model_health timestamps are unix
	// seconds, so an error written in the same second as the setup success would
	// leave IsHealthy true anyway and the test would pass without the fix.
	if h := store.GetHealth(model); !h.LastErrorAt.IsZero() {
		t.Fatalf("a policy deadline stamped LastErrorAt on %s (%q)", model, h.LastError)
	}
}

// TestOwnAPICallCapDoesNotMarkTheModelUnhealthy pins the class that is live in
// the real store on this host: claude-sonnet-4-5 has been unhealthy since
// 2026-04-05 with last_error "exceeded max API calls (50)". Reaching that cap
// means the provider answered fifty times in a row.
func TestOwnAPICallCapDoesNotMarkTheModelUnhealthy(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "claude-sonnet-4-5"
	healthyModel(t, e, model)

	// Exactly what Agent.Run builds, wrapper and count included.
	e.recordModelHealth(model, 300_000, fmt.Errorf("%w (%d)", agent.ErrMaxAPICallsExceeded, 50))

	if h := store.GetHealth(model); !h.LastErrorAt.IsZero() {
		t.Fatalf("inber's own runaway cap stamped LastErrorAt on %s (%q) — that row is what "+
			"selectModel reads on the next turn, in this process and every other one", model, h.LastError)
	}
}

// TestProviderFailureStillMarksTheModelUnhealthy is the control. Without it a
// recordModelHealth that recorded nothing ever would pass every test above.
//
// It asserts on the row rather than on IsHealthy for a reason worth knowing:
// model_health stores unix seconds, so an error arriving in the same second as a
// success leaves LastErrorAt equal to — not after — LastSuccessAt, and
// ModelHealth.IsHealthy keeps returning true. A real 529 does flip health, but
// only because a turn takes longer than a second; the assertion here must not
// rest on that.
func TestProviderFailureStillMarksTheModelUnhealthy(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "claude-sonnet-4-5"
	healthyModel(t, e, model)

	e.recordModelHealth(model, 8000, errors.New(`api call failed: POST "https://api.anthropic.com/v1/messages": 529 Overloaded`))

	h := store.GetHealth(model)
	if h.LastErrorAt.IsZero() {
		t.Fatal("a 529 from the provider was not recorded — failover can no longer fire")
	}
	if h.LastError == "" {
		t.Fatal("the provider's complaint was not recorded, so an operator cannot see why failover fired")
	}
}

// TestUnexpectedStopReasonStillRecordsAnError pins that this change did NOT
// settle the open question on todo 4c511c8f. Whether a refusal or an unhandled
// pause_turn is a provider fault, a model fault or a gap in inber is the owner's
// call; until it is made, the path behaves exactly as it did before.
func TestUnexpectedStopReasonStillRecordsAnError(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "claude-opus-4-6"
	healthyModel(t, e, model)

	e.recordModelHealth(model, 2000, fmt.Errorf("unexpected stop reason: %s", "pause_turn"))

	if store.GetHealth(model).LastErrorAt.IsZero() {
		t.Fatal("an unexpected stop reason stopped being recorded — that is an open decision, " +
			"not something this filter should have taken")
	}
}

// TestSuccessStillRecordsAResponseTime guards the untouched branch: the filter
// must not have cost the store its only source of AvgResponseMs.
func TestSuccessStillRecordsAResponseTime(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "claude-sonnet-4-5"

	e.recordModelHealth(model, 1500, nil)

	h := store.GetHealth(model)
	if !h.IsHealthy(healthWindow) {
		t.Fatal("a successful turn did not make the model healthy")
	}
	if h.AvgResponseMs == 0 {
		t.Fatal("a successful turn recorded no response time, so timeoutFromHealth has nothing to work from")
	}
}

// TestMaxAPICallsMessageIsUnchanged pins the operator-visible text across the
// switch to a wrapped sentinel. The string appears in logs and in the live
// model_health rows on this host; changing it silently would break a grep and
// make the old rows unrecognisable.
func TestMaxAPICallsMessageIsUnchanged(t *testing.T) {
	got := fmt.Errorf("%w (%d)", agent.ErrMaxAPICallsExceeded, 50).Error()
	if want := "exceeded max API calls (50)"; got != want {
		t.Fatalf("error text changed: got %q, want %q", got, want)
	}
}

// TestCancelWrappedByACallerIsStillACancel pins that the classifier reads the
// error chain rather than the top-level value. Agent.Run returns ctx.Err() bare
// today, but any caller that adds context to it must not turn a cancel back into
// a provider fault.
func TestCancelWrappedByACallerIsStillACancel(t *testing.T) {
	wrapped := fmt.Errorf("running agent on claude-sonnet-4-5: %w", context.Canceled)
	if errorIsEvidenceAboutTheModel(wrapped) {
		t.Fatalf("a wrapped cancel (%v) was read as evidence about the model", wrapped)
	}
}
