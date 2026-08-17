package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/kayushkin/inber/engine"
	modelstore "github.com/kayushkin/model-store"
)

// This file pins what the FAILED-turn path in Server.run asks for. It is the
// sibling of turn_request_pricing_test.go, which pins the completed one, and it
// exists because the two are separate call sites: server.go prices the error
// row at its own line, with its own arguments, and a mutation of that line was
// invisible to every test in this repo.
//
// A turn that fails after the model has already been paid for is not a corner
// case. The API call that fails is rarely the first one — a tool-using turn
// makes several, and every one before the failure was billed. The error row is
// where that spend is recorded, and it is the only record of it.
//
// ⚠️ Why this needs a two-response fixture, which is the whole difficulty of the
// row. Token counts are accumulated in Agent.processResponse, and Agent.Run
// returns before reaching it when the API call errors. So a turn whose FIRST
// call fails carries four zeros, CalcCostWithCache returns $0.00 under every
// mutation of this line, and a fixture built that way scores the row as covered
// while asserting nothing. The provider below therefore answers the first call
// with a tool_use the turn must follow up, and fails the second.

// aProviderThatIsPaidAndThenFails serves one billable tool_use response and
// fails every call after it. It counts requests, and the count is asserted:
// a fixture that silently failed to intercept would send this turn to whatever
// the environment's real Anthropic endpoint is.
//
// The tool it asks for is one no session carries. That is deliberate and it is
// the cheapest way to make the agent loop come back for a second API call:
// an unknown tool is reported to the model as an error tool_result and the turn
// continues, so no tool has to run for the turn to be billed twice.
func aProviderThatIsPaidAndThenFails(t *testing.T) (*anthropic.Client, func() int) {
	t.Helper()

	frame := func(event string, payload map[string]any) string {
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encoding the %s frame: %v", event, err)
		}
		return "event: " + event + "\ndata: " + string(body) + "\n\n"
	}

	var mu sync.Mutex
	calls := 0

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		nth := calls
		mu.Unlock()

		if nth > 1 {
			// The provider stops answering. Everything the turn was already
			// charged for stands, and that is what the error row must record.
			http.Error(w, `{"type":"error","error":{"type":"api_error","message":"the provider gave up"}}`,
				http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, frame("message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": "msg_paid_then_failed", "type": "message", "role": "assistant",
				"model": straddlingModel, "content": []any{},
				"stop_reason": nil, "stop_sequence": nil,
				"usage": map[string]any{
					"input_tokens":                turnInputTokens,
					"output_tokens":               0,
					"cache_read_input_tokens":     turnCacheRead,
					"cache_creation_input_tokens": turnCacheWrite,
				},
			},
		}))
		io.WriteString(w, frame("content_block_start", map[string]any{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]any{
				"type": "tool_use", "id": "toolu_fixture",
				"name": "a_tool_no_session_carries", "input": map[string]any{},
			},
		}))
		io.WriteString(w, frame("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": "{}"},
		}))
		io.WriteString(w, frame("content_block_stop", map[string]any{
			"type": "content_block_stop", "index": 0,
		}))
		io.WriteString(w, frame("message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": "tool_use", "stop_sequence": nil},
			"usage": map[string]any{"output_tokens": turnOutputTokens},
		}))
		io.WriteString(w, frame("message_stop", map[string]any{"type": "message_stop"}))
	}))
	t.Cleanup(provider.Close)

	client := anthropic.NewClient(
		option.WithBaseURL(provider.URL),
		option.WithAPIKey("unused"),
		option.WithMaxRetries(0),
	)

	// Force the fallback this fixture is built on, rather than depending on the
	// environment to provide it.
	//
	// The engine below carries no model client, so executeAgent asks
	// agent.NewModelClient for one and only falls back to Engine.Client — the
	// stub — when that fails. It fails because no credential can be resolved,
	// and with a nil auth store the last place it looks is ANTHROPIC_API_KEY.
	// So on a host with that variable set, the engine would build its OWN
	// client, reach a real provider, and the stub above would be bypassed
	// entirely. Emptying it here makes the path under test the one this fixture
	// names, on any host.
	//
	// The request count is still asserted, and deliberately: this line removes
	// the likeliest way the premise breaks, not every way. A premise that
	// breaks anyway must fail the test rather than quietly send a real turn.
	t.Setenv("ANTHROPIC_API_KEY", "")

	countRequests := func() int {
		mu.Lock()
		defer mu.Unlock()
		return calls
	}
	return &client, countRequests
}

// aTurnThatFailedAfterBeingBilled drives Server.run to the error branch and
// returns the cost written to the request row.
//
// It mirrors aCompletedTurn deliberately: same server, same pre-stored session,
// same registry under test. Only the provider differs, so a difference in the
// two rows' verdicts is a difference between the two CALL SITES and not between
// two fixtures.
func aTurnThatFailedAfterBeingBilled(t *testing.T, model string, registry *modelstore.Store) RequestRow {
	t.Helper()

	const agentName = "brigid"

	provider, countRequests := aProviderThatIsPaidAndThenFails(t)

	server := &Server{
		store:      tempStore(t),
		queue:      NewQueue(map[string]int{"main": 1}),
		modelStore: registry,
		config: Config{
			DataDir:      t.TempDir(),
			DefaultAgent: agentName,
			Agents:       map[string]AgentConfig{agentName: {}},
		},
	}

	key := mainSessionKey(agentName)
	session := &Session{
		Key:        key,
		AgentName:  agentName,
		Status:     Idle,
		injections: make(chan string, 1),
		Engine: &engine.Engine{
			Client: provider,
			Model:  model,
		},
	}
	server.sessions.Store(key, session)

	if _, err := server.Run(context.Background(), RunRequest{Agent: agentName, Message: "hello"}); err == nil {
		t.Fatal("the turn succeeded; this fixture drives the failed-turn path and has nothing to assert")
	}

	// The safety assertion, and it is not a formality. If the stub were never
	// reached — a base URL that did not take, a client the engine replaced —
	// the turn would have gone to a live provider on this host's credentials.
	// Zero requests is that case, and it must fail rather than read as a turn
	// that simply cost nothing.
	if got := countRequests(); got < 2 {
		t.Fatalf("the stubbed provider was asked %d times, want at least 2 — "+
			"the turn was not billed before it failed, so this fixture cannot see the error row's price", got)
	}

	requests, err := server.store.RecentRequests(key, 1)
	if err != nil {
		t.Fatalf("reading the request row back: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("the failed turn wrote %d request rows, want exactly 1", len(requests))
	}
	if requests[0].Status != "error" {
		t.Fatalf("the request row is %q, not %q — this fixture drives the failed-turn path",
			requests[0].Status, "error")
	}
	return requests[0]
}

// TestTheFailedTurnIsChargedAtTheRegisteredPrice pins the argument at
// server.go's error branch. The registry reaches this call site by its own
// line, and nothing else in the repo asserts on it: replacing g.modelStore with
// nil here left every package green before this file existed.
//
// The consequence is not cosmetic. A turn that fails mid-way is precisely the
// turn most likely to have made several expensive calls, and the error row is
// the only place its spend is written down — the completed-turn row is never
// reached. Priced at the flat rate, a Haiku session's failures are billed at
// roughly four times what they cost and an Opus session's at a fraction.
func TestTheFailedTurnIsChargedAtTheRegisteredPrice(t *testing.T) {
	row := aTurnThatFailedAfterBeingBilled(t, straddlingModel, registryStraddlingTheFallback(t))

	want := atTheRegisteredPrice(row.InputTokens, row.OutputTokens, row.CacheReadTokens, row.CacheWriteTokens)
	if row.Cost != want {
		t.Errorf("the failed turn was charged $%.4f, want $%.4f from the registered $%.2f/$%.2f per 1M",
			row.Cost, want, straddlingInputCostPer1M, straddlingOutputCostPer1M)
	}
	if fallback := theFallbackRateFor(row.InputTokens, row.OutputTokens, row.CacheReadTokens, row.CacheWriteTokens); row.Cost == fallback {
		t.Errorf("the failed turn was charged $%.4f, the unknown-model flat rate — the registry "+
			"never reached the error branch's calculation", fallback)
	}
}

// TestTheFailedTurnRecordsTheTokensItWasAlreadyBilledFor is the assertion the
// two tests above rest on, and it is stated separately because it is the one
// that can quietly stop being true. Every cost assertion on this path is
// vacuous if the row carries four zeros: $0.00 equals $0.00 at any price, so a
// change that lost the partial result would turn both of them green forever
// rather than failing.
//
// It was shown able to say "no" before being trusted to say "yes". Control run
// 2026-08-17: the provider's first response was edited to report all four token
// counts as zero, leaving the two-call shape intact so the request-count guard
// above still passed. This test then failed on its own message. A guard no case
// can redden is a comment, not a rule.
func TestTheFailedTurnRecordsTheTokensItWasAlreadyBilledFor(t *testing.T) {
	row := aTurnThatFailedAfterBeingBilled(t, straddlingModel, registryStraddlingTheFallback(t))

	if row.InputTokens == 0 && row.OutputTokens == 0 && row.CacheReadTokens == 0 && row.CacheWriteTokens == 0 {
		t.Fatal("the failed turn recorded no tokens at all — the call the provider was paid for " +
			"was dropped with the error, and every price assertion on this path is comparing $0.00 to $0.00")
	}
	if row.Cost == 0 {
		t.Errorf("the failed turn recorded in=%d out=%d cacheRead=%d cacheWrite=%d and a cost of $0.00 — "+
			"the tokens survived the error and the charge for them did not",
			row.InputTokens, row.OutputTokens, row.CacheReadTokens, row.CacheWriteTokens)
	}
}

// TestTheFailedTurnCountsTheCacheTraffic pins that the error branch asks the
// cache-aware function and not the plain one — the second of the two defects,
// and the one a nil-registry assertion cannot see, because dropping the cache
// adjustment leaves the registry lookup intact.
func TestTheFailedTurnCountsTheCacheTraffic(t *testing.T) {
	row := aTurnThatFailedAfterBeingBilled(t, straddlingModel, registryStraddlingTheFallback(t))

	if row.CacheReadTokens == 0 && row.CacheWriteTokens == 0 {
		t.Fatal("the failed turn recorded no cache traffic; this assertion cannot see the adjustment being dropped")
	}
	ignoringTheCache := atTheRegisteredPrice(row.InputTokens, row.OutputTokens, 0, 0)
	if row.Cost == ignoringTheCache {
		t.Errorf("the failed turn was charged $%.4f, which is its uncached tokens alone — "+
			"%d cache-read and %d cache-write tokens were billed at nothing",
			ignoringTheCache, row.CacheReadTokens, row.CacheWriteTokens)
	}
}

// TestAFailedTurnOnAnUnregisteredModelStillPaysTheFallbackRate is the
// complement, and it is what stops the three tests above being satisfied by an
// error branch that refused to price anything at all. A model the registry does
// not carry has no registered price, and the documented answer is the fallback
// — not zero, and not an error.
func TestAFailedTurnOnAnUnregisteredModelStillPaysTheFallbackRate(t *testing.T) {
	row := aTurnThatFailedAfterBeingBilled(t, "a-model-nobody-registered", registryStraddlingTheFallback(t))

	want := theFallbackRateFor(row.InputTokens, row.OutputTokens, row.CacheReadTokens, row.CacheWriteTokens)
	if row.Cost != want {
		t.Errorf("a failed turn on an unregistered model was charged $%.4f, want the $%.2f/$%.2f fallback of $%.4f",
			row.Cost, fallbackInputCostPer1M, fallbackOutputCostPer1M, want)
	}
}
