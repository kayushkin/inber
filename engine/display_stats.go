package engine

import (
	"fmt"
	"os"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/session"
)

// DisplayStats prints token usage and cost using the shared CalcCost from session package.
// Now also displays cumulative session stats if engine is provided.
func DisplayStats(result *agent.TurnResult, model string) {
	cost := session.CalcCost(model, result.InputTokens, result.OutputTokens)
	total := result.InputTokens + result.OutputTokens
	
	// Show prominent token summary
	fmt.Fprintf(os.Stderr, "\n%s┌─ Turn Tokens ─────────────────%s\n", dim, reset)
	fmt.Printf("%s│%s in=%s%d%s  out=%s%d%s  total=%s%d%s", 
		dim, reset,
		cyan, result.InputTokens, reset,
		cyan, result.OutputTokens, reset,
		bold+cyan, total, reset)
	
	if result.ToolCalls > 0 {
		fmt.Fprintf(os.Stderr, "  tools=%s%d%s", yellow, result.ToolCalls, reset)
	}
	
	if cost > 0 {
		fmt.Fprintf(os.Stderr, "  cost=%s$%.4f%s", green, cost, reset)
	}
	
	fmt.Fprintf(os.Stderr, "\n%s└───────────────────────────────%s\n", dim, reset)
}

// DisplaySessionStats prints cumulative session token usage and cost.
func DisplaySessionStats(turnNum, inputTokens, outputTokens int, cost float64) {
	total := inputTokens + outputTokens
	
	fmt.Fprintf(os.Stderr, "%s│ Session (turn %d): in=%d out=%d total=%s%d%s cost=%s$%.4f%s%s\n", 
		dim,
		turnNum,
		inputTokens,
		outputTokens,
		cyan, total, dim,
		green, cost, dim,
		reset)
}