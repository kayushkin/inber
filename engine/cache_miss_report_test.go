package engine

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/kayushkin/inber/agent"
)

// captureStderr runs fn with os.Stderr replaced by a pipe and returns what was
// written. Log.Info writes to os.Stderr directly, which is the surface being
// asserted: the point of the report is that an operator can read it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = write

	done := make(chan string, 1)
	go func() {
		out, _ := io.ReadAll(read)
		done <- string(out)
	}()

	fn()

	os.Stderr = original
	write.Close()
	captured := <-done
	read.Close()
	return captured
}

// TestATurnWithACacheMissingCallSaysSo covers the link from the per-call record
// to something an operator can read.
//
// recordTurnUsage prices the turn as one number and adds it to the session and
// guard totals. That number cannot answer the question this reporting exists
// for — a call that re-bought the whole prompt is averaged in with the cheap
// ones and vanishes — so the withheld-tools calls are priced again on their own
// and named. Without this link the per-call record is a field nobody reads.
func TestATurnWithACacheMissingCallSaysSo(t *testing.T) {
	e := &Engine{Model: "claude-sonnet-4-5-20250929"}
	result := &agent.TurnResult{
		InputTokens:         200010,
		OutputTokens:        60,
		CacheReadTokens:     100,
		CacheCreationTokens: 200000,
		APICalls: []agent.APICallUsage{
			{InputTokens: 10, OutputTokens: 10, CacheReadTokens: 100},
			{InputTokens: 200000, OutputTokens: 50, CacheCreationTokens: 200000, ToolsWithheld: true},
		},
	}

	logged := captureStderr(t, func() { e.recordTurnUsage(result) })

	if !strings.Contains(logged, "sent no tools block") {
		t.Fatalf("a turn containing a call that matched no cached prefix reported nothing.\n"+
			"Nobody can size the force-summary cost from the turn total alone, which is the whole "+
			"reason the per-call record exists.\nstderr was: %q", logged)
	}
	if !strings.Contains(logged, "200000 cache write") {
		t.Errorf("the report does not carry the call's own cache-write count, so it says a miss "+
			"happened without saying how big it was.\nstderr was: %q", logged)
	}
	if !strings.Contains(logged, "API call 2 of 2") {
		t.Errorf("the report does not say which call of the turn it was.\nstderr was: %q", logged)
	}
}

// TestAnOrdinaryTurnReportsNothing is the complement, and it is what keeps the
// line worth reading. Almost every turn is entirely cached calls; a report that
// also fired on those would be noise in the journal of every session, and the
// one line that matters would be unfindable.
func TestAnOrdinaryTurnReportsNothing(t *testing.T) {
	e := &Engine{Model: "claude-sonnet-4-5-20250929"}
	result := &agent.TurnResult{
		InputTokens:     30,
		OutputTokens:    3,
		CacheReadTokens: 300,
		APICalls: []agent.APICallUsage{
			{InputTokens: 10, OutputTokens: 1, CacheReadTokens: 100},
			{InputTokens: 20, OutputTokens: 2, CacheReadTokens: 200},
		},
	}

	logged := captureStderr(t, func() { e.recordTurnUsage(result) })

	if strings.Contains(logged, "sent no tools block") {
		t.Fatalf("a turn whose calls all carried their tools was reported as missing the cache.\n"+
			"stderr was: %q", logged)
	}
}

// TestTheTurnIsStillPricedAndCharged pins that adding the report did not
// disturb what recordTurnUsage was already for. The session's running total and
// the guard's cost total are the numbers a spend cap is enforced against.
func TestTheTurnIsStillPricedAndCharged(t *testing.T) {
	e := &Engine{Model: "claude-sonnet-4-5-20250929"}
	result := &agent.TurnResult{
		InputTokens:  1000,
		OutputTokens: 100,
		APICalls: []agent.APICallUsage{
			{InputTokens: 1000, OutputTokens: 100, ToolsWithheld: true},
		},
	}

	captureStderr(t, func() { e.recordTurnUsage(result) })

	if e.Tokens.Input != 1000 || e.Tokens.Output != 100 {
		t.Errorf("the turn's tokens did not reach the session total: got %d in / %d out, want 1000 and 100",
			e.Tokens.Input, e.Tokens.Output)
	}
	if e.Tokens.Cost <= 0 {
		t.Errorf("the turn was priced at %v; a turn with tokens costs money and that figure is what "+
			"MaxCost is compared against", e.Tokens.Cost)
	}
}
