package session

import (
	"encoding/json"
	"path/filepath"
	"testing"

	modelstore "github.com/kayushkin/model-store"
)

// This file pins what Session.EndTurn ASKS FOR, as distinct from what
// CalcCostWithCache answers.
//
// The helper pair in timeline_cost.go is pinned by fourteen assertions in
// timeline_cost_test.go, and those tests hand the helper a registry themselves.
// They cannot see whether this caller passes s.modelStore or nil, nor whether
// it asks the cache-aware function or the plain one, because both of those are
// chosen here. Measured before this file existed: replacing s.modelStore with
// nil in EndTurn left every package in this repo green.
//
// The two mutations are the defects timeline_cost.go's own doc comments record
// as having shipped. A nil registry prices every model at the unknown-model
// flat rate of $3.00/$15.00 per million; dropping the cache adjustment bills
// cache reads at the full input rate and cache writes at nothing.

// The fixture registry STRADDLES the unknown-model fallback: its input price is
// above $3.00 and its output price is below $15.00.
//
// That is deliberate and it is the difference between this fixture and one that
// merely differs from the fallback. Every price on this host's real registry is
// uniformly cheaper or uniformly dearer than the fallback, so a cost computed
// from the fallback is the registered cost times a single constant, and an
// assertion can be satisfied by a calculation that scaled rather than looked
// up. Straddling removes that: no single factor carries $5.00/$2.00 to
// $3.00/$15.00, so a figure that reached the fallback is wrong in one direction
// on input and the other on output and cannot coincide with the right answer.
const (
	straddlingInputCostPer1M  = 5.00
	straddlingOutputCostPer1M = 2.00
	straddlingModel           = "a-model-priced-either-side-of-the-fallback"

	fallbackInputCostPer1M  = 3.00
	fallbackOutputCostPer1M = 15.00
)

// registryStraddlingTheFallback is the smallest registry that can tell a real
// lookup from the flat rate in both directions at once.
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
// never reached a registry computes it. Naming it means a regression that
// reintroduces the flat rate is reported as that, rather than as two numbers
// that happen to differ.
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

// aLoggedTurn drives the real path a turn's cost takes into the turns table:
// a real Session over a real JSONL log, a real SQLite store, LogRequest to open
// turn 1, then EndTurn to close it. It returns the cost that reached the row.
//
// Nothing here is a double. The card this file answers warns that a fake
// derived from the code under test cannot disagree with it, and the store is
// the one component whose disagreement is the whole measurement — the number
// asserted below is read back out of SQLite, not off a captured argument.
func aLoggedTurn(t *testing.T, model string, registry *modelstore.Store, tokens TurnTokens) float64 {
	t.Helper()

	session, err := New(t.TempDir(), model, "test", "", registry)
	if err != nil {
		t.Fatalf("building the session: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	store := openTestDB(t)
	session.AttachStore(store, "test")

	session.LogRequest(json.RawMessage(`{}`))
	session.EndTurn(tokens, 0, "end_turn", "")

	turns, err := store.GetTurns(session.SessionID())
	if err != nil {
		t.Fatalf("reading the turns back: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("the session log wrote %d turn rows, want exactly 1 — "+
			"the fixture never reached the row whose cost this file asserts", len(turns))
	}
	return turns[0].Cost
}

// TestTheLoggedTurnCostIsPricedByTheRegistry pins the argument itself. The
// per-turn cost written to the turns table is what a session's spend is read
// back from after the fact, so a turn priced at the fallback misreports every
// model at once and leaves no trace saying so.
func TestTheLoggedTurnCostIsPricedByTheRegistry(t *testing.T) {
	const inputTokens, outputTokens = 1_000_000, 100_000

	cost := aLoggedTurn(t, straddlingModel, registryStraddlingTheFallback(t),
		TurnTokens{Input: inputTokens, Output: outputTokens})

	want := atTheRegisteredPrice(inputTokens, outputTokens, 0, 0)
	if cost != want {
		t.Errorf("the logged turn cost $%.4f, want $%.4f from the registered $%.2f/$%.2f per 1M",
			cost, want, straddlingInputCostPer1M, straddlingOutputCostPer1M)
	}
	if fallback := theFallbackRateFor(inputTokens, outputTokens, 0, 0); cost == fallback {
		t.Errorf("the logged turn cost $%.4f, the unknown-model flat rate — the registry never "+
			"reached the calculation, so every model is logged at the same price per token", fallback)
	}
}

// TestTheLoggedTurnCostCountsTheCacheTraffic pins that this caller asks the
// cache-aware function and not the plain one. The two agree exactly on a turn
// with no cache traffic, which is why it needs its own fixture rather than more
// assertions on the one above.
//
// Cache reads cost a tenth of input and cache writes cost a quarter more than
// it, so dropping the adjustment does not merely round. On the numbers below a
// turn that cost $1.76 is logged at $0.52.
func TestTheLoggedTurnCostCountsTheCacheTraffic(t *testing.T) {
	const inputTokens, outputTokens = 100_000, 10_000
	const cacheRead, cacheWrite = 400_000, 200_000

	cost := aLoggedTurn(t, straddlingModel, registryStraddlingTheFallback(t), TurnTokens{
		Input: inputTokens, Output: outputTokens,
		CacheRead: cacheRead, CacheWrite: cacheWrite,
	})

	want := atTheRegisteredPrice(inputTokens, outputTokens, cacheRead, cacheWrite)
	if cost != want {
		t.Errorf("the cached turn was logged at $%.4f, want $%.4f", cost, want)
	}

	// The specific wrong answer dropping the adjustment produces. Named, so a
	// regression reads as the cache traffic going unbilled rather than as an
	// arithmetic difference.
	ignoringTheCache := atTheRegisteredPrice(inputTokens, outputTokens, 0, 0)
	if cost == ignoringTheCache {
		t.Errorf("the turn was logged at $%.4f, which is its uncached tokens alone — "+
			"%d cache-read and %d cache-write tokens were logged as costing nothing",
			ignoringTheCache, cacheRead, cacheWrite)
	}
	if fallback := theFallbackRateFor(inputTokens, outputTokens, cacheRead, cacheWrite); cost == fallback {
		t.Errorf("the cached turn was logged at $%.4f, the unknown-model flat rate", fallback)
	}
}

// TestAnUnregisteredModelStillLogsTheFallbackRate is the complement, and it is
// what keeps the two tests above from being satisfied by a caller that refused
// to price anything. A model the registry does not carry has no registered
// price and the fallback is the documented answer, so asserting it explicitly
// means a change that started logging zero for unknown models fails here rather
// than looking like every other cost assertion passing.
func TestAnUnregisteredModelStillLogsTheFallbackRate(t *testing.T) {
	const inputTokens, outputTokens = 1_000_000, 100_000

	cost := aLoggedTurn(t, "a-model-nobody-registered", registryStraddlingTheFallback(t),
		TurnTokens{Input: inputTokens, Output: outputTokens})

	want := theFallbackRateFor(inputTokens, outputTokens, 0, 0)
	if cost != want {
		t.Errorf("an unregistered model logged $%.4f, want the $%.2f/$%.2f fallback of $%.4f",
			cost, fallbackInputCostPer1M, fallbackOutputCostPer1M, want)
	}
}
