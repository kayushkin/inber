package engine

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/memory"
)

// stashConfigForTurn is the stash configuration with the one field the engine
// owns filled in: the tools this turn is actually putting on the wire.
//
// It is read per turn rather than stored on the engine because the wire set is
// not fixed for a session's life — SetDisabledTools can take memory_expand away
// between one turn and the next, and a config captured at construction would go
// on promising a recall the model can no longer perform. summarizeIfNeeded reads
// the same set at the same moment for the same reason.
func (e *Engine) stashConfigForTurn() conversation.StashConfig {
	cfg := e.stashCfg
	cfg.RecallToolNames = e.EnabledToolNames()
	return cfg
}

// stashAssistantResponse processes the last assistant message and stashes large blocks
// to reduce context size if stashing is enabled and thresholds are met.
func (e *Engine) stashAssistantResponse(sessionID string, result *agent.TurnResult) {
	if !e.stashCfg.Enabled || e.MemStore == nil {
		return
	}
	responseTokens := memory.EstimateTokens(result.Text)
	if responseTokens <= e.stashCfg.AssistantThreshold {
		return
	}
	if len(e.Messages) == 0 || e.Messages[len(e.Messages)-1].Role != anthropic.MessageParamRoleAssistant {
		return
	}

	stashCfg := e.stashConfigForTurn()

	lastMsg := &e.Messages[len(e.Messages)-1]
	var modifiedContent []anthropic.ContentBlockParamUnion
	stashedAny := false

	for _, block := range lastMsg.Content {
		if block.OfText != nil {
			text := block.OfText.Text
			textTokens := memory.EstimateTokens(text)

			if textTokens > stashCfg.MinBlockSize {
				modifiedText, stashed, err := conversation.DetectAndStashLargeBlocks(text, sessionID, e.MemStore, stashCfg)
				if err != nil {
					Log.Warn("failed to stash assistant response: %v", err)
					modifiedContent = append(modifiedContent, block)
				} else if len(stashed) > 0 {
					stashedAny = true
					modifiedContent = append(modifiedContent, anthropic.ContentBlockParamUnion{
						OfText: &anthropic.TextBlockParam{Text: modifiedText},
					})
					totalStashed := 0
					for _, s := range stashed {
						totalStashed += s.Tokens
					}
					Log.Info("stashed %d large blocks from assistant response (%d tokens)", len(stashed), totalStashed)
					if e.Session != nil {
						e.Session.LogStash("assistant", len(stashed), totalStashed)
					}
				} else {
					modifiedContent = append(modifiedContent, block)
				}
			} else {
				modifiedContent = append(modifiedContent, block)
			}
		} else {
			modifiedContent = append(modifiedContent, block)
		}
	}

	if stashedAny {
		lastMsg.Content = modifiedContent
	}
}
