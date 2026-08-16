package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

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

// cancelledTurnContext is the turn context as recordModelHealth finds it after
// the stop button: Session.turn hands RunTurn the context whose cancel func
// InterruptSession calls, so by the time the error surfaces the context is done.
// Passing a live context here instead would test a state that cannot occur.
func cancelledTurnContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// expiredTurnContext is the turn context after one of inber's own policy clocks
// ran out — a sub-agent spawn timeout, a session max_duration, the bus cap. All
// three are context deadlines on the turn context, so all three leave it done.
func expiredTurnContext() context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	return ctx
}

// healthyModel puts a model in the state selectModel wants to find: a recent
// success and no error after it.
func healthyModel(t *testing.T, e *Engine, model string) {
	t.Helper()
	e.recordModelHealth(context.Background(), model, 1200, nil)
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
	e.recordModelHealth(cancelledTurnContext(), model, 40, context.Canceled)

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
// The deadlines a turn runs under on its own context are all inber's own — a
// spawn timeout, a session max_duration, the bus cap — and each leaves that
// context done, which is how the filter recognises them.
func TestPolicyDeadlineDoesNotMarkTheModelUnhealthy(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "claude-opus-4-6"
	healthyModel(t, e, model)

	e.recordModelHealth(expiredTurnContext(), model, 600_000, context.DeadlineExceeded)

	h := store.GetHealth(model)
	if !h.LastErrorAt.IsZero() {
		t.Fatalf("a policy deadline stamped LastErrorAt on %s (%q)", model, h.LastError)
	}
	// Assert the consequence too, not just the row. This used to be unassertable:
	// model_health stores unix seconds, so an error written in the same second as
	// the setup success left IsHealthy true whatever the filter did, and the test
	// would have passed without the fix. IsHealthy now reads the outcome counter,
	// so it is a real assertion again.
	if !h.IsHealthy(healthWindow) {
		t.Fatalf("a policy deadline failed %s over for every session on the box", model)
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
	e.recordModelHealth(context.Background(), model, 300_000, fmt.Errorf("%w (%d)", agent.ErrMaxAPICallsExceeded, 50))

	h := store.GetHealth(model)
	if !h.LastErrorAt.IsZero() {
		t.Fatalf("inber's own runaway cap stamped LastErrorAt on %s (%q) — that row is what "+
			"selectModel reads on the next turn, in this process and every other one", model, h.LastError)
	}
	if !h.IsHealthy(healthWindow) {
		t.Fatalf("inber's own runaway cap failed %s over for every session on the box", model)
	}
}

// TestAnUnconvertibleContentBlockDoesNotMarkTheModelUnhealthy covers the class
// the pinned SDK creates rather than the provider: the API grows a content block
// type, the SDK cannot convert it, and Agent.Run reports
// ErrUnconvertibleContentBlock. The provider answered, and the answer was valid.
//
// This one is worse than the cap if it is recorded, because it repeats: every
// turn that meets the new block writes another error, so failover walks the
// whole roster into the same wall and leaves every model on the box unhealthy.
func TestAnUnconvertibleContentBlockDoesNotMarkTheModelUnhealthy(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "claude-sonnet-4-5"
	healthyModel(t, e, model)

	// Exactly what Agent.executeAPICall builds, wrapper and detail included.
	e.recordModelHealth(context.Background(), model, 900,
		fmt.Errorf("%w: content block %d has type %q", agent.ErrUnconvertibleContentBlock, 1, "some_future_block"))

	h := store.GetHealth(model)
	if !h.LastErrorAt.IsZero() {
		t.Fatalf("an SDK-version failure stamped LastErrorAt on %s (%q) — that row is host-shared, "+
			"so one unknown block type fails every session on the box over", model, h.LastError)
	}
	if !h.IsHealthy(healthWindow) {
		t.Fatalf("an SDK-version failure marked %s unhealthy", model)
	}
}

// TestProviderFailureStillMarksTheModelUnhealthy is the control. Without it a
// recordModelHealth that recorded nothing ever would pass every test above.
//
// It asserts the model actually goes unhealthy, which is what failover reads.
// That assertion was impossible while ModelHealth.IsHealthy compared
// LastErrorAt against LastSuccessAt: both are stored as unix seconds, so an
// error recorded in the same second as this setup success compared equal rather
// than after, and IsHealthy stayed true no matter what was written. IsHealthy
// now reads the outcome counter, so the control is a control.
func TestProviderFailureStillMarksTheModelUnhealthy(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "claude-sonnet-4-5"
	healthyModel(t, e, model)

	e.recordModelHealth(context.Background(), model, 8000, errors.New(`api call failed: POST "https://api.anthropic.com/v1/messages": 529 Overloaded`))

	h := store.GetHealth(model)
	if h.LastErrorAt.IsZero() {
		t.Fatal("a 529 from the provider was not recorded — failover can no longer fire")
	}
	if h.LastError == "" {
		t.Fatal("the provider's complaint was not recorded, so an operator cannot see why failover fired")
	}
	if h.IsHealthy(healthWindow) {
		t.Fatal("a 529 was recorded but left the model healthy, so selectModel will keep " +
			"choosing it and failover never fires")
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

	e.recordModelHealth(context.Background(), model, 2000, fmt.Errorf("unexpected stop reason: %s", "pause_turn"))

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

	e.recordModelHealth(context.Background(), model, 1500, nil)

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
	if errorIsEvidenceAboutTheModel(context.Background(), wrapped) {
		t.Fatalf("a wrapped cancel (%v) was read as evidence about the model", wrapped)
	}
}

// TestAProviderThatStopsAnsweringIsRecorded is the regression test for the hole
// the cancel fix opened.
//
// The OpenAI-compatible client carries a flat 120s http.Client.Timeout
// (agent/openai.go) and serves openai, google, openrouter, ollama and the
// catch-all for every provider inber does not name. When a provider on that path
// hangs, net/http returns an error that satisfies errors.Is(context.DeadlineExceeded)
// — so a filter reading only the error shape wrote nothing, the model never went
// unhealthy, selectModel kept choosing it, and every turn burned 120s with
// failover never firing. That timeout is the only signal a hung provider on that
// path produces.
//
// The error is built by making a request that really times out, not by hand: the
// point of the test is the shape net/http hands back, and a hand-written error
// would pin my guess at it instead.
func TestAProviderThatStopsAnsweringIsRecorded(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "llama3.3:70b"
	healthyModel(t, e, model)

	err := timeoutFromAProviderThatNeverAnswers(t)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("premise gone: a client timeout no longer reads as a deadline (%v)", err)
	}

	// The turn context is live and stays live: nothing of inber's expired here.
	e.recordModelHealth(context.Background(), model, 120_000, err)

	h := store.GetHealth(model)
	if h.LastErrorAt.IsZero() {
		t.Fatalf("a provider that stopped answering was not recorded against %s — nothing else "+
			"reports it, so selectModel keeps choosing it and every turn burns the full timeout", model)
	}
	if h.IsHealthy(healthWindow) {
		t.Fatalf("a provider that stopped answering left %s healthy, so failover never fires", model)
	}
}

// TestAPolicyDeadlineDuringAProviderTimeoutIsStillInbersOwn covers the overlap.
// A session's max_duration can expire while an OpenAI-compatible request is
// already timing out, and then both clocks have fired. inber's own wins: it is
// the one that decided to stop, and marking a host-shared model unhealthy is the
// damaging direction, so the tie goes to writing nothing.
func TestAPolicyDeadlineDuringAProviderTimeoutIsStillInbersOwn(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "gpt-5.2"
	healthyModel(t, e, model)

	e.recordModelHealth(expiredTurnContext(), model, 120_000, timeoutFromAProviderThatNeverAnswers(t))

	if h := store.GetHealth(model); !h.LastErrorAt.IsZero() {
		t.Fatalf("inber's own max_duration was recorded against %s as a provider fault (%q)", model, h.LastError)
	}
}

// timeoutFromAProviderThatNeverAnswers returns the error the OpenAI-compatible
// client produces when the provider does not answer in time, wrapped exactly as
// ChatCompletion and runOpenAITurn wrap it on the way to recordModelHealth.
func timeoutFromAProviderThatNeverAnswers(t *testing.T) error {
	t.Helper()
	provider := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	t.Cleanup(provider.Close)

	client := &http.Client{Timeout: 50 * time.Millisecond}
	req, err := http.NewRequestWithContext(context.Background(), "POST", provider.URL+"/chat/completions", nil)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("the provider answered inside the timeout; this helper produces no error")
	}
	return fmt.Errorf("OpenAI API call failed: %w", fmt.Errorf("send request: %w", err))
}
