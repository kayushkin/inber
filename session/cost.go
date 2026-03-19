package session

import (
	"github.com/kayushkin/inber/agent"
)

// cost calculates the total session cost in USD based on token usage.
func (s *Session) cost() float64 {
	info := agent.GetModelInfo(s.model, s.modelStore)
	return (float64(s.totalIn) * info.InputCostPer1M / 1_000_000) +
		(float64(s.totalOut) * info.OutputCostPer1M / 1_000_000)
}

// calculateTurnCost calculates the cost for a specific turn.
func (s *Session) calculateTurnCost(inTokens, outTokens int) float64 {
	info := agent.GetModelInfo(s.model, s.modelStore)
	return (float64(inTokens)*info.InputCostPer1M + float64(outTokens)*info.OutputCostPer1M) / 1_000_000
}