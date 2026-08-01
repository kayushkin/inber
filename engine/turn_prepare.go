package engine

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
)

// prepareInput handles user message stashing, alternation repair, and context management.
// Returns the processed input (post-stash).
//
// ctx is the turn's context. Preparation is not free local work: summarization
// makes its own API call, so a turn interrupted here has to stop like a turn
// interrupted anywhere else.
func (e *Engine) prepareInput(ctx context.Context, input, sessionID string) string {
	e.emitStatus("Preparing conversation...")
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

	// 1a. Repair dangling tool_use blocks (from interrupted turns, injections, or dedup)
	repaired, repairCount := conversation.RepairDanglingToolUse(e.Messages)
	if repairCount > 0 {
		Log.Warn("repaired %d dangling tool_use blocks before API call", repairCount)
		e.Messages = repaired
	}

	// 1b. Summarize if conversation is very long (compress old turns into summary).
	// Best effort: a failed summarization has already logged and left the conversation
	// whole, and losing a compaction is not a reason to fail the user's turn.
	_ = e.summarizeIfNeeded(ctx)
	// 1c. Prune remaining conversation (truncate tool results, old messages)
	e.emitStatus("Pruning context...")
	e.pruneIfNeeded(ctx)

	return processedInput
}

// buildTurnContext assembles system prompt blocks for the turn, and owns the
// lifetime of the turn's volatile context.
//
// When the memory store cannot answer, the turn falls back to the blocks the
// last turn was built from. That is the whole point of reusing them rather than
// assembling a replacement: the prefix stays byte-identical, so the degraded
// turn still hits the prompt cache instead of paying for the input twice. Its
// tag-matched memories are one message stale, which is a far smaller loss than
// the identity and instructions that come with them. The turn's volatile
// context is not reused — it describes this turn's fleet and files, and last
// turn's copy would be wrong rather than merely old.
//
// With no previous turn to fall back on there is nothing to degrade to, so the
// error is returned and the turn does not run.
func (e *Engine) buildTurnContext(processedInput string) ([]sessionMod.NamedBlock, error) {
	e.emitStatus("Building system prompt...")
	// The volatile context is per-turn: derive it fresh rather than inherit the
	// last turn's copy. BuildSystemPrompt assigns the field only when a memory
	// store is set, so without this a raw session accumulates every note it has
	// ever been given.
	e.Turn.VolatileContext = ""
	systemBlocks, err := e.BuildSystemPrompt(processedInput)
	if err != nil {
		if len(e.Cache.LastNamedBlocks) == 0 {
			return nil, err
		}
		Log.Warn("%v — reusing the system prompt from the previous turn", err)
		systemBlocks = e.Cache.LastNamedBlocks
	}
	// Every repository the session works in is named once per turn. It is
	// queued rather than written onto the field because BuildSystemPrompt
	// assigns that field wholesale, and it is queued here rather than during
	// preparation because it describes the session, not the conversation.
	e.queueVolatileNote(renderWorkspaceRoots(e.workspaceRoots))
	// BuildSystemPrompt assigns e.Turn.VolatileContext, so the notes queued
	// during preparation are folded in after it, never before.
	e.applyPendingVolatileNotes()
	e.Cache.LastNamedBlocks = systemBlocks
	return systemBlocks, nil
}
