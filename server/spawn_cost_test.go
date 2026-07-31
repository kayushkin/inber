package server

import (
	"strings"
	"testing"
)

// theFlatRateFor is what a cost calculation lands on when it is handed no model
// id and no registry: the unknown-model fallback of $3.00/$15.00 per million.
// The tests below use it as the wrong answer to assert against, so a regression
// that reintroduces the flat rate fails loudly rather than merely differing.
func theFlatRateFor(inputTokens, outputTokens int) float64 {
	return (float64(inputTokens)*3.00 + float64(outputTokens)*15.00) / 1_000_000
}

// TestParentIsToldWhatTheChildActuallyCost pins the delivery half of the flat-rate
// defect. deliverResult used to recompute the cost from the tokens alone, with
// an empty model id and a nil registry, so the figure the parent orchestrator
// read was the $3/$15 fallback no matter which model the child ran on — and it
// could disagree with the cost written to the request row for the same spawn.
// It now reports the number the spawn already worked out.
func TestParentIsToldWhatTheChildActuallyCost(t *testing.T) {
	parent := &Session{Key: "parent", Status: Running, injections: make(chan string, 1)}
	server := &Server{}
	server.sessions.Store("parent", parent)

	const inputTokens, outputTokens = 1_000_000, 100_000
	const childCost = 1.2 // Haiku's registered $0.80/$4.00 on those tokens.

	server.deliverResult("parent", SpawnResult{
		ChildKey: "child",
		Agent:    "brigid",
		Task:     "read the repo",
		Status:   "success",
		Summary:  "done",
		Model:    "claude-haiku-4-5-20251001",
		Tokens: TokenUsage{
			Input:  inputTokens,
			Output: outputTokens,
			Cost:   childCost,
		},
	})

	var message string
	select {
	case message = <-parent.injections:
	default:
		t.Fatal("nothing was injected into the running parent")
	}

	if !strings.Contains(message, "$1.200") {
		t.Errorf("the parent was told %q, which does not carry the child's $%.3f", message, childCost)
	}
	flatRate := theFlatRateFor(inputTokens, outputTokens)
	if strings.Contains(message, "4.500") {
		t.Errorf("the parent was told $%.3f, the unknown-model flat rate — "+
			"the cost was recomputed from the tokens instead of taken from the result", flatRate)
	}
}

// TestParentIsNotToldACostTheSpawnNeverIncurred is the complement. A spawn that
// failed before it ran has no tokens and no cost, and must not be reported as
// having one — an assertion that only checked "some dollar figure appears"
// would pass on a fabricated number.
func TestParentIsNotToldACostTheSpawnNeverIncurred(t *testing.T) {
	parent := &Session{Key: "parent", Status: Running, injections: make(chan string, 1)}
	server := &Server{}
	server.sessions.Store("parent", parent)

	server.deliverResult("parent", SpawnResult{
		ChildKey: "child",
		Agent:    "brigid",
		Task:     "read the repo",
		Status:   "error",
		Error:    "enqueue failed",
	})

	var message string
	select {
	case message = <-parent.injections:
	default:
		t.Fatal("nothing was injected into the running parent")
	}

	if !strings.Contains(message, "$0.000") {
		t.Errorf("a spawn that never ran was reported as %q, want $0.000", message)
	}
}
