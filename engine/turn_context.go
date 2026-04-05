package engine

import (
	"github.com/kayushkin/inber/memory"
)

// contextBudget returns the token budget for memory context loading.
func (e *Engine) contextBudget(userMessage string) (minImportance float64, tokenBudget int) {
	msgTokens := memory.EstimateTokens(userMessage)

	// Use larger context budget for error recovery
	if e.Turn.ConsecutiveErrors >= 5 {
		return 0, 50000
	}
	if e.Turn.ConsecutiveErrors >= 3 {
		return 0, 35000
	}
	if e.Turn.ConsecutiveErrors >= 1 || e.Turn.LastHadError {
		return 0, 20000
	}

	// First turn gets base budget
	if e.Turn.Counter == 0 {
		return 0, 4000
	}

	// Scale budget based on message complexity
	if msgTokens > 1000 {
		return 0, 15000
	}
	if msgTokens > 300 {
		return 0, 10000
	}
	if e.Turn.Counter > 15 {
		return 0, 8000
	}

	return 0, 6000
}
