package conversation

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/kayushkin/inber/memory"
)

// ShouldSummarize checks if the conversation is long enough to warrant summarization
func ShouldSummarize(messages []anthropic.MessageParam, cfg SummarizeConfig) bool {
	return len(messages) > cfg.TriggerMessages
}

// SummarizeConversation compresses old conversation turns into a summary.
// It keeps the most recent turns in full and replaces older ones with a
// summary message. The full conversation is optionally saved to memory.
//
// Returns the new (shorter) message list with a summary prefix.
func SummarizeConversation(
	ctx context.Context,
	client *anthropic.Client,
	messages []anthropic.MessageParam,
	memStore memory.MemoryStore,
	sessionID string,
	cfg SummarizeConfig,
	model string,
) ([]anthropic.MessageParam, *SummarizeResult, error) {
	result := &SummarizeResult{
		KeptMessages: len(messages),
	}

	if len(messages) <= cfg.TriggerMessages {
		return messages, result, nil
	}

	// Find the split point: keep last N turns (a turn = user + assistant pair)
	keepFrom := findTurnBoundary(messages, cfg.KeepRecentTurns)
	if keepFrom <= 0 {
		// Nothing to summarize
		return messages, result, nil
	}

	oldMessages := messages[:keepFrom]
	recentMessages := messages[keepFrom:]
	result.SummarizedTurns = countTurns(oldMessages)

	// Build text representation of old conversation for summarization
	oldText := messagesToText(oldMessages)
	oldTokens := memory.EstimateTokens(oldText)

	// Save full old conversation to memory before summarizing
	if cfg.SaveToMemory && memStore != nil {
		memID := fmt.Sprintf("conversation-summary:%s:%s", sessionID, uuid.New().String()[:8])
		err := memStore.Save(memory.Memory{
			ID:         memID,
			Content:    oldText,
			Summary:    fmt.Sprintf("Full conversation history (%d turns, ~%d tokens) from session %s", result.SummarizedTurns, oldTokens, sessionID),
			Tags:       []string{"conversation", "history", "session:" + sessionID},
			Importance: 0.4,
			Source:     "summarization",
			IsLazy:     true, // Don't auto-load, but available via memory_expand
		})
		if err != nil {
			// Log but don't fail
			fmt.Printf("warning: failed to save conversation to memory: %v\n", err)
		} else {
			result.MemorySaved = true
			result.MemoryID = memID
		}
	}

	// Generate summary via LLM
	summaryModel := cfg.Model
	if summaryModel == "" {
		summaryModel = model
	}

	summary, err := generateSummary(ctx, client, oldText, summaryModel, cfg.MaxSummaryTokens)
	if err != nil {
		// Fallback: mechanical summary (no LLM call). Record the degradation —
		// a caller that cannot tell an LLM summary from a word-frequency list
		// will report a successful compaction that silently lost the reasoning.
		summary = mechanicalSummary(oldMessages)
		result.SummaryDegraded = true
		result.SummaryError = err.Error()
		fmt.Printf("warning: LLM summarization failed, using mechanical fallback: %v\n", err)
	}

	result.Summarized = true
	result.SummaryTokens = memory.EstimateTokens(summary)
	result.KeptMessages = len(recentMessages) + 2 // +2 for summary user+assistant pair

	// Build new message list: summary + recent messages
	// The summary goes as a user message with assistant acknowledgment
	// to maintain valid message alternation
	summaryBlock := fmt.Sprintf("[Conversation Summary — %d earlier turns condensed]\n\n%s\n\n[End of summary. Recent conversation follows.]", result.SummarizedTurns, summary)

	var newMessages []anthropic.MessageParam

	// Add summary as user message followed by assistant acknowledgment
	newMessages = append(newMessages, anthropic.NewUserMessage(
		anthropic.NewTextBlock(summaryBlock),
	))
	newMessages = append(newMessages, anthropic.NewAssistantMessage(
		anthropic.NewTextBlock("Understood. I have the conversation context from the summary above. Continuing from where we left off."),
	))
	
	// Strip orphaned tool_results from recent messages
	// (tool_use was in summarized messages, tool_result is in kept messages)
	recentMessages = stripOrphanedToolResults(recentMessages)

	// Append recent messages, ensuring valid alternation
	for _, msg := range recentMessages {
		newMessages = append(newMessages, msg)
	}

	// Fix any alternation issues
	newMessages = fixAlternation(newMessages)

	return newMessages, result, nil
}