package session

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
)

// The closing entry is the last thing a session log says about what it
// consumed, and it is the line a human reads when asking what a run cost. It
// reported `in=` and `out=` only.
//
// Those two names do not cover the prompt. The provider reports four disjoint
// counts and Input is the part of the prompt the cache served neither from nor
// into, so on a session that caches — which inber does deliberately — Input is
// a small fraction of what was sent. The cost on the same line has priced all
// four since commit 961f6dd, so the entry closed with a dollar figure worked
// out from the whole prompt sitting beside a token figure describing a
// twentieth of it.
//
// These tests assert differences rather than figures: adding cache traffic to
// an otherwise identical session must change what the closing entry reports.

// closingEntryAfterTurn returns a closed session's final JSONL entry after
// logging one assistant turn with the given counts.
//
// The model and registry are the straddling pair from turn_log_pricing_test.go,
// not a real model. This fixture used to run Sonnet against
// registryWithTwoDifferentlyPricedModels, and Sonnet is priced at exactly the
// unknown-model fallback of $3.00/$15.00 — so the registry argument computed the
// identical float as nil and pinned nothing at all. The straddling model is
// above the fallback on input and below it on output, so no figure that reached
// the fallback can coincide with the right answer.
func closingEntryAfterTurn(t *testing.T, tokens TurnTokens) Entry {
	t.Helper()

	session, err := New(t.TempDir(), straddlingModel, "test-agent", "",
		registryStraddlingTheFallback(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	session.LogRequest(json.RawMessage(`{"messages":[]}`))
	session.LogAssistant("reply", tokens, 0)

	path := session.FilePath()
	session.Close()

	return lastEntry(t, path)
}

// lastEntry reads back the final line of a session log.
func lastEntry(t *testing.T, path string) Entry {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open session log: %v", err)
	}
	defer file.Close()

	var last string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			last = line
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan session log: %v", err)
	}
	if last == "" {
		t.Fatal("session log is empty — the fixture wrote nothing")
	}

	var entry Entry
	if err := json.Unmarshal([]byte(last), &entry); err != nil {
		t.Fatalf("unmarshal closing entry %q: %v", last, err)
	}
	if entry.Role != "system" {
		t.Fatalf("last entry has role %q, want the system closing entry — "+
			"the fixture is not reading the line under test", entry.Role)
	}
	return entry
}

// TestClosingEntryRecordsCacheTraffic is the defect this file was written for.
// A session that read 900k tokens out of the cache must not close with the same
// structured record as one that read none.
func TestClosingEntryRecordsCacheTraffic(t *testing.T) {
	fresh := closingEntryAfterTurn(t, TurnTokens{Input: 1_000, Output: 500})
	cached := closingEntryAfterTurn(t, TurnTokens{
		Input: 1_000, Output: 500, CacheRead: 900_000, CacheWrite: 100_000,
	})

	if cached.CacheRead == fresh.CacheRead && cached.CacheWrite == fresh.CacheWrite {
		t.Fatalf("a session with 900k cache reads and 100k cache writes closed with "+
			"cache_read=%d cache_write=%d, the same as one with no cache traffic at all — "+
			"the counts are dropped at the closing entry",
			cached.CacheRead, cached.CacheWrite)
	}

	if cached.CacheRead != 900_000 {
		t.Errorf("cache_read_tokens = %d, want 900000", cached.CacheRead)
	}
	if cached.CacheWrite != 100_000 {
		t.Errorf("cache_write_tokens = %d, want 100000", cached.CacheWrite)
	}
}

// TestClosingLineNamesTheWholePrompt pins the human-readable half. The rendered
// text is the artifact a person actually reads, and a structured field being
// right does not help them if the sentence beside it still says 1,000.
func TestClosingLineNamesTheWholePrompt(t *testing.T) {
	entry := closingEntryAfterTurn(t, TurnTokens{
		Input: 1_000, Output: 500, CacheRead: 900_000, CacheWrite: 100_000,
	})

	const wholePrompt = "prompt=1001000"
	if !strings.Contains(entry.Content, wholePrompt) {
		t.Fatalf("closing line %q does not contain %q — it is still reporting a "+
			"prompt figure that leaves the cache traffic out", entry.Content, wholePrompt)
	}

	// The breakdown has to survive too: a total with no parts cannot be checked
	// against the provider's own four numbers.
	for _, part := range []string{"fresh=1000", "cache_read=900000", "cache_write=100000", "out=500"} {
		if !strings.Contains(entry.Content, part) {
			t.Errorf("closing line %q is missing %q", entry.Content, part)
		}
	}
}

// TestClosingEntryTokensAndCostDescribeTheSamePrompt is the contradiction that
// made this worth fixing rather than merely worth widening: the cost has priced
// the whole prompt since 961f6dd, so a reader dividing the reported cost by the
// reported tokens was getting a rate ~25x off. Both halves must move together.
func TestClosingEntryTokensAndCostDescribeTheSamePrompt(t *testing.T) {
	fresh := closingEntryAfterTurn(t, TurnTokens{Input: 1_000, Output: 500})
	cached := closingEntryAfterTurn(t, TurnTokens{
		Input: 1_000, Output: 500, CacheRead: 900_000, CacheWrite: 100_000,
	})

	if cached.TotalCost <= fresh.TotalCost {
		t.Fatalf("a session that moved 1M tokens through the cache cost $%.6f against "+
			"$%.6f for one that moved none — the pricing half has regressed, and this "+
			"test cannot say anything about the token half",
			cached.TotalCost, fresh.TotalCost)
	}

	freshPrompt := fresh.InputTokens + fresh.CacheRead + fresh.CacheWrite
	cachedPrompt := cached.InputTokens + cached.CacheRead + cached.CacheWrite
	if cachedPrompt <= freshPrompt {
		t.Fatalf("the cost rose from $%.6f to $%.6f but the recorded prompt stayed at "+
			"%d tokens against %d — the entry prices traffic it does not record",
			fresh.TotalCost, cached.TotalCost, freshPrompt, cachedPrompt)
	}
}

// TestClosingEntryPricesTheModelTheSessionRanOn pins the registry argument the
// three tests above merely carry. Each of them asserts a *difference*, and a
// difference survives the registry never being consulted: the fallback prices
// every model alike, so cached still costs more than fresh under a nil store.
// Measured 2026-08-17 — replacing the registry with nil in this file left the
// whole package green.
//
// A figure, not a difference, is what closes that. The straddling fixture is
// what makes the figure meaningful: its input price is above the fallback's and
// its output price below, so no one scale factor carries the registered price to
// the flat rate and a cost that reached the fallback is wrong in one direction
// on input and the other on output.
func TestClosingEntryPricesTheModelTheSessionRanOn(t *testing.T) {
	const inputTokens, outputTokens = 1_000, 500
	const cacheRead, cacheWrite = 900_000, 100_000

	entry := closingEntryAfterTurn(t, TurnTokens{
		Input: inputTokens, Output: outputTokens,
		CacheRead: cacheRead, CacheWrite: cacheWrite,
	})

	want := atTheRegisteredPrice(inputTokens, outputTokens, cacheRead, cacheWrite)
	if entry.TotalCost != want {
		t.Errorf("the closing entry reports $%.6f, want $%.6f from the registered "+
			"$%.2f/$%.2f per 1M", entry.TotalCost, want, straddlingInputCostPer1M, straddlingOutputCostPer1M)
	}
	if fallback := theFallbackRateFor(inputTokens, outputTokens, cacheRead, cacheWrite); entry.TotalCost == fallback {
		t.Errorf("the closing entry reports $%.6f, the unknown-model flat rate — the "+
			"session's registry never reached the price on the line a human reads", fallback)
	}
}

// TestClosingEntryReadsTokenTotalsUnderTheMutex is the race found beside the
// defect, and it is the same one SaveCheckpoint already carries a named test
// for. The closing entry took the mutex to price the session and then read the
// four counts outside it, so a turn still logging when a session closed could
// land in the cost and not in the counts.
//
// This is not a pricing test, so it passes no registry. What it needs of the
// cost is only that it be non-zero while the counts are still zero, and the
// unknown-model fallback supplies that. It used to hand a registry to a session
// running Sonnet, which the registry priced at exactly the fallback — an
// argument that read as coverage and pinned nothing.
func TestClosingEntryReadsTokenTotalsUnderTheMutex(t *testing.T) {
	session, err := New(t.TempDir(), "claude-sonnet-4-20250514", "test-agent", "", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(session.Close)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 200 {
			session.LogAssistant("reply", TurnTokens{
				Input: 10, Output: 5, CacheRead: 100, CacheWrite: 20,
			}, 0)
		}
	}()

	for range 200 {
		tokens, cost := session.closingTotals()
		if cost > 0 && tokens.Input == 0 && tokens.CacheRead == 0 {
			t.Errorf("closingTotals reported $%.6f against zero tokens — "+
				"the cost and the counts were not read together", cost)
			break
		}
	}
	wg.Wait()
}
