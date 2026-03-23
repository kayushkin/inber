package conversation

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"
)

// PruneResult contains statistics about what was pruned
type PruneResult struct {
	OriginalMessages   int
	PrunedMessages     int
	TokensFreed        int
	MemoriesSaved      int
	Strategy           string
	TruncatedToolCalls int
	TruncatedAssistant int
	DroppedToolResults int
}

// PruneConversation intelligently prunes conversation history while preserving important information.
// It applies role-specific pruning strategies based on message type and age.
// Before pruning, it auto-saves important decisions/facts to memory.
func PruneConversation(
	ctx context.Context,
	messages []anthropic.MessageParam,
	memStore *memory.Store,
	sessionID string,
	cfg PruneConfig,
) ([]anthropic.MessageParam, *PruneResult, error) {
	result := &PruneResult{
		OriginalMessages: len(messages),
		Strategy:         fmt.Sprintf("role-based-%s", cfg.Role),
	}

	// If we have fewer messages than the threshold, no pruning needed
	if len(messages) <= cfg.KeepRecentTurns {
		return messages, result, nil
	}

	// Calculate message ages (turns from the end)
	messageAges := make([]int, len(messages))
	turnsFromEnd := 0
	for i := len(messages) - 1; i >= 0; i-- {
		messageAges[i] = turnsFromEnd
		// Count turns by user messages
		if messages[i].Role == anthropic.MessageParamRoleUser {
			turnsFromEnd++
		}
	}

	// Auto-save important content to memory before pruning
	if memStore != nil {
		saved, err := autoSaveToMemory(ctx, messages, memStore, sessionID, cfg, messageAges)
		if err != nil {
			// Log error but continue with pruning
			fmt.Printf("warning: failed to auto-save memories: %v\n", err)
		} else {
			result.MemoriesSaved = saved
		}
	}

	// Apply role-based pruning per message
	var prunedMessages []anthropic.MessageParam
	tokensFreed := 0
	
	for i, msg := range messages {
		age := messageAges[i]
		prunedMsg := msg
		pruned := false

		switch msg.Role {
		case anthropic.MessageParamRoleUser:
			// User messages: always keep full (they're small)
			// But process tool results within them
			var prunedContent []anthropic.ContentBlockParamUnion
			for _, block := range msg.Content {
				if block.OfToolResult != nil {
					// Apply tool result pruning
					prunedBlock, wasPruned := pruneToolResult(block, age, cfg)
					prunedContent = append(prunedContent, prunedBlock)
					if wasPruned {
						pruned = true
						if age >= cfg.ToolResultDrop {
							result.DroppedToolResults++
						} else {
							result.TruncatedToolCalls++
						}
					}
				} else {
					prunedContent = append(prunedContent, block)
				}
			}
			if pruned {
				prunedMsg.Content = prunedContent
			}

		case anthropic.MessageParamRoleAssistant:
			// Assistant messages: truncate based on age
			if age > cfg.AssistantTruncateAfter {
				var prunedContent []anthropic.ContentBlockParamUnion
				for _, block := range msg.Content {
					if block.OfText != nil {
						// Truncate to first 2-3 sentences
						truncated := truncateToSummary(block.OfText.Text)
						prunedContent = append(prunedContent, anthropic.ContentBlockParamUnion{
							OfText: &anthropic.TextBlockParam{
								Text: truncated,
							},
						})
						pruned = true
						result.TruncatedAssistant++
					} else if block.OfToolUse != nil {
						// Tool calls: truncate input if old
						if age > cfg.ToolCallKeepFull {
							prunedContent = append(prunedContent, truncateToolCall(block))
							pruned = true
							result.TruncatedToolCalls++
						} else {
							prunedContent = append(prunedContent, block)
						}
					} else {
						prunedContent = append(prunedContent, block)
					}
				}
				if pruned {
					prunedMsg.Content = prunedContent
				}
			} else {
				// Recent assistant messages: still check tool calls
				var prunedContent []anthropic.ContentBlockParamUnion
				for _, block := range msg.Content {
					if block.OfToolUse != nil && age > cfg.ToolCallKeepFull {
						prunedContent = append(prunedContent, truncateToolCall(block))
						pruned = true
						result.TruncatedToolCalls++
					} else {
						prunedContent = append(prunedContent, block)
					}
				}
				if pruned {
					prunedMsg.Content = prunedContent
				}
			}
		}

		if pruned {
			tokensFreed += estimateMessageTokens(msg) - estimateMessageTokens(prunedMsg)
		}
		prunedMessages = append(prunedMessages, prunedMsg)
	}

	result.PrunedMessages = len(messages) - len(prunedMessages)
	result.TokensFreed = tokensFreed
	return prunedMessages, result, nil
}