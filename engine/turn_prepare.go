package engine

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
)

// prepareInput handles user message stashing, alternation repair, and context management.
// Returns the processed input (post-stash).
func (e *Engine) prepareInput(input, sessionID string) string {
	processedInput := input

	// 1. STASH LARGE USER MESSAGES (before sending to LLM)
	if e.stashCfg.Enabled && e.MemStore != nil {
		tokens := memory.EstimateTokens(input)
		if tokens > e.stashCfg.UserMessageThreshold {
			modifiedInput, stashed, err := conversation.DetectAndStashLargeBlocks(input, sessionID, e.MemStore, e.stashCfg)
			if err != nil {
				Log.Warn("failed to stash large user message: %v", err)
			} else if len(stashed) > 0 {
				processedInput = modifiedInput
				totalStashed := 0
				for _, s := range stashed {
					totalStashed += s.Tokens
				}
				Log.Info("stashed %d large blocks from user message (%d tokens)", len(stashed), totalStashed)

				if e.Session != nil {
					e.Session.LogStash("user", len(stashed), totalStashed)
				}
			}
		}
	}

	// Repair alternation before appending: if last message is also user role
	// (e.g. previous turn errored after appending user but before assistant response),
	// merge into the existing user message to maintain alternation.
	if len(e.Messages) > 0 && e.Messages[len(e.Messages)-1].Role == anthropic.MessageParamRoleUser {
		e.Messages[len(e.Messages)-1].Content = append(
			e.Messages[len(e.Messages)-1].Content,
			anthropic.NewTextBlock(processedInput),
		)
	} else {
		e.Messages = append(e.Messages, anthropic.NewUserMessage(anthropic.NewTextBlock(processedInput)))
	}

	// 1a. Summarize if conversation is very long (compress old turns into summary)
	e.summarizeIfNeeded()
	// 1b. Prune remaining conversation (truncate tool results, old messages)
	e.pruneIfNeeded()

	return processedInput
}

// buildTurnContext assembles system prompt blocks for the turn.
func (e *Engine) buildTurnContext(processedInput string) []sessionMod.NamedBlock {
	systemBlocks := e.BuildSystemPrompt(processedInput)
	e.Cache.LastNamedBlocks = systemBlocks
	return systemBlocks
}
