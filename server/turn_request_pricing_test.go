package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/kayushkin/inber/engine"
	modelstore "github.com/kayushkin/model-store"
)

// This file pins what the completed-turn path in Server.run ASKS FOR, as
// distinct from what session.CalcCostWithCache answers.
//
// TokenUsage.Cost assigned here is the figure api_bridge.go reports to
// llm-bridge as the turn's TotalUSD, and it is the same figure written to the
// request row. It is the only price anything outside inber sees. The helper
// that computes it is pinned by fourteen assertions in session/, but those
// tests hand the helper a registry themselves; they cannot see whether this
// caller passes g.modelStore or nil, nor whether it asks the cache-aware
// function or the plain one, because both are chosen here. Measured before this
// file existed: both mutations left every package in this repo green.
//
// The two defects are the ones session/timeline_cost.go's own doc comments
// record as having shipped. A nil registry prices every model at the
// unknown-model flat rate of $3.00/$15.00 per million; dropping the cache
// adjustment bills cache reads at the full input rate and cache writes at
// nothing.

// The fixture registry STRADDLES the unknown-model fallback: its input price is
// above $3.00 and its output price is below $15.00.
//
// That is deliberate, and it is the difference between this fixture and one
// that merely differs from the fallback. Measured 2026-08-17 against the live
// model-store registry, over all 56 models it carries:
//
//	 0 of 56 straddle the fallback. Every model is at-or-below the fallback on
//	         both prices or at-or-above it on both, so no real model has this
//	         fixture's shape and no real model can stand in for it.
//	15 of 56 are priced so that the fallback is the registered price times ONE
//	         constant. An assertion on such a model can be satisfied by a
//	         calculation that scaled rather than looked anything up.
//	 4 of those 15 are priced at exactly $3.00/$15.00 — every Sonnet row,
//	         including the model this host runs by default. A fixture on Sonnet
//	         cannot tell a registry hit from a registry miss at all: passing nil
//	         computes the identical float, so the argument under test here would
//	         be unpinned by a test that looked like it pinned it.
//
// Straddling removes all three. No single factor carries $5.00/$2.00 to
// $3.00/$15.00, so a figure that reached the fallback is wrong in one direction
// on input and the other on output and cannot coincide with the right answer.
const (
	straddlingInputCostPer1M  = 5.00
	straddlingOutputCostPer1M = 2.00
	straddlingModel           = "a-model-priced-either-side-of-the-fallback"

	fallbackInputCostPer1M  = 3.00
	fallbackOutputCostPer1M = 15.00
)

// The token counts the stubbed provider reports for the one turn these tests
// run. Cache traffic is present and large, because the two cost functions agree
// exactly on a turn that has none — a fixture without it cannot see the
// cache adjustment being dropped.
const (
	turnInputTokens  = 100_000
	turnOutputTokens = 10_000
	turnCacheRead    = 400_000
	turnCacheWrite   = 200_000
)

func registryStraddlingTheFallback(t *testing.T) *modelstore.Store {
	t.Helper()

	store, err := modelstore.Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("open model store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	if err := store.AddProvider(modelstore.Provider{ID: "anthropic", Name: "Anthropic"}); err != nil {
		t.Fatalf("add provider: %v", err)
	}
	if err := store.AddModel(modelstore.Model{
		ID: straddlingModel, Provider: "anthropic", Name: "Straddling fixture model",
		MaxTokens: 200_000,
		InputCost: straddlingInputCostPer1M, OutputCost: straddlingOutputCostPer1M,
		Enabled: true,
	}); err != nil {
		t.Fatalf("add model: %v", err)
	}
	return store
}

// theFallbackRateFor is the wrong answer, computed the way a calculation that
// never reached a registry computes it.
func theFallbackRateFor(inputTokens, outputTokens, cacheRead, cacheWrite int) float64 {
	return (float64(inputTokens)*fallbackInputCostPer1M +
		float64(cacheRead)*fallbackInputCostPer1M*0.1 +
		float64(cacheWrite)*fallbackInputCostPer1M*1.25 +
		float64(outputTokens)*fallbackOutputCostPer1M) / 1_000_000
}

// atTheRegisteredPrice is the right answer for the straddling fixture.
func atTheRegisteredPrice(inputTokens, outputTokens, cacheRead, cacheWrite int) float64 {
	return (float64(inputTokens)*straddlingInputCostPer1M +
		float64(cacheRead)*straddlingInputCostPer1M*0.1 +
		float64(cacheWrite)*straddlingInputCostPer1M*1.25 +
		float64(outputTokens)*straddlingOutputCostPer1M) / 1_000_000
}

// aProviderReportingCacheTraffic is a real HTTP endpoint speaking the Anthropic
// Messages wire format, reached through the real SDK client. The engine carries
// no model client, so buildAgent falls back to Engine.Client and every layer
// below this — the SDK, the agent loop, Engine.RunTurn, Session.turn, the queue
// — is the shipping one. Only the far side of the socket is a fixture.
//
// It answers with a server-sent event stream rather than a single JSON body,
// because that is the call the engine makes: the display hooks a session
// installs set OnTextDelta, and executeAPICall takes the streaming branch
// whenever that hook is present. A fixture answering the non-streaming shape
// is served to a client that never asked for it, and the SDK accumulates an
// empty message with no stop reason and no usage — which fails the turn before
// anything is priced rather than pricing it wrongly.
//
// The four token counts arrive by two different routes and that is the point:
// input and both cache figures are named once in message_start, and the output
// count only in message_delta at the end. A fixture that carried them all in
// one frame would not exercise the accumulation the real usage figures depend
// on.
func aProviderReportingCacheTraffic(t *testing.T) *anthropic.Client {
	t.Helper()

	frame := func(event string, payload map[string]any) string {
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encoding the %s frame: %v", event, err)
		}
		return "event: " + event + "\ndata: " + string(body) + "\n\n"
	}

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, frame("message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": "msg_fixture", "type": "message", "role": "assistant",
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
			"content_block": map[string]any{"type": "text", "text": ""},
		}))
		io.WriteString(w, frame("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": "answered"},
		}))
		io.WriteString(w, frame("content_block_stop", map[string]any{
			"type": "content_block_stop", "index": 0,
		}))
		io.WriteString(w, frame("message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
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
	return &client
}

// aCompletedTurn drives Server.run end to end against the stubbed provider and
// returns the cost that reached the request row.
//
// The session is pre-stored so that getOrCreateSession hands the turn this
// engine rather than building one from config — the engine is the only part
// that has to be a fixture, because a real one would need credentials for a
// live provider. The Server's own registry, which is the argument under test,
// is the real thing.
func aCompletedTurn(t *testing.T, model string, registry *modelstore.Store) (float64, TokenUsage) {
	t.Helper()

	const agentName = "brigid"

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
			Client: aProviderReportingCacheTraffic(t),
			Model:  model,
		},
	}
	server.sessions.Store(key, session)

	resp, err := server.Run(context.Background(), RunRequest{Agent: agentName, Message: "hello"})
	if err != nil {
		t.Fatalf("the turn failed before it could be priced: %v", err)
	}

	requests, err := server.store.RecentRequests(key, 1)
	if err != nil {
		t.Fatalf("reading the request row back: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("the turn wrote %d request rows, want exactly 1 — the fixture never reached "+
			"the row whose cost this file asserts", len(requests))
	}
	if requests[0].Status != "completed" {
		t.Fatalf("the request row is %q, not %q — this fixture drives the completed-turn path",
			requests[0].Status, "completed")
	}

	// The token counts are asserted here rather than in each test because a row
	// priced from the wrong tokens would satisfy no cost assertion for an
	// honest reason, and the failure should say which of the two went wrong.
	if requests[0].InputTokens != turnInputTokens || requests[0].OutputTokens != turnOutputTokens ||
		requests[0].CacheReadTokens != turnCacheRead || requests[0].CacheWriteTokens != turnCacheWrite {
		t.Fatalf("the request row recorded in=%d out=%d cacheRead=%d cacheWrite=%d, want %d/%d/%d/%d — "+
			"the turn was priced from tokens other than the ones the provider reported",
			requests[0].InputTokens, requests[0].OutputTokens, requests[0].CacheReadTokens, requests[0].CacheWriteTokens,
			turnInputTokens, turnOutputTokens, turnCacheRead, turnCacheWrite)
	}

	return requests[0].Cost, resp.Tokens
}

// TestTheTurnChargeOnTheRequestRowIsPricedByTheRegistry pins the argument
// itself, on the path that produces the only price anything outside inber sees.
func TestTheTurnChargeOnTheRequestRowIsPricedByTheRegistry(t *testing.T) {
	cost, _ := aCompletedTurn(t, straddlingModel, registryStraddlingTheFallback(t))

	want := atTheRegisteredPrice(turnInputTokens, turnOutputTokens, turnCacheRead, turnCacheWrite)
	if cost != want {
		t.Errorf("the completed turn was charged $%.4f, want $%.4f from the registered $%.2f/$%.2f per 1M",
			cost, want, straddlingInputCostPer1M, straddlingOutputCostPer1M)
	}
	if fallback := theFallbackRateFor(turnInputTokens, turnOutputTokens, turnCacheRead, turnCacheWrite); cost == fallback {
		t.Errorf("the completed turn was charged $%.4f, the unknown-model flat rate — the registry "+
			"never reached the calculation, so every model is billed alike", fallback)
	}
}

// TestTheTurnChargeOnTheRequestRowCountsTheCacheTraffic pins that this caller
// asks the cache-aware function and not the plain one. Cache reads cost a tenth
// of input and cache writes cost a quarter more than it, so dropping the
// adjustment does not merely round: on this fixture's tokens a turn that cost
// $1.77 is charged $0.52.
func TestTheTurnChargeOnTheRequestRowCountsTheCacheTraffic(t *testing.T) {
	cost, _ := aCompletedTurn(t, straddlingModel, registryStraddlingTheFallback(t))

	ignoringTheCache := atTheRegisteredPrice(turnInputTokens, turnOutputTokens, 0, 0)
	if cost == ignoringTheCache {
		t.Errorf("the turn was charged $%.4f, which is its uncached tokens alone — "+
			"%d cache-read and %d cache-write tokens were billed at nothing",
			ignoringTheCache, turnCacheRead, turnCacheWrite)
	}
}

// TestTheBridgeIsToldTheSameCostAsTheRequestRow is the assertion this card was
// filed for. TokenUsage.Cost is what api_bridge.go reports as TotalUSD; it was
// once computed into a local and never assigned, so every bridge session billed
// $0.00 however much it spent. Two figures for one turn is the shape that let
// them disagree, so they are asserted against each other rather than only
// against a number.
func TestTheBridgeIsToldTheSameCostAsTheRequestRow(t *testing.T) {
	cost, tokens := aCompletedTurn(t, straddlingModel, registryStraddlingTheFallback(t))

	if tokens.Cost != cost {
		t.Errorf("the caller was told the turn cost $%.4f and the request row says $%.4f — "+
			"the two prices for one turn disagree", tokens.Cost, cost)
	}
	if tokens.Cost == 0 {
		t.Error("the caller was told the turn cost $0.0000 — TotalUSD is unassigned again")
	}
}

// TestAnUnregisteredModelIsStillChargedTheFallbackRate is the complement, and
// it is what keeps the tests above from being satisfied by a path that refused
// to price anything. A model the registry does not carry has no registered
// price and the fallback is the documented answer.
func TestAnUnregisteredModelIsStillChargedTheFallbackRate(t *testing.T) {
	cost, _ := aCompletedTurn(t, "a-model-nobody-registered", registryStraddlingTheFallback(t))

	want := theFallbackRateFor(turnInputTokens, turnOutputTokens, turnCacheRead, turnCacheWrite)
	if cost != want {
		t.Errorf("an unregistered model was charged $%.4f, want the $%.2f/$%.2f fallback of $%.4f",
			cost, fallbackInputCostPer1M, fallbackOutputCostPer1M, want)
	}
}
