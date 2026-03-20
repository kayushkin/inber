package session

import (
	"github.com/kayushkin/inber/agent"
	modelstore "github.com/kayushkin/model-store"
)

// CalcCost calculates cost from model and token counts (without cache adjustment).
// Uses model-store if available, falls back to defaults.
func CalcCost(model string, inTok, outTok int) float64 {
	return CalcCostWithStore(model, inTok, outTok, nil)
}

// CalcCostWithStore calculates cost using model-store if provided.
func CalcCostWithStore(model string, inTok, outTok int, store *modelstore.Store) float64 {
	info := agent.GetModelInfo(model, store)
	return (float64(inTok)*info.InputCostPer1M + float64(outTok)*info.OutputCostPer1M) / 1_000_000
}

// CalcCostWithCache calculates cost factoring in cache pricing.
// Cache reads cost 10% of normal input. Cache writes cost 125% of normal input.
// "Fresh" input = inTok - cacheRead - cacheWrite (the uncached portion).
func CalcCostWithCache(model string, inTok, outTok, cacheRead, cacheWrite int) float64 {
	return CalcCostWithCacheAndStore(model, inTok, outTok, cacheRead, cacheWrite, nil)
}

func CalcCostWithCacheAndStore(model string, inTok, outTok, cacheRead, cacheWrite int, store *modelstore.Store) float64 {
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