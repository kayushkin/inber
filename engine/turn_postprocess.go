package engine

import (
	"context"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
	sessionMod "github.com/kayushkin/inber/session"
)

// turnTokens carries a turn's four usage counts from the agent's result into
// the session log in one piece. Both call sites used to hand over the input and
// output counts and leave the cache counts behind, which is how the session's
// own cost line came to price a twentieth of the prompt.
func turnTokens(result *agent.TurnResult) sessionMod.TurnTokens {
	return sessionMod.TurnTokens{
		Input:      result.InputTokens,
		Output:     result.OutputTokens,
		CacheRead:  result.CacheReadTokens,
		CacheWrite: result.CacheCreationTokens,
	}
}

// postProcessResult handles background memory extraction, response stashing, session save,
// checkpointing, and usage tracking after a successful turn.
func (e *Engine) postProcessResult(result *agent.TurnResult, input, sessionID string) error {
	// Log assistant response to session
	if e.Session != nil {
		e.Session.LogAssistant(result.Text, turnTokens(result), result.ToolCalls)
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

	e.reportCallsThatBoughtNoCache(result)
}

// reportCallsThatBoughtNoCache prices, on its own, any call in the turn that
// went out without the tools block, and says so on stderr.
//
// The turn total above cannot show this. It sums every call, so one call that
// re-bought the whole prompt at the 1.25x write rate is averaged in with the
// cheap cached ones around it and disappears — which is why inber has been
// unable to say how often this happens or what it costs, despite the counters
// being right there.
//
// The line is a measurement, not a complaint: whether a guaranteed-prose
// summary is worth buying with the tools block is an open question (todo
// 8754300f), and it should be answered against numbers from this host rather
// than an estimate. Priced with the same function as the turn, so the two
// figures are comparable and the fraction is meaningful.
func (e *Engine) reportCallsThatBoughtNoCache(result *agent.TurnResult) {
	for i, call := range result.APICalls {
		if !call.ToolsWithheld {
			continue
		}
		cost := sessionMod.CalcCostWithCache(e.Model, call.InputTokens, call.OutputTokens,
			call.CacheReadTokens, call.CacheCreationTokens, e.modelStore)
		Log.Info("cache: API call %d of %d sent no tools block, so it matched no cached prefix — "+
			"%d in / %d out, %d cache write, %d cache read, $%.4f",
			i+1, len(result.APICalls), call.InputTokens, call.OutputTokens,
			call.CacheCreationTokens, call.CacheReadTokens, cost)
	}
}
