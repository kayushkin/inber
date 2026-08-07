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
	e.reportCacheTheProviderServedButNobodyPriced(result)
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

// reportCacheTheProviderServedButNobodyPriced says how much of this turn's
// prompt an OpenAI-compatible provider served from its own cache, and states
// that the turn's dollar figure does not reflect it.
//
// The number is new. It arrives as prompt_tokens_details.cached_tokens and used
// to be dropped at the JSON boundary, so there was no figure on this path that
// could say whether a cache change did anything — which is what blocks the
// volatile-context placement question in todo ec9c7122, and what makes the
// pricing half of 0d052752 undecidable rather than merely undecided.
//
// It is reported and not priced, deliberately. OpenAI's prompt_tokens INCLUDES
// the cached span where Anthropic's input_tokens excludes it, so pricing this
// beside an unadjusted input count double-charges; see
// agent.APICallUsage.CachedTokensIncludedInInputTokens. Saying the overstatement
// out loud is the honest thing an unpriced measurement can do — the alternative
// is a cost line that is wrong and silent about it.
//
// Silent when the provider reported no cached tokens, which is every turn on
// the Anthropic path, so this adds nothing to the output there.
func (e *Engine) reportCacheTheProviderServedButNobodyPriced(result *agent.TurnResult) {
	var cached, input int
	for _, call := range result.APICalls {
		cached += call.CachedTokensIncludedInInputTokens
		input += call.InputTokens
	}
	if cached == 0 {
		return
	}
	share := 0.0
	if input > 0 {
		share = 100 * float64(cached) / float64(input)
	}
	Log.Info("cache: the provider served %d of %d prompt tokens from its own cache (%.0f%%) across %d API call(s) — "+
		"this turn's cost prices all %d at the full input rate, so it is an overstatement (todo 0d052752)",
		cached, input, share, len(result.APICalls), input)
}
