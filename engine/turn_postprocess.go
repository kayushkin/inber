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

	// Track cumulative session tokens
	e.Tokens.Input += result.InputTokens
	e.Tokens.Output += result.OutputTokens
	e.Tokens.Cost += sessionMod.CalcCostWithCache(e.Model, result.InputTokens, result.OutputTokens,
		result.CacheReadTokens, result.CacheCreationTokens, e.modelStore)

	return nil
}
