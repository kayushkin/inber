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
	modelUsed, _ := e.selectModel()

	// Ensure we have the right client for the selected model
	if e.modelClient == nil || (e.modelClient.Model != nil && e.modelClient.Model.ID != modelUsed) {
		mc, mcErr := agent.NewModelClient(modelUsed, e.modelStore)
		if mcErr == nil {
			e.modelClient = mc
		}
	}
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

	// Record health regardless of success/failure
	e.recordModelHealth(modelUsed, time.Since(apiStart).Milliseconds(), err)

	return result, err
}
