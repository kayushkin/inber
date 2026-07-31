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
// "Fresh" input = inTok - cacheRead - cacheWrite (the uncached portion).
func CalcCostWithCache(model string, inTok, outTok, cacheRead, cacheWrite int, store *modelstore.Store) float64 {
	info := agent.GetModelInfo(model, store)
	freshInput := inTok - cacheRead - cacheWrite
	if freshInput < 0 {
		freshInput = 0
	}
	inputCost := float64(freshInput) * info.InputCostPer1M
	cacheReadCost := float64(cacheRead) * info.InputCostPer1M * 0.1
	cacheWriteCost := float64(cacheWrite) * info.InputCostPer1M * 1.25
	outputCost := float64(outTok) * info.OutputCostPer1M
	return (inputCost + cacheReadCost + cacheWriteCost + outputCost) / 1_000_000
}
