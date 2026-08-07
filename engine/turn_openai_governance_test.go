package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kayushkin/inber/agent"
)

// A turn served by an OpenAI-compatible provider is governed by less than a
// turn served by Anthropic, and the reason is structural rather than a series
// of oversights.
//
// `buildAgent` (engine/build.go) is where inber attaches everything that makes
// a turn a governed turn. It has exactly two callers, `engine/turn_execute.go`
// :41 and :48, and both sit on the Anthropic branch — `turn_execute.go:29-31`
// routes an OpenAI-compatible client to `runOpenAITurn` and returns before
// reaching either. So every invariant wired inside `buildAgent` is an invariant
// the second turn loop does not have, and adding a hook there gives it to one
// path only.
//
// Nine separate todos have each recorded one instance of that and each called
// itself "the Nth instance of one root cause". Counting them is not the point;
// none of them could see the whole list, so each priced its own fix against a
// structural change it could not scope. This file is the list, measured, in one
// place, and each row is a running test rather than a claim in prose.
//
//	invariant                       wired at            OpenAI path  filed as
//	------------------------------------------------------------------------------
//	tools                           build.go:81-83      yes          —
//	hooks                           build.go:89         yes          —
//	sideband callbacks              build.go:92-94      yes          —
//	tool refusal (the guard)        build.go:105        yes          —
//	runaway API-call cap            agent/agent.go:336  branch only  9fb35070 (done)
//	volatile context                build.go:40         NO           ec9c7122
//	mid-turn injection channel      build.go:97-99      NO           fc6323ca
//	configured limits               build.go:108-110    NO           fc6323ca
//	context window / overflow       build.go:113-114    NO           df1de352
//	context pruning                 build.go:48         NO           df1de352
//	thinking budget / effort        build.go:85-87      NO           e68b05e0
//	per-call usage record           agent_run.go:273    NO           see below
//	cached-token accounting         (wire)              NO           0d052752
//	reasoning-model parameter       turn_openai.go:70   WRONG        25b91c78
//
// `result.APICalls` is the one row with no todo of its own, and it is not worth
// one: its only reader is `reportCallsThatBoughtNoCache`
// (`turn_postprocess.go:107`), a log line that prices the calls which sent no
// tools block. That diagnostic can never fire on this path, but the numbers it
// would print are the same ones `0d052752` is about, so it closes with that one.
//
// Three rows of `buildAgent` are deliberately absent from that table, because
// they are not gaps: `SetRepoRoot` feeds `agent.Agent`'s read cache, `FrozenIdx`
// and the prompt blueprint both place Anthropic `cache_control` breakpoints, and
// none of the three has anything to do on a path that builds no `agent.Agent`
// and sends no breakpoints. Do not "port" them.
//
// Every test below pins a defect AS PRESENT. That is deliberate and it is the
// point: whichever way the gaps are closed, the fix has to come here and change
// the assertion, so a fix cannot land while the table still says the hole is
// open, and a tenth hook cannot be added to one loop in silence.

// rawOpenAI serves canned response BODIES rather than typed responses, which is
// how a test reaches a field the Go type does not have. `agent.OpenAIUsage`
// carries three counters and no cache figures, so a typed fake cannot put
// `prompt_tokens_details.cached_tokens` on the wire at all — the very drop one
// of these tests is about.
type rawOpenAI struct {
	bodies   []string
	status   int
	requests []agent.OpenAIRequest
	server   *httptest.Server
}

func newRawOpenAI(t *testing.T, status int, bodies ...string) *rawOpenAI {
	t.Helper()
	f := &rawOpenAI{bodies: bodies, status: status}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agent.OpenAIRequest
		json.NewDecoder(r.Body).Decode(&req)
		f.requests = append(f.requests, req)
		body := `{}`
		if len(f.bodies) > 0 {
			body = f.bodies[0]
			f.bodies = f.bodies[1:]
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		w.Write([]byte(body))
	}))
	t.Cleanup(f.server.Close)
	return f
}

// openAIEngineWithModel is openAIEngine with the model id under test. The
// reasoning-parameter branch reads `client.Model`, so the id is the input.
func openAIEngineWithModel(t *testing.T, f *fakeOpenAI, model string, tools ...agent.Tool) *Engine {
	t.Helper()
	return &Engine{
		agentTools:  tools,
		modelClient: &agent.ModelClient{OpenAIClient: agent.NewOpenAIClient(f.server.URL, "test-key", model)},
	}
}

// requestText renders a whole request as JSON, which is how these tests ask
// "did this string reach the provider at all" without caring which message it
// would have ridden in.
func requestText(t *testing.T, req agent.OpenAIRequest) string {
	t.Helper()
	encoded, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(encoded)
}

// TestTheOpenAILoopNeverConsultsTheSessionsConfiguredLimits. `MaxTurns`,
// `MaxInputTokens` and `MaxResponseTime` are parsed off the request, copied
// onto `e.Limits` by `applyRequestOverrides`, and read by `buildLimitCheck` —
// which is installed only at `build.go:108-110`, inside `buildAgent`. A caller
// who sets a budget on an OpenAI-served session is answered 200 and gets no
// budget.
//
// The second half of this test is what makes it a wiring finding rather than a
// configuration one: the same engine's own limit check, asked directly, says
// stop. The check is built, correct and simply never called.
func TestTheOpenAILoopNeverConsultsTheSessionsConfiguredLimits(t *testing.T) {
	var seen []string
	f := newFakeOpenAI(t,
		toolCallResponse("call_1", "read_it", `{"path":"a.txt"}`),
		toolCallResponse("call_2", "read_it", `{"path":"b.txt"}`),
		toolCallResponse("call_3", "read_it", `{"path":"c.txt"}`),
		textResponse("finished"),
	)
	e := openAIEngine(t, f, recordingTool("read_it", "contents", &seen))
	e.Limits.MaxTurns = 1

	result, err := e.runOpenAITurn(context.Background(), nil)
	if err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}

	if result.ToolCalls != 3 {
		t.Fatalf("the turn made %d tool calls, want 3 — the canned responses did not all run", result.ToolCalls)
	}
	if len(f.requests) != 4 {
		t.Fatalf("the loop made %d requests, want 4", len(f.requests))
	}

	// The limit check exists and would have stopped this turn after its first
	// tool call. Nothing consulted it.
	exceeded, reason := e.buildLimitCheck()(result)
	if !exceeded {
		t.Fatalf("buildLimitCheck did not trip on a result of %d tool calls against MaxTurns=%d — "+
			"this test no longer measures what it claims", result.ToolCalls, e.Limits.MaxTurns)
	}
	t.Logf("the limit that was never consulted would have said: %s", reason)
}

// TestTheOpenAILoopDropsTheTurnsVolatileContext. `turn_prompt.go:126-152`
// splits each turn's context in two and puts everything volatile — the agent
// fleet, the live session status, the task plan and scratchpad, the workspace
// roots and the cross-zone stale-read warning — on `e.Turn.VolatileContext`.
// That field has exactly one consumer, `build.go:40`. It is assembled every
// turn on this path and thrown away unread, and `turn_prepare.go:94` clears it
// at the top of the next turn.
func TestTheOpenAILoopDropsTheTurnsVolatileContext(t *testing.T) {
	const marker = "[Context]\nfleet: claxon is running · stale read: engine/build.go was re-read"

	f := newFakeOpenAI(t, textResponse("done"))
	e := openAIEngine(t, f)
	e.Turn.VolatileContext = marker

	if _, err := e.runOpenAITurn(context.Background(), nil); err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}

	if strings.Contains(requestText(t, f.request(0)), "stale read") {
		t.Errorf("the volatile context reached the provider — this path grew a consumer and the table above is stale")
	}
	if e.Turn.VolatileContext != marker {
		t.Errorf("the volatile context was taken (%q) but did not reach the request, which loses it silently", e.Turn.VolatileContext)
	}
}

// TestTheOpenAILoopNeverDrainsTheInjectionChannel. `e.injections` is the
// channel a mid-turn steer arrives on, drained by `buildInjectCheck` and wired
// at `build.go:97-99`. `Agent.Run` drains it from its second API call onward, so
// a message sent while a turn is running reaches the model inside that turn.
//
// This half is bounded rather than lost — `Session.turn` requeues anything
// unread onto `pendingMessages` — so it surfaces at the NEXT turn instead of
// never. A delivery-latency defect, not data loss, and the test says which by
// asserting the message is still in the channel afterwards.
func TestTheOpenAILoopNeverDrainsTheInjectionChannel(t *testing.T) {
	const steer = "stop reading and answer with what you have"

	var seen []string
	f := newFakeOpenAI(t,
		toolCallResponse("call_1", "read_it", `{"path":"a.txt"}`),
		textResponse("finished"),
	)
	e := openAIEngine(t, f, recordingTool("read_it", "contents", &seen))

	injections := make(chan string, 1)
	injections <- steer
	e.injections = injections

	if _, err := e.runOpenAITurn(context.Background(), nil); err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}

	if len(f.requests) < 2 {
		t.Fatalf("the loop made %d requests; an injection point exists only from the second", len(f.requests))
	}
	for i := range f.requests {
		if strings.Contains(requestText(t, f.request(i)), steer) {
			t.Errorf("request %d carried the injected steer — this path grew a drain and the table above is stale", i)
		}
	}
	if len(injections) != 1 {
		t.Errorf("the injection channel was drained (%d queued) but the message reached no request, which loses it outright", len(injections))
	}
}

// TestTheOpenAILoopSendsMaxTokensToEveryReasoningModelItDoesNotNameByPrefix.
// `turn_openai.go:69-73` picks `max_completion_tokens` on
// `HasPrefix(model, "o1") || HasPrefix(model, "o3")`. OpenAI rejects `max_tokens`
// for reasoning-era models with a hard 400 on the first request, and
// `errorIsEvidenceAboutTheModel` does not exclude a provider 400 — so the model
// is recorded unhealthy in a host-shared model-store for a parameter name inber
// chose itself.
//
// `openrouter` is routed through this same client and every OpenRouter id is
// namespaced, so the prefix match cannot fire for any reasoning model reached
// that way.
func TestTheOpenAILoopSendsMaxTokensToEveryReasoningModelItDoesNotNameByPrefix(t *testing.T) {
	missed := []string{"gpt-5", "gpt-5.6-terra", "o4-mini", "openai/o3-mini", "azure/o3"}
	for _, model := range missed {
		t.Run(model, func(t *testing.T) {
			f := newFakeOpenAI(t, textResponse("done"))
			e := openAIEngineWithModel(t, f, model)

			if _, err := e.runOpenAITurn(context.Background(), nil); err != nil {
				t.Fatalf("runOpenAITurn: %v", err)
			}

			req := f.request(0)
			if req.MaxCompletionTokens != 0 {
				t.Errorf("%s was sent max_completion_tokens — the prefix match grew a case and the table above is stale", model)
			}
			if req.MaxTokens == 0 {
				t.Errorf("%s was sent neither bound", model)
			}
		})
	}

	// The control. Without it a matcher that sent max_tokens to everything
	// would turn the whole test green for the wrong reason.
	for _, model := range []string{"o1-preview", "o3-mini"} {
		t.Run(model+"/control", func(t *testing.T) {
			f := newFakeOpenAI(t, textResponse("done"))
			e := openAIEngineWithModel(t, f, model)

			if _, err := e.runOpenAITurn(context.Background(), nil); err != nil {
				t.Fatalf("runOpenAITurn: %v", err)
			}

			if f.request(0).MaxCompletionTokens == 0 {
				t.Errorf("%s was not sent max_completion_tokens, so the branch this test contrasts with is gone", model)
			}
		})
	}
}

// TestTheOpenAILoopPricesEveryCachedTokenAsFreshInput. `agent.OpenAIUsage` has
// a home for three counters and none for `prompt_tokens_details.cached_tokens`,
// the field every OpenAI-compatible endpoint that caches reports, so it is
// dropped at the JSON boundary. `CalcCostWithCache` is then called with
// `cacheRead=0` by construction and every cached token is billed at the full
// input price.
//
// The second consequence is the one that blocks other work: there is no number
// on this path that could say whether a cache change did anything, so the
// volatile-context placement decision in `ec9c7122` cannot be measured here
// until this is fixed.
//
// ⚠️ Note what is NOT asserted: which of `prompt_tokens` and `cached_tokens` a
// fix should subtract from which. OpenAI's `prompt_tokens` INCLUDES the cached
// span and Anthropic's `input_tokens` EXCLUDES it, and getting that backwards
// double-charges — the same class of error as `00093e48`, sign flipped. That is
// the open decision on `0d052752` and this test deliberately does not prejudge
// it.
func TestTheOpenAILoopPricesEveryCachedTokenAsFreshInput(t *testing.T) {
	body := `{
		"id": "chatcmpl-1",
		"choices": [{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
		"usage": {
			"prompt_tokens": 1000,
			"completion_tokens": 20,
			"total_tokens": 1020,
			"prompt_tokens_details": {"cached_tokens": 800}
		}
	}`
	f := newRawOpenAI(t, http.StatusOK, body)
	e := &Engine{modelClient: &agent.ModelClient{
		OpenAIClient: agent.NewOpenAIClient(f.server.URL, "test-key", "test-model"),
	}}

	result, err := e.runOpenAITurn(context.Background(), nil)
	if err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}

	if result.InputTokens != 1000 {
		t.Fatalf("input tokens %d, want the 1000 the provider reported — the fake is not being read", result.InputTokens)
	}
	if result.CacheReadTokens != 0 || result.CacheCreationTokens != 0 {
		t.Errorf("cache tokens reached the result (read=%d write=%d) — the wire gained a cache field and the table above is stale",
			result.CacheReadTokens, result.CacheCreationTokens)
	}
	// 800 of those 1000 were cached and are billed as if they were not.
	if len(result.APICalls) != 0 {
		t.Errorf("per-call usage reached the result (%d entries) — this path grew the APICalls record and the table above is stale", len(result.APICalls))
	}
	// With APICalls empty, reportCallsThatBoughtNoCache has nothing to walk, so
	// the one diagnostic that could have shown this turn's cache behaviour is
	// silent for the same reason the counters are zero.
}

// TestTheOpenAILoopAnswersAContextOverflowWithNoGuardAtAll. `buildAgent` is the
// sole caller of both `SetContextWindow` (`build.go:113-114`) and
// `configureContextPruning` (`build.go:48`), so on this path
// `Agent.BeforeRequest` is nil, `a.contextWindow` is 0 and
// `isContextLengthError` is never consulted. A `context_length_exceeded` is an
// unrecoverable turn failure with zero compaction attempted, where the Anthropic
// path would at least try — and the raw error then goes to `recordModelHealth`,
// which marks the model unhealthy host-wide for inber's own context-assembly
// failure.
func TestTheOpenAILoopAnswersAContextOverflowWithNoGuardAtAll(t *testing.T) {
	body := `{"error":{"message":"This model's maximum context length is 128000 tokens","type":"invalid_request_error","code":"context_length_exceeded"}}`
	f := newRawOpenAI(t, http.StatusBadRequest, body, body, body)
	e := &Engine{modelClient: &agent.ModelClient{
		OpenAIClient: agent.NewOpenAIClient(f.server.URL, "test-key", "test-model"),
	}}

	_, err := e.runOpenAITurn(context.Background(), nil)
	if err == nil {
		t.Fatal("an overflowing turn returned no error")
	}
	if !strings.Contains(err.Error(), "context_length_exceeded") {
		t.Fatalf("error %q does not carry the provider's overflow code — the fake is not being read", err)
	}
	if len(f.requests) != 1 {
		t.Errorf("the loop made %d requests after an overflow — it grew a compaction retry and the table above is stale", len(f.requests))
	}
}
