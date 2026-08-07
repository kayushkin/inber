package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/kayushkin/inber/agent"
)

// The OpenAI turn loop kept running totals and no per-call record, so a turn of
// one expensive call and a turn of ten cheap ones were the same two numbers,
// and reportCallsThatBoughtNoCache — the one diagnostic that reads per-call
// usage — had nothing to walk. The Anthropic path has kept the record since
// agent/agent_run.go processResponse; this closes the gap on the second loop.

func usageResponse(text string, prompt, completion, cached int) agent.OpenAIResponse {
	resp := textResponse(text)
	resp.Usage = agent.OpenAIUsage{
		PromptTokens:        prompt,
		CompletionTokens:    completion,
		TotalTokens:         prompt + completion,
		PromptTokensDetails: agent.OpenAIPromptTokensDetails{CachedTokens: cached},
	}
	return resp
}

// One record per API call, in call order, and the turn totals stay exactly
// their sum. The second half matters as much as the first: a per-call record
// that shifted the totals would be a billing change wearing an observability
// change's clothes.
func TestAnOpenAIServedTurnRecordsUsageForEveryAPICall(t *testing.T) {
	var seen []string
	toolCall := toolCallResponse("call_1", "read", `{"path":"a.txt"}`)
	toolCall.Usage = agent.OpenAIUsage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110}
	f := newFakeOpenAI(t, toolCall, usageResponse("done", 300, 20, 0))
	e := openAIEngine(t, f, recordingTool("read", "contents", &seen))

	result, err := e.runOpenAITurn(context.Background(), nil)
	if err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}

	if len(result.APICalls) != 2 {
		t.Fatalf("%d per-call records, want 2 — this turn made two API calls", len(result.APICalls))
	}
	if result.APICalls[0].InputTokens != 100 || result.APICalls[1].InputTokens != 300 {
		t.Errorf("per-call input tokens %d and %d, want 100 then 300 in call order",
			result.APICalls[0].InputTokens, result.APICalls[1].InputTokens)
	}

	var inputs, outputs int
	for _, call := range result.APICalls {
		inputs += call.InputTokens
		outputs += call.OutputTokens
	}
	if result.InputTokens != inputs || result.OutputTokens != outputs {
		t.Errorf("turn totals (%d in / %d out) are not the sum of the records (%d in / %d out) — "+
			"the records must be the same numbers before they were added up, not a second count",
			result.InputTokens, result.OutputTokens, inputs, outputs)
	}
}

// The number the whole change exists for reaches the result, and the Anthropic
// cache counters stay untouched while it does.
//
// Both halves are the assertion. Carrying the count is what makes this path's
// cache behaviour measurable at all; leaving CacheReadTokens alone is what keeps
// it from being priced under the wrong convention, since CalcCostWithCache
// treats that field as disjoint from InputTokens and OpenAI's cached_tokens is
// a subset of it. See todo 0d052752.
func TestAnOpenAIServedTurnCarriesTheProvidersCachedCountWithoutPricingIt(t *testing.T) {
	f := newFakeOpenAI(t, usageResponse("done", 1000, 20, 800))
	e := openAIEngine(t, f)

	result, err := e.runOpenAITurn(context.Background(), nil)
	if err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}

	if len(result.APICalls) != 1 {
		t.Fatalf("%d per-call records, want 1", len(result.APICalls))
	}
	if got := result.APICalls[0].CachedTokensIncludedInInputTokens; got != 800 {
		t.Errorf("cached tokens on the record %d, want the 800 the provider reported — "+
			"without it there is no number on this path that could say whether a cache change did anything", got)
	}
	if result.APICalls[0].InputTokens != 1000 {
		t.Errorf("input tokens %d, want the provider's unadjusted 1000", result.APICalls[0].InputTokens)
	}
	if result.CacheReadTokens != 0 || result.CacheCreationTokens != 0 {
		t.Errorf("Anthropic cache counters were set (read=%d write=%d) — that prices the cached span, "+
			"which is the open half of todo 0d052752", result.CacheReadTokens, result.CacheCreationTokens)
	}
}

// The link from the record to something an operator can read. Without it the
// count is a field nobody looks at, which is the same as never having carried
// it — and the line has to say the cost is an overstatement, because a wrong
// number that admits it is wrong is the most an unpriced measurement can offer.
func TestATurnTheProviderServedFromCacheSaysSoAndSaysTheCostIsOverstated(t *testing.T) {
	e := &Engine{Model: "gpt-4.1"}
	result := &agent.TurnResult{
		InputTokens:  1000,
		OutputTokens: 20,
		APICalls: []agent.APICallUsage{
			{InputTokens: 1000, OutputTokens: 20, CachedTokensIncludedInInputTokens: 800},
		},
	}

	logged := captureStderr(t, func() { e.recordTurnUsage(result) })

	for _, want := range []string{"800", "1000", "80%", "0d052752"} {
		if !strings.Contains(logged, want) {
			t.Errorf("the report does not mention %q — an operator cannot size the cache from it.\ngot: %s", want, logged)
		}
	}
}

// Silent when the provider reported no cached tokens, which is every turn on
// the Anthropic path. A diagnostic that printed a zero on every turn would be
// noise on the path this change is not about.
func TestATurnWithNoProviderCacheAddsNoLine(t *testing.T) {
	e := &Engine{Model: "claude-sonnet-4-5-20250929"}
	result := &agent.TurnResult{
		InputTokens:     1000,
		OutputTokens:    20,
		CacheReadTokens: 900,
		APICalls: []agent.APICallUsage{
			{InputTokens: 1000, OutputTokens: 20, CacheReadTokens: 900},
		},
	}

	logged := captureStderr(t, func() { e.recordTurnUsage(result) })

	if strings.Contains(logged, "from its own cache") {
		t.Errorf("the provider-cache line fired on a turn that reported none.\ngot: %s", logged)
	}
}
