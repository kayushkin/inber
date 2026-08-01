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

	// Generate summary via LLM. This is the one step that can fail, and it runs
	// before anything is written or dropped: the summary is what the older turns
	// are exchanged for, so without it there is nothing to exchange them for and
	// the caller keeps the transcript it handed in.
	summaryModel := cfg.Model
	if summaryModel == "" {
		summaryModel = model
	}

	summary, err := generateSummary(ctx, client, oldText, summaryModel, cfg.MaxSummaryTokens)
	if err != nil {
		return messages, result, fmt.Errorf("summarizing %d earlier turns: %w", result.SummarizedTurns, err)
	}

	// Save the full old conversation to memory. This runs only after the summary
	// exists, because the row claims a compaction happened: writing it on the
	// failure path would file a copy of the whole transcript on every retry.
	//
	// The row is kept out of the next prompt by its tags — see
	// ConversationArchiveTags. It used to say so with `IsLazy: true` and the
	// comment "don't auto-load, but available via memory_expand", which was not a
	// second belt: memory-store's IsLazy means "the content is not in this row,
	// read it from RefTarget", the content here is in the row, and no read path
	// has ever treated the flag as a context policy. A flag that states an intent
	// nothing enforces reads as protection and is not any, so it is gone rather
	// than kept alongside the tag that does the work.
	if cfg.SaveToMemory && memStore != nil {
		memID := fmt.Sprintf("conversation-summary:%s:%s", sessionID, uuid.New().String()[:8])
		if err := memStore.Save(memory.Memory{
			ID:         memID,
			Content:    oldText,
			Summary:    fmt.Sprintf("Full conversation history (%d turns, ~%d tokens) from session %s", result.SummarizedTurns, oldTokens, sessionID),
			Tags:       ConversationArchiveTags(sessionID),
			Importance: 0.4,
			Source:     "summarization",
		}); err != nil {
			// Log but don't fail
			fmt.Printf("warning: failed to save conversation to memory: %v\n", err)
		} else {
			result.MemorySaved = true
			result.MemoryID = memID
		}
	}

	result.Summarized = true
	result.SummaryTokens = memory.EstimateTokens(summary)

	// Build new message list: summary + recent messages
	// The summary goes as a user message with assistant acknowledgment
	// to maintain valid message alternation
	summaryBlock := fmt.Sprintf("[Conversation Summary — %d earlier turns condensed]\n\n%s\n\n%s", result.SummarizedTurns, summary, summaryFooter(result, cfg))

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

	// Count what is actually being returned. Stripping and alternation-fixing both
	// drop messages, so a count taken before them reports messages that no longer exist.
	result.KeptMessages = len(newMessages)

	return newMessages, result, nil
}

// summaryFooter closes the injected summary block, and names the archived
// transcript when — and only when — the model can actually get it back.
//
// The compaction archive was a write with no reachable read. It is saved on
// every compaction, it is tagged out of the automatic context so it is never
// offered back, and memory_expand needs an id that was returned to the caller,
// logged, and never put anywhere the model could see. StashLargeContent, the
// other writer that takes content out of a conversation, has always left its
// pointer inline; this one had nothing but a header claiming turns were
// "condensed", which reads as "gone".
//
// The two conditions are the write's own and the read's own: the archive was
// saved, and memory_expand is on the wire. A pointer emitted without the first
// promises a recall of something that was never stored; without the second it
// names a tool the model cannot call. Both are the failure this fixes, inverted.
func summaryFooter(result *SummarizeResult, cfg SummarizeConfig) string {
	const end = "[End of summary. Recent conversation follows.]"
	if !result.MemorySaved || !cfg.ArchiveIsRecallable {
		return end
	}
	return fmt.Sprintf(
		"[The %d condensed turns are archived verbatim. memory_expand(id=\"%s\") returns them.]\n%s",
		result.SummarizedTurns, result.MemoryID, end,
	)
}