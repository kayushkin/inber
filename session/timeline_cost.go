package session

import (
	"github.com/kayushkin/inber/agent"
	modelstore "github.com/kayushkin/model-store"
)

// Both functions here take the model registry rather than defaulting it,
// because there is no honest answer without one. agent.GetModelInfo can only
// meet a nil store with the unknown-model flat rate of $3.00/$15.00 per million
// tokens, so a convenience wrapper that supplied nil on the caller's behalf did
// not compute a cheaper cost — it computed the wrong one, silently, for every
// model. Two such wrappers existed and every live server caller reached for
// one, which is why a Haiku sub-agent was billed at twelve times its registered
// price and an Opus one at a fifth of its own. Passing the store is the whole
// point of calling these; a caller that has no store to pass should say so out
// loud by passing nil, not by picking an overload that hides it.

// CalcCost calculates cost from the model that ran and its token counts,
// without cache adjustment.
func CalcCost(model string, inTok, outTok int, store *modelstore.Store) float64 {
	info := agent.GetModelInfo(model, store)
	return (float64(inTok)*info.InputCostPer1M + float64(outTok)*info.OutputCostPer1M) / 1_000_000
}

// CalcCostWithCache calculates cost factoring in cache pricing.
// Cache reads cost 10% of normal input. Cache writes cost 125% of normal input.
//
// The three input counts are disjoint. Anthropic reports `input_tokens` as the
// portion of the prompt that was neither read from the cache nor written to it,
// and reports `cache_read_input_tokens` and `cache_creation_input_tokens`
// alongside it; the whole prompt is their sum. This function used to subtract
// the two cache figures from inTok to recover a "fresh" remainder, which
// charges the uncached input at zero whenever the cached part of the prompt is
// larger than the uncached part — which is what caching is for, so it is the
// normal case rather than an edge one.
//
// It was not an edge case here either. Across the 167 recorded requests in the
// server's own store that carry cache traffic, inTok was smaller than
// cacheRead+cacheWrite in 167 of them, so the subtraction went negative every
// single time and the clamp below turned it into a zero input charge: 781,355
// genuinely fresh input tokens priced at nothing. A row reading input=18 with
// cache_creation=16,172 cannot be a total that contains its own cache figure.
//
// Everything inber says about money runs through here — the engine's per-turn
// charge and therefore the MaxCost cap, the request rows, the spawn path, and
// the TotalUSD reported over the bridge — so the understatement was in all of
// them at once.
func CalcCostWithCache(model string, inTok, outTok, cacheRead, cacheWrite int, store *modelstore.Store) float64 {
	info := agent.GetModelInfo(model, store)
	inputCost := float64(inTok) * info.InputCostPer1M
	cacheReadCost := float64(cacheRead) * info.InputCostPer1M * 0.1
	cacheWriteCost := float64(cacheWrite) * info.InputCostPer1M * 1.25
	outputCost := float64(outTok) * info.OutputCostPer1M
	return (inputCost + cacheReadCost + cacheWriteCost + outputCost) / 1_000_000
}
