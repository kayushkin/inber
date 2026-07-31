package engine

import (
	"context"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
	sessionMod "github.com/kayushkin/inber/session"
)

// postProcessResult handles background memory extraction, response stashing, session save,
// checkpointing, and usage tracking after a successful turn.
func (e *Engine) postProcessResult(result *agent.TurnResult, input, sessionID string) error {
	// Log assistant response to session
	if e.Session != nil {
		e.Session.LogAssistant(result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)
	}

	// 2. BACKGROUND MEMORY EXTRACTION (after turn completes, async)
	if e.extractCfg.Enabled && e.MemStore != nil {
		var toolCalls []conversation.ToolCallSummary
		go conversation.BackgroundExtractMemories(
			context.Background(),
			e.Client,
			input,
			result.Text,
			toolCalls,
			sessionID,
			e.MemStore,
			e.extractCfg,
		)
	}

	// 3. STASH LARGE ASSISTANT RESPONSES (for next turn)
	e.stashAssistantResponse(sessionID, result)

	// Save the messages snapshot and turn count for session resume
	e.emitStatus("Saving session...")
	e.saveResumableState()

	// Checkpoint if needed (every 20 turns)
	e.checkpointIfNeeded()

	e.recordTurnUsage(result)

	return nil
}

// recordTurnUsage adds one turn's tokens and cost to every running total that
// tracks them: the session's, which is what gets reported, and the guard's,
// which is what enforces MaxCost.
//
// Both totals are the same money, so the turn is priced once and the single
// figure is added to both. Pricing it again at the guard would let the number a
// cap is enforced against drift from the number the session reports for the
// same turn.
//
// Guard.RecordCost had no caller at all before this, so the guard's cost total
// sat at zero for the life of every session and its MaxCost comparison could
// never be true however much a session spent.
//
// This runs here rather than beside Guard.RecordTurn in RunTurn's record step
// because a turn that fails part-way through still reaches this function, and a
// failed turn still cost money. Charging only the turns that finished would let
// a session that keeps erroring run past its cap without limit.
func (e *Engine) recordTurnUsage(result *agent.TurnResult) {
	e.Tokens.Input += result.InputTokens
	e.Tokens.Output += result.OutputTokens

	turnCost := sessionMod.CalcCostWithCache(e.Model, result.InputTokens, result.OutputTokens,
		result.CacheReadTokens, result.CacheCreationTokens, e.modelStore)
	e.Tokens.Cost += turnCost
	if e.Guard != nil {
		e.Guard.RecordCost(turnCost)
	}
}
