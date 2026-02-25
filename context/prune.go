package context

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// PruneResult describes the outcome of a pruning operation.
type PruneResult struct {
	Removed     int    // number of messages removed
	TokensFreed int    // estimated tokens freed
	Strategy    string // strategy used
}

// PruneMessages removes messages from the conversation to fit within maxTokens.
// Strategies:
//   - "oldest" — drop oldest messages first, keeping the last 2 user/assistant turns
//   - "summary" — marks messages as prunable (placeholder for future LLM summarization)
func PruneMessages(messages *[]anthropic.MessageParam, maxTokens int, strategy string) PruneResult {
	result := PruneResult{Strategy: strategy}

	if messages == nil || len(*messages) == 0 {
		return result
	}

	switch strategy {
	case "oldest":
		result = pruneOldest(messages, maxTokens)
	case "summary":
		result = pruneSummary(messages, maxTokens)
	default:
		result = pruneOldest(messages, maxTokens)
		result.Strategy = "oldest"
	}

	return result
}

// estimateMessageTokens estimates the token count for a message list.
func estimateMessageTokens(messages []anthropic.MessageParam) int {
	total := 0
	for _, msg := range messages {
		total += estimateSingleMessageTokens(msg)
	}
	return total
}

func estimateSingleMessageTokens(msg anthropic.MessageParam) int {
	tokens := 4 // role overhead
	for _, block := range msg.Content {
		if block.OfText != nil {
			tokens += EstimateTokens(block.OfText.Text)
		}
		if block.OfToolUse != nil {
			tokens += 50 // rough estimate for tool use blocks
		}
		if block.OfToolResult != nil {
			tokens += 50 // rough estimate for tool results
		}
	}
	return tokens
}

// pruneOldest drops oldest messages first, keeping the last 2 turns (4 messages).
func pruneOldest(messages *[]anthropic.MessageParam, maxTokens int) PruneResult {
	result := PruneResult{Strategy: "oldest"}

	msgs := *messages
	total := estimateMessageTokens(msgs)

	if total <= maxTokens {
		return result
	}

	// Keep at least the last 4 messages (2 turns)
	keepAtEnd := 4
	if keepAtEnd > len(msgs) {
		keepAtEnd = len(msgs)
	}

	// Try removing from the front
	for len(msgs) > keepAtEnd && total > maxTokens {
		freed := estimateSingleMessageTokens(msgs[0])
		msgs = msgs[1:]
		total -= freed
		result.Removed++
		result.TokensFreed += freed
	}

	*messages = msgs
	return result
}

// pruneSummary is a placeholder that marks messages as prunable without actually removing them.
// Future implementation would use LLM summarization.
func pruneSummary(messages *[]anthropic.MessageParam, maxTokens int) PruneResult {
	result := PruneResult{Strategy: "summary"}

	msgs := *messages
	total := estimateMessageTokens(msgs)

	if total <= maxTokens {
		return result
	}

	// For now, fall back to oldest strategy but mark as summary
	keepAtEnd := 4
	if keepAtEnd > len(msgs) {
		keepAtEnd = len(msgs)
	}

	for len(msgs) > keepAtEnd && total > maxTokens {
		freed := estimateSingleMessageTokens(msgs[0])
		msgs = msgs[1:]
		total -= freed
		result.Removed++
		result.TokensFreed += freed
	}

	*messages = msgs
	return result
}
