package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	modelstore "github.com/kayushkin/model-store"
)

// The prices below are the ones the registry on this host actually carries for
// these models. They matter to the assertions only in that they differ from
// each other and from the unknown-model fallback of $3.00/$15.00 — a test that
// picked the fallback's own numbers for one of its models could not tell a
// registry hit from a miss.
const (
	haikuInputCostPer1M  = 0.80
	haikuOutputCostPer1M = 4.00
	opusInputCostPer1M   = 15.00
	opusOutputCostPer1M  = 75.00
)

// registryWithTwoDifferentlyPricedModels builds the smallest registry that can
// distinguish a real lookup from the flat rate: a cheap model and an expensive
// one, neither priced at the unknown-model fallback.
func registryWithTwoDifferentlyPricedModels(t *testing.T) *modelstore.Store {
	t.Helper()

	store, err := modelstore.Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	if err := store.AddProvider(modelstore.Provider{ID: "anthropic", Name: "Anthropic"}); err != nil {
		t.Fatalf("add provider: %v", err)
	}

	for _, model := range []modelstore.Model{
		{ID: "claude-haiku-4-5-20251001", Provider: "anthropic", Name: "Claude Haiku 4.5",
			MaxTokens: 200_000, InputCost: haikuInputCostPer1M, OutputCost: haikuOutputCostPer1M, Enabled: true},
		{ID: "claude-opus-4-20250514", Provider: "anthropic", Name: "Claude Opus 4",
			MaxTokens: 200_000, InputCost: opusInputCostPer1M, OutputCost: opusOutputCostPer1M, Enabled: true},
		{ID: "claude-sonnet-4-20250514", Provider: "anthropic", Name: "Claude Sonnet 4",
			MaxTokens: 200_000, InputCost: 3.00, OutputCost: 15.00, Enabled: true},
	} {
		if err := store.AddModel(model); err != nil {
			t.Fatalf("add model %s: %v", model.ID, err)
		}
	}
	return store
}

// TestCostDependsOnWhichModelRan is the defect this file was written for, in
// the shape the finding asked for: the same tokens on a cheap model and on an
// expensive one must not cost the same. Every live caller used to pass an empty
// model id and a nil store, so both landed on the unknown-model fallback and
// every spawn on the box reported the identical price per token.
//
// Asserting the difference rather than a number is deliberate. A test pinned to
// a figure goes red when the registry is re-synced with new list prices, which
// is a fact about Anthropic rather than about this code.
func TestCostDependsOnWhichModelRan(t *testing.T) {
	store := registryWithTwoDifferentlyPricedModels(t)

	const inputTokens, outputTokens = 1_000_000, 100_000

	haiku := CalcCost("claude-haiku-4-5-20251001", inputTokens, outputTokens, store)
	opus := CalcCost("claude-opus-4-20250514", inputTokens, outputTokens, store)

	if haiku == opus {
		t.Fatalf("Haiku and Opus both cost $%.4f for the same tokens — "+
			"an equal price means the model id never reached the registry", haiku)
	}
	if haiku >= opus {
		t.Fatalf("Haiku cost $%.4f and Opus cost $%.4f; the cheap model must be cheaper", haiku, opus)
	}

	wantHaiku := (inputTokens*haikuInputCostPer1M + outputTokens*haikuOutputCostPer1M) / 1_000_000
	if haiku != wantHaiku {
		t.Errorf("Haiku cost $%.4f, want $%.4f from its registered $%.2f/$%.2f per 1M",
			haiku, wantHaiku, haikuInputCostPer1M, haikuOutputCostPer1M)
	}
	wantOpus := (inputTokens*opusInputCostPer1M + outputTokens*opusOutputCostPer1M) / 1_000_000
	if opus != wantOpus {
		t.Errorf("Opus cost $%.4f, want $%.4f from its registered $%.2f/$%.2f per 1M",
			opus, wantOpus, opusInputCostPer1M, opusOutputCostPer1M)
	}
}

// TestCachedCostDependsOnWhichModelRan is the same assertion for the call the
// server actually makes. All four server call sites use the cache-aware
// function, so pinning only the plain one would leave the live path uncovered.
func TestCachedCostDependsOnWhichModelRan(t *testing.T) {
	store := registryWithTwoDifferentlyPricedModels(t)

	const inputTokens, outputTokens = 1_000_000, 100_000
	const cacheRead, cacheWrite = 400_000, 200_000

	haiku := CalcCostWithCache("claude-haiku-4-5-20251001", inputTokens, outputTokens, cacheRead, cacheWrite, store)
	opus := CalcCostWithCache("claude-opus-4-20250514", inputTokens, outputTokens, cacheRead, cacheWrite, store)

	if haiku == opus {
		t.Fatalf("Haiku and Opus both cost $%.4f for the same cached turn — "+
			"an equal price means the model id never reached the registry", haiku)
	}

	// Prices are per million and scale linearly in them, so the expensive
	// model's whole bill is its price ratio times the cheap one's. Checking the
	// ratio rather than re-deriving the cache arithmetic keeps this test from
	// being a second copy of the function it is testing — a copy would agree
	// with a wrong implementation.
	const priceRatio = opusInputCostPer1M / haikuInputCostPer1M
	if opusOutputCostPer1M/haikuOutputCostPer1M != priceRatio {
		t.Fatalf("fixture no longer scales input and output by the same factor; the ratio check below is invalid")
	}
	if want := haiku * priceRatio; !closeEnough(opus, want) {
		t.Errorf("Opus cost $%.4f, want $%.4f — %.4gx Haiku, matching their registered prices",
			opus, want, priceRatio)
	}
}

// TestUnknownModelStillFallsBackLoudly is the complement. The fallback is not
// the bug — reaching it for models the registry knows was. It has to survive,
// or a model-store outage would bill everything at zero.
func TestUnknownModelStillFallsBackLoudly(t *testing.T) {
	store := registryWithTwoDifferentlyPricedModels(t)

	cost := CalcCost("a-model-nobody-registered", 1_000_000, 100_000, store)

	want := (1_000_000*3.00 + 100_000*15.00) / 1_000_000
	if cost != want {
		t.Errorf("unknown model cost $%.4f, want the $3.00/$15.00 fallback's $%.4f", cost, want)
	}
}

// TestNilRegistryPricesEverythingTheSame records what the store-less wrappers
// used to do at every call site, so the reason they were deleted stays visible.
// Nothing should call the package this way; if something must, this is the bill
// it gets.
func TestNilRegistryPricesEverythingTheSame(t *testing.T) {
	haiku := CalcCost("claude-haiku-4-5-20251001", 1_000_000, 100_000, nil)
	opus := CalcCost("claude-opus-4-20250514", 1_000_000, 100_000, nil)

	if haiku != opus {
		t.Errorf("with no registry the two models cost $%.4f and $%.4f; "+
			"there is nothing to tell them apart, so they must be equal", haiku, opus)
	}
}

// TestRebuiltTimelinePricesTheModelTheTurnRanOn covers the call site the
// finding did not name: `/api/sessions/{key}/timeline` rebuilds a session's
// history from its jsonl, and every turn in it was priced at the flat rate even
// though the log records the model each turn ran on. This is the one cost path
// a user reads directly.
func TestRebuiltTimelinePricesTheModelTheTurnRanOn(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "session.jsonl")
	writeSessionLogForOneTurn(t, logFile, "claude-haiku-4-5-20251001", 1_000_000, 100_000)

	events, _, err := ReconstructTimelineFromJSONL(logFile, registryWithTwoDifferentlyPricedModels(t))
	if err != nil {
		t.Fatalf("ReconstructTimelineFromJSONL: %v", err)
	}

	stats := statsEvent(t, events)
	flatRate := (1_000_000*3.00 + 100_000*15.00) / 1_000_000
	wantHaiku := (1_000_000*haikuInputCostPer1M + 100_000*haikuOutputCostPer1M) / 1_000_000

	if stats.Cost == flatRate {
		t.Fatalf("the Haiku turn is priced at $%.4f, the unknown-model flat rate — "+
			"the registry never saw the model id the log recorded", stats.Cost)
	}
	if stats.Cost != wantHaiku {
		t.Errorf("the Haiku turn is priced at $%.4f, want $%.4f from its registered $%.2f/$%.2f per 1M",
			stats.Cost, wantHaiku, haikuInputCostPer1M, haikuOutputCostPer1M)
	}
}

func writeSessionLogForOneTurn(t *testing.T, logFile, model string, inputTokens, outputTokens int) {
	t.Helper()

	file, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create log: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, entry := range []Entry{
		{
			Timestamp: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
			Turn:      1,
			Role:      "request",
			Request:   json.RawMessage(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`),
		},
		{
			Timestamp:    time.Date(2026, 7, 31, 12, 0, 1, 0, time.UTC),
			Turn:         1,
			Role:         "assistant",
			Content:      "hi",
			Model:        model,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	} {
		if err := encoder.Encode(entry); err != nil {
			t.Fatalf("write entry: %v", err)
		}
	}
}

func statsEvent(t *testing.T, events []TimelineEvent) TimelineEvent {
	t.Helper()
	for _, event := range events {
		if event.Type == "stats" {
			return event
		}
	}
	t.Fatalf("no stats event in the rebuilt timeline; got %d events", len(events))
	return TimelineEvent{}
}

func closeEnough(got, want float64) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-9
}
