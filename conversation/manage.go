package conversation

import (
	"context"
	"fmt"
	"log"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"
)

// ManagementResult contains statistics about what was managed (pruned/stashed)
type ManagementResult struct {
	OriginalMessages   int
	ManagedMessages    int
	PrunedMessages     int // Alias for ManagedMessages for backward compatibility
	TokensFreed        int
	MemoriesSaved      int
	Strategy           string
	TruncatedToolCalls int
	TruncatedAssistant int
	DroppedToolResults int
	StashedBlocks      int
	DeduplicatedFiles  int
}

// PruneResult is an alias for backward compatibility
type PruneResult = ManagementResult

// ManageConversation applies unified conversation management including both
// pruning (truncating old content) and stashing (moving large content to memory).
// It replaces the separate PruneConversation and stashing logic.
func ManageConversation(
	ctx context.Context,
	messages []anthropic.MessageParam,
	memStore memory.MemoryStore,
	sessionID string,
	cfg ManagementConfig,
) ([]anthropic.MessageParam, *ManagementResult, error) {
	result := &ManagementResult{
		OriginalMessages: len(messages),
		Strategy:         fmt.Sprintf("unified-%s", cfg.Role),
	}

	// If we have fewer messages than the threshold, no management needed
	if len(messages) <= cfg.KeepRecentTurns {
		return messages, result, nil
	}

	// Step 0: Deduplicate file references — replace older read/write/edit results
	// for the same file path with a compact stub. Only the latest is kept full.
	deduped := DeduplicateFileRefs(messages)
	result.DeduplicatedFiles = deduped

	// Step 1: Apply stashing first (move large content blocks to memory)
	managedMessages, totalStashed, err := ApplyStashing(messages, sessionID, memStore, cfg.Stash)
	if err != nil {
		return nil, nil, fmt.Errorf("apply stashing: %w", err)
	}
	
	result.StashedBlocks = totalStashed

	// Step 2: Apply pruning logic (truncate old content)
	messageAges := make([]int, len(managedMessages))
	turnsFromEnd := 0
	for i := len(managedMessages) - 1; i >= 0; i-- {
		messageAges[i] = turnsFromEnd
		if managedMessages[i].Role == anthropic.MessageParamRoleUser {
			turnsFromEnd++
		}
	}

	// Auto-save important content to memory before pruning
	if memStore != nil {
		saved, err := autoSaveToMemory(ctx, managedMessages, memStore, sessionID, cfg, messageAges)
		if err != nil {
			log.Printf("[warn] failed to auto-save memories: %v", err)
		} else {
			result.MemoriesSaved = saved
		}
	}

	// Apply role-based pruning per message
	var finalMessages []anthropic.MessageParam
	tokensFreed := 0
	
	for i, msg := range managedMessages {
		age := messageAges[i]
		managedMsg := msg
		pruned := false

		switch msg.Role {
		case anthropic.MessageParamRoleUser:
			// User messages: keep full text, but process tool results within them
			var prunedContent []anthropic.ContentBlockParamUnion
			for _, block := range msg.Content {
				if block.OfToolResult != nil {
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
				managedMsg.Content = prunedContent
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
							OfText: &anthropic.TextBlockParam{Text: truncated},
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
					managedMsg.Content = prunedContent
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
					managedMsg.Content = prunedContent
				}
			}
		}

		if pruned {
			tokensFreed += estimateMessageTokens(msg) - estimateMessageTokens(managedMsg)
		}
		finalMessages = append(finalMessages, managedMsg)
	}

	result.ManagedMessages = len(messages) - len(finalMessages)
	result.PrunedMessages = result.ManagedMessages // Backward compatibility
	result.TokensFreed = tokensFreed
	return finalMessages, result, nil
}

// PruneConversation is a backward compatibility wrapper
func PruneConversation(
	ctx context.Context,
	messages []anthropic.MessageParam,
	memStore memory.MemoryStore,
	sessionID string,
	cfg PruneConfig,
) ([]anthropic.MessageParam, *PruneResult, error) {
	return ManageConversation(ctx, messages, memStore, sessionID, cfg)
}

// ShouldManage determines if conversation should be managed based on current size
func ShouldManage(messages []anthropic.MessageParam, cfg ManagementConfig) bool {
	return ShouldPrune(messages, cfg)
}

// ShouldPrune determines if conversation should be pruned (backward compatibility)
func ShouldPrune(messages []anthropic.MessageParam, cfg PruneConfig) bool {
	// Message count check — prune if we have too many messages regardless of
	// token estimate (the estimator is known to undercount by 3-4x)
	if len(messages) > cfg.KeepRecentTurns*2 {
		return true
	}

	if len(messages) <= cfg.KeepRecentTurns {
		return false
	}

	// Token budget check for borderline cases
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += estimateMessageTokens(msg)
	}

	return totalTokens > cfg.TokenBudget
}