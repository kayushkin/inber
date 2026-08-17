package engine

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/guard"
	modelstore "github.com/kayushkin/model-store"
)

// The tests in this file pin what recordTurnUsage ASKS FOR, as distinct from
// what session.CalcCostWithCache answers.
//
// The helper is pinned by fourteen assertions in session/timeline_cost_test.go,
// and max_cost_wiring_test.go's TestTurnCostReachesTheGuard used to say in
// prose that the price of a turn "belongs to the model registry and is pinned
// by the cost tests in session/, not here". That handoff cannot be honoured.
// Those tests hand the helper a registry themselves; they cannot see whether
// this caller passes e.modelStore or nil, because that argument is chosen here.
// Measured before these tests were written: replacing e.modelStore with nil in
// recordTurnUsage, and replacing the whole cache-aware call with the plain one,
// both left every package in this repo green.
//
// Both mutations are the defects timeline_cost.go's own doc comments record as
// having shipped — a nil registry prices every model at the unknown-model flat
// rate of $3.00/$15.00 per million, and dropping the cache adjustment bills
// cache reads at the full input rate and cache writes at nothing.

// The prices a turn is charged at here belong to the fixture registry, not to
// the live one. They matter only in that every one of them differs from the
// unknown-model flat rate, so no assertion below can be satisfied by a lookup
// that missed.
const (
	fixtureInputCostPer1M  = 0.80
	fixtureOutputCostPer1M = 4.00
	fixtureModel           = "claude-haiku-4-5-20251001"

	flatRateInputCostPer1M  = 3.00
	flatRateOutputCostPer1M = 15.00
)

// registryPricingOneModel is the smallest registry that can tell a real lookup
// from the flat rate: one model, priced unlike the fallback in both directions.
func registryPricingOneModel(t *testing.T) *modelstore.Store {
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
		ID: fixtureModel, Provider: "anthropic", Name: "Claude Haiku 4.5",
		MaxTokens: 200_000,
		InputCost: fixtureInputCostPer1M, OutputCost: fixtureOutputCostPer1M,
		Enabled: true,
	}); err != nil {
		t.Fatalf("add model: %v", err)
	}
	return store
}

// theFlatRateFor is the wrong answer, computed the way a lookup that never
// reached a registry computes it. Asserting against it by name means a
// regression that reintroduces the flat rate is reported as that, rather than
// as two numbers that happen to differ.
func theFlatRateFor(inputTokens, outputTokens, cacheRead, cacheWrite int) float64 {
	return (float64(inputTokens)*flatRateInputCostPer1M +
		float64(cacheRead)*flatRateInputCostPer1M*0.1 +
		float64(cacheWrite)*flatRateInputCostPer1M*1.25 +
		float64(outputTokens)*flatRateOutputCostPer1M) / 1_000_000
}

// TestTheTurnChargeIsPricedByTheRegistry pins the argument itself. A turn on a
// model registered below the fallback must be charged the registered price.
//
// This is the half of the flat-rate defect that lived at the call site. The
// comment on timeline_cost.go records what it cost when it last shipped: a
// Haiku sub-agent billed at twelve times its registered price, an Opus one at a
// fifth of its own.
func TestTheTurnChargeIsPricedByTheRegistry(t *testing.T) {
	const inputTokens, outputTokens = 1_000_000, 100_000

	e := &Engine{Model: fixtureModel, modelStore: registryPricingOneModel(t)}
	e.recordTurnUsage(&agent.TurnResult{InputTokens: inputTokens, OutputTokens: outputTokens})

	want := (inputTokens*fixtureInputCostPer1M + outputTokens*fixtureOutputCostPer1M) / 1_000_000
	if e.Tokens.Cost != want {
		t.Errorf("the turn was charged $%.4f, want $%.4f from the registered $%.2f/$%.2f per 1M",
			e.Tokens.Cost, want, fixtureInputCostPer1M, fixtureOutputCostPer1M)
	}
	if flatRate := theFlatRateFor(inputTokens, outputTokens, 0, 0); e.Tokens.Cost == flatRate {
		t.Errorf("the turn was charged $%.4f, the unknown-model flat rate — "+
			"the registry never reached the calculation, so every model is billed alike", flatRate)
	}
}

// TestTheTurnChargeCountsTheCacheTraffic pins that the caller asks the
// cache-aware function and not the plain one. The two agree exactly when a turn
// has no cache traffic, which is why this needs its own fixture rather than
// more assertions on the one above.
//
// Cache reads cost a tenth of input and cache writes cost a quarter more than
// it, so dropping the adjustment does not merely round: on the numbers below it
// charges $0.12 for a turn that cost $0.352.
func TestTheTurnChargeCountsTheCacheTraffic(t *testing.T) {
	const inputTokens, outputTokens = 100_000, 10_000
	const cacheRead, cacheWrite = 400_000, 200_000

	e := &Engine{Model: fixtureModel, modelStore: registryPricingOneModel(t)}
	e.recordTurnUsage(&agent.TurnResult{
		InputTokens: inputTokens, OutputTokens: outputTokens,
		CacheReadTokens: cacheRead, CacheCreationTokens: cacheWrite,
	})

	want := (inputTokens*fixtureInputCostPer1M +
		cacheRead*fixtureInputCostPer1M*0.1 +
		cacheWrite*fixtureInputCostPer1M*1.25 +
		outputTokens*fixtureOutputCostPer1M) / 1_000_000
	if e.Tokens.Cost != want {
		t.Errorf("the turn was charged $%.4f, want $%.4f", e.Tokens.Cost, want)
	}

	// The specific wrong answer dropping the adjustment produces. Named, so a
	// regression is reported as the cache traffic going unbilled rather than as
	// an arithmetic difference.
	ignoringTheCache := (inputTokens*fixtureInputCostPer1M + outputTokens*fixtureOutputCostPer1M) / 1_000_000
	if e.Tokens.Cost == ignoringTheCache {
		t.Errorf("the turn was charged $%.4f, which is its uncached tokens alone — "+
			"%d cache-read and %d cache-write tokens were billed at nothing",
			ignoringTheCache, cacheRead, cacheWrite)
	}
	if flatRate := theFlatRateFor(inputTokens, outputTokens, cacheRead, cacheWrite); e.Tokens.Cost == flatRate {
		t.Errorf("the cached turn was charged $%.4f, the unknown-model flat rate", flatRate)
	}
}

// TestTheCostCapIsEnforcedAtTheRegisteredPrice is the consequence, asserted
// where a user would feel it. A cap is a promise about dollars, so a turn
// mispriced on its way to the guard stops a session that had not reached its
// cap — or lets one run past it.
//
// The two caps straddle the turn's real price of $1.20. The upper one is the
// load-bearing case: under the flat rate the same turn prices at $4.50 and a
// session capped at $2.00 is refused after one turn. The lower one is there so
// that a recordTurnUsage which charged nothing at all could not satisfy the
// upper one on its own.
func TestTheCostCapIsEnforcedAtTheRegisteredPrice(t *testing.T) {
	const inputTokens, outputTokens = 1_000_000, 100_000
	const registeredPrice = 1.20 // Haiku's $0.80/$4.00 on those tokens.

	store := registryPricingOneModel(t)

	underCap := guard.New(guard.Config{MaxCost: 2.00})
	e := &Engine{Model: fixtureModel, modelStore: store, Guard: underCap}
	e.recordTurnUsage(&agent.TurnResult{InputTokens: inputTokens, OutputTokens: outputTokens})

	if got := underCap.CostSoFar(); got != registeredPrice {
		t.Errorf("the guard is enforcing against $%.4f, want the registered $%.4f", got, registeredPrice)
	}
	if exceeded, reason := underCap.CheckLimits(); exceeded {
		t.Errorf("a $%.2f turn under a $2.00 cap was refused (%q) — at the unknown-model flat rate "+
			"the same turn prices at $%.2f, which is over the cap the user set",
			registeredPrice, reason, theFlatRateFor(inputTokens, outputTokens, 0, 0))
	}

	overCap := guard.New(guard.Config{MaxCost: 1.00})
	e = &Engine{Model: fixtureModel, modelStore: store, Guard: overCap}
	e.recordTurnUsage(&agent.TurnResult{InputTokens: inputTokens, OutputTokens: outputTokens})

	if exceeded, _ := overCap.CheckLimits(); !exceeded {
		t.Errorf("a $%.2f turn under a $1.00 cap was allowed to continue — the cap is not being "+
			"enforced against the turn charge at all", registeredPrice)
	}
}

// TestTheCacheMissReportIsPricedByTheRegistry covers the second call site in
// this function. The report exists to answer, from this host's own numbers,
// whether a guaranteed-prose summary is worth buying with the tools block — so
// a figure carrying the flat rate is not a smaller problem than a missing one.
// It is the wrong answer to the question the line was added to ask.
func TestTheCacheMissReportIsPricedByTheRegistry(t *testing.T) {
	const callInput, callWrite = 200_000, 200_000

	e := &Engine{Model: fixtureModel, modelStore: registryPricingOneModel(t)}
	logged := captureStderr(t, func() {
		e.recordTurnUsage(&agent.TurnResult{
			InputTokens: callInput, CacheCreationTokens: callWrite,
			APICalls: []agent.APICallUsage{
				{InputTokens: callInput, CacheCreationTokens: callWrite, ToolsWithheld: true},
			},
		})
	})

	want := (callInput*fixtureInputCostPer1M + callWrite*fixtureInputCostPer1M*1.25) / 1_000_000
	if !strings.Contains(logged, fmt.Sprintf("$%.4f", want)) {
		t.Errorf("the cache-miss report does not carry the call's registered price of $%.4f.\n"+
			"stderr was: %q", want, logged)
	}
	flatRate := theFlatRateFor(callInput, 0, 0, callWrite)
	if strings.Contains(logged, fmt.Sprintf("$%.4f", flatRate)) {
		t.Errorf("the cache-miss report priced the call at $%.4f, the unknown-model flat rate — "+
			"the number an operator would size the force-summary cost from is %.1fx the real one.\n"+
			"stderr was: %q", flatRate, flatRate/want, logged)
	}
}

// TestAnUnregisteredModelStillFallsBackToTheFlatRate is the complement, and it
// is what keeps these tests from being satisfied by refusing to price anything.
// A model the registry does not carry has no registered price, and the flat
// rate is the documented answer there — asserting it explicitly means a change
// that started returning zero for unknown models fails here rather than looking
// like every other cost assertion passing.
func TestAnUnregisteredModelStillFallsBackToTheFlatRate(t *testing.T) {
	const inputTokens, outputTokens = 1_000_000, 100_000

	e := &Engine{Model: "a-model-nobody-registered", modelStore: registryPricingOneModel(t)}
	e.recordTurnUsage(&agent.TurnResult{InputTokens: inputTokens, OutputTokens: outputTokens})

	want := theFlatRateFor(inputTokens, outputTokens, 0, 0)
	if e.Tokens.Cost != want {
		t.Errorf("an unregistered model was charged $%.4f, want the $%.2f/$%.2f fallback of $%.4f",
			e.Tokens.Cost, flatRateInputCostPer1M, flatRateOutputCostPer1M, want)
	}
}
