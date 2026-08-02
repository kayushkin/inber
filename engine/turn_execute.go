package engine

import (
	"context"
	"time"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/internal/apiutil"
	sessionMod "github.com/kayushkin/inber/session"
)

// executeAgent runs the agent with the built system prompt and conversation messages.
// Handles model selection, client setup, OpenAI vs Anthropic routing, and thinking signature errors.
func (e *Engine) executeAgent(ctx context.Context, systemBlocks []sessionMod.NamedBlock) (*agent.TurnResult, error) {
	e.emitStatus("Selecting model...")
	// Select model based on health data (failover if primary is down)
	selected, _ := e.selectModel()

	// Install the client for it, and take back the model actually in force —
	// which differs from `selected` when no client could be built for it.
	modelUsed := e.resolveModelClient(selected)
	e.Model = modelUsed

	var result *agent.TurnResult
	var err error
	apiStart := time.Now()

	if e.modelClient != nil && e.modelClient.IsOpenAI() {
		result, err = e.runOpenAITurn(ctx, systemBlocks)
	} else {
		// Filter out OpenAI-sourced tool_use/tool_result pairs for Anthropic
		originalLen := len(e.Messages)
		var stats agent.FilterStats
		e.Messages, stats = agent.FilterMessagesForAnthropic(e.Messages)
		if stats.ToolUseFiltered > 0 || stats.ToolResultFiltered > 0 {
			Log.Info("filtered %d tool_use, %d tool_result blocks from OpenAI provider (%d→%d messages)",
				stats.ToolUseFiltered, stats.ToolResultFiltered, originalLen, len(e.Messages))
		}
		e.emitStatus("Running agent...")
		e.buildAgent(systemBlocks)
		result, err = e.Agent.Run(ctx, e.Model, &e.Messages)

		// If we hit a thinking signature error, strip thinking blocks and retry once
		if err != nil && apiutil.IsThinkingSignatureError(err) {
			Log.Warn("API returned generic 'Error', likely stale thinking signatures — stripping and retrying")
			e.Messages = conversation.RepairThinkingSignatures(e.Messages)
			e.buildAgent(systemBlocks) // Rebuild agent with repaired messages
			result, err = e.Agent.Run(ctx, e.Model, &e.Messages)
		}
	}

	// Record health for outcomes that are evidence about the model. A turn that
	// inber itself stopped — a cancel, one of its own policy deadlines, its own
	// API-call cap — leaves the record alone. ctx is passed because that is how
	// the deadline case tells inber's own clock from the OpenAI client's flat
	// 120s timeout, which is a provider not answering; see recordModelHealth.
	e.recordModelHealth(ctx, modelUsed, time.Since(apiStart).Milliseconds(), err)

	return result, err
}
