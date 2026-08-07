package agent

import (
	"encoding/json"
	"testing"
)

// An OpenAI-compatible provider reports the cached part of a prompt in exactly
// one place — prompt_tokens_details.cached_tokens — and OpenAIUsage used to
// have no home for it, so it was dropped where the response was unmarshalled.
// Nothing downstream could see it, which made this path's cache behaviour
// unmeasurable rather than merely unmeasured.
//
// These tests are written against literal response bodies rather than a
// round-tripped Go value on purpose: a typed fake proves the struct holds the
// number, not that the JSON tag matches what a provider actually sends. The tag
// is the whole defect, so the tag is what is pinned.

const cachingProviderResponse = `{
	"id": "chatcmpl-1",
	"choices": [{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
	"usage": {
		"prompt_tokens": 1000,
		"completion_tokens": 20,
		"total_tokens": 1020,
		"prompt_tokens_details": {"cached_tokens": 800}
	}
}`

func TestTheCachedTokenCountSurvivesTheJSONBoundary(t *testing.T) {
	var resp OpenAIResponse
	if err := json.Unmarshal([]byte(cachingProviderResponse), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Usage.PromptTokens != 1000 {
		t.Fatalf("prompt tokens %d, want 1000 — the fixture is not being read", resp.Usage.PromptTokens)
	}
	if got := resp.Usage.PromptTokensDetails.CachedTokens; got != 800 {
		t.Errorf("cached tokens %d, want the 800 the provider reported; the json tag has to be "+
			"prompt_tokens_details.cached_tokens or the number is dropped again", got)
	}
}

// A provider that does not cache omits the details object entirely. Reading it
// must be a plain zero rather than an unmarshal failure, because the alternative
// is that adding this field turns every non-caching provider's response into a
// turn-ending error.
func TestAProviderThatSendsNoDetailsBlockReportsZeroRatherThanFailing(t *testing.T) {
	body := `{
		"id": "chatcmpl-2",
		"choices": [{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
		"usage": {"prompt_tokens": 1000, "completion_tokens": 20, "total_tokens": 1020}
	}`

	var resp OpenAIResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal a usage block with no details: %v", err)
	}
	if got := resp.Usage.PromptTokensDetails.CachedTokens; got != 0 {
		t.Errorf("cached tokens %d, want 0 when the provider said nothing about caching", got)
	}
}

// ⚠️ This test pins a NON-decision, and it is the one assertion here worth not
// deleting.
//
// OpenAI's prompt_tokens INCLUDES the cached span; Anthropic's input_tokens
// EXCLUDES it and counts it again in cache_read_input_tokens, which is what
// CalcCostWithCache prices separately. So mapping cached_tokens onto
// CacheReadInputTokens while leaving InputTokens at the full prompt_tokens
// charges the cached span twice — the same class of error as the one recorded
// at session/timeline_cost.go:30-43 and closed as todo 00093e48, sign flipped.
//
// Where that normalisation belongs is the open half of todo 0d052752 and it is
// the owner's call. Until it is answered the conversion says nothing about
// caching at all, and this asserts that silence: making the number visible was
// not permission to price it.
func TestConvertingAResponseDoesNotClaimTheCachedSpanIsAnthropicCacheRead(t *testing.T) {
	var resp OpenAIResponse
	if err := json.Unmarshal([]byte(cachingProviderResponse), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	converted := ConvertOpenAIResponseToAnthropic(&resp)

	if converted.Usage.CacheReadInputTokens != 0 || converted.Usage.CacheCreationInputTokens != 0 {
		t.Fatalf("the conversion set Anthropic cache counters (read=%d write=%d) — that decides how the "+
			"cached span is priced, which todo 0d052752 leaves to the owner",
			converted.Usage.CacheReadInputTokens, converted.Usage.CacheCreationInputTokens)
	}
	if converted.Usage.InputTokens != 1000 {
		t.Errorf("input tokens %d, want the provider's unadjusted 1000 — subtracting the cached span here "+
			"is one of the two options that decision is between, not a free cleanup", converted.Usage.InputTokens)
	}
}
