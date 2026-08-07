package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// The OpenAI-compatible turn loop had no runaway cap and no cancellation check.
// The Anthropic loop has had both since it was written, and engine.failover
// already reasons about the cap's sentinel by name — so this was one of two
// turn loops enforcing an invariant inber has an opinion about, not a missing
// opinion. These tests drive the real runOpenAITurn against a provider that
// never stops asking for tools, which is the only condition under which either
// guard is the thing that ends the turn.

// alwaysToolCallingOpenAI answers every request with the same tool call, so the
// loop has no way to end except a guard of its own. It counts requests, which is
// what makes "the cap held" a number rather than an absence of a hang.
type alwaysToolCallingOpenAI struct {
	mu       sync.Mutex
	requests int
	server   *httptest.Server
}

func newAlwaysToolCallingOpenAI(t *testing.T) *alwaysToolCallingOpenAI {
	t.Helper()
	f := &alwaysToolCallingOpenAI{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agent.OpenAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.requests++
		n := f.requests
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toolCallResponse(fmt.Sprintf("call_%d", n), "spin", `{"path":"a.txt"}`))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *alwaysToolCallingOpenAI) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

// TestAnOpenAIServedRunawayStopsAtTheSharedCap is the defect. A model that keeps
// emitting tool_use had nothing to stop it on this path: the loop exits only on
// end_turn, max_tokens, an unexpected stop reason or a request error, and a
// runaway produces none of those.
func TestAnOpenAIServedRunawayStopsAtTheSharedCap(t *testing.T) {
	var spun []string
	f := newAlwaysToolCallingOpenAI(t)
	e := openAIEngine(t, f.engineClientHost(), recordingTool("spin", "ok", &spun))

	result, err := e.runOpenAITurn(context.Background(), nil)

	if err == nil {
		t.Fatalf("the turn ended on its own after %d API calls; nothing stopped the runaway", f.requestCount())
	}
	if !errors.Is(err, agent.ErrMaxAPICallsExceeded) {
		t.Fatalf("the cap returned %q, which errors.Is cannot match against agent.ErrMaxAPICallsExceeded — "+
			"recordModelHealth will read it as a provider fault and mark a model that answered every call "+
			"unhealthy for every harness on the host", err)
	}
	if got := f.requestCount(); got != agent.MaxAPICallsPerTurn {
		t.Errorf("the provider was called %d times, want %d — the cap is off by the number of calls it lets through",
			got, agent.MaxAPICallsPerTurn)
	}
	if !strings.Contains(result.Text, "exceeded") {
		t.Errorf("the turn stopped with %q, so the user is shown nothing that says why", result.Text)
	}
}

// TestTheTwoTurnLoopsShareOneCapValue. The point of the fix is that the cap is a
// property of a turn, not of a provider branch. Two loops reading two numbers
// would be the same defect with a smaller gap, and it would come back the first
// time somebody tuned one of them.
func TestTheTwoTurnLoopsShareOneCapValue(t *testing.T) {
	var spun []string
	f := newAlwaysToolCallingOpenAI(t)
	e := openAIEngine(t, f.engineClientHost(), recordingTool("spin", "ok", &spun))

	_, err := e.runOpenAITurn(context.Background(), nil)
	if err == nil {
		t.Fatal("the turn did not hit the cap")
	}

	// The Anthropic loop's own test pins this text against its cap. Reading the
	// same sentence off this path is how a second, drifting constant shows up.
	want := fmt.Sprintf("exceeded max API calls (%d)", agent.MaxAPICallsPerTurn)
	if got := err.Error(); got != want {
		t.Errorf("this loop reports %q and the shared cap is %q — the two loops are not reading one value", got, want)
	}
}

// TestAnOpenAIServedTurnStopsWhenItsContextIsCancelled. Cancellation did reach
// the in-flight request, so this was never a wedged-forever bug — but the loop
// checked nothing at its head, so a cancel arriving between calls was noticed
// only by the next paid request failing. The cancel must end the turn at the
// top of the iteration, before another call is built.
//
// The cancel fires from inside the TOOL, not from inside the request, and the
// difference is the whole test. Cancelling mid-request kills the round trip, so
// the loop returns "OpenAI API call failed: context canceled" from the error
// path and the head check is never reached — an earlier draft of this test
// passed that way and proved nothing. A stop button pressed while a tool is
// running leaves the loop to arrive at its own head with a live conversation
// and a dead context, which is the state the guard exists for.
func TestAnOpenAIServedTurnStopsWhenItsContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f := newAlwaysToolCallingOpenAI(t)
	e := openAIEngine(t, f.engineClientHost(), cancellingTool("spin", cancel))

	result, err := e.runOpenAITurn(ctx, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled turn returned %v, which recordModelHealth cannot tell from a provider failure", err)
	}
	if strings.Contains(err.Error(), "OpenAI API call failed") {
		t.Fatalf("the turn ended because a later request failed, not because the loop noticed the cancel: %v", err)
	}
	if got := f.requestCount(); got != 1 {
		t.Errorf("the provider was called %d times, want 1 — the loop built another paid call after the stop", got)
	}
	if !strings.Contains(result.Text, "stopped") {
		t.Errorf("a cancelled turn reported %q, so the stop is invisible to the user", result.Text)
	}
}

// TestTheCapDoesNotOverwriteAnAnswerTheTurnAlreadyHas. The placeholder exists
// for a turn that produced nothing; writing it over real text would delete the
// only part of a runaway worth keeping.
func TestTheCapDoesNotOverwriteAnAnswerTheTurnAlreadyHas(t *testing.T) {
	result := &agent.TurnResult{Text: "here is the part I did finish"}

	if err := agent.StopForAPICallCap(result); !errors.Is(err, agent.ErrMaxAPICallsExceeded) {
		t.Fatalf("StopForAPICallCap returned %v", err)
	}
	if result.Text != "here is the part I did finish" {
		t.Errorf("the placeholder overwrote the turn's own text: %q", result.Text)
	}
}

// cancellingTool stops the turn from inside a tool call, the way a stop button
// pressed while a tool is running does.
func cancellingTool(name string, cancel context.CancelFunc) agent.Tool {
	return agent.Tool{
		Name:        name,
		Description: name,
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{"path": map[string]any{"type": "string"}},
		},
		Run: func(ctx context.Context, input string) (string, error) {
			cancel()
			return "ok", nil
		},
	}
}

// engineClientHost adapts the fake to openAIEngine, which takes the chain
// tests' fake. Only the URL is used, so this keeps one engine constructor
// instead of a second copy that could drift from how a real turn is built.
func (f *alwaysToolCallingOpenAI) engineClientHost() *fakeOpenAI {
	return &fakeOpenAI{server: f.server}
}
