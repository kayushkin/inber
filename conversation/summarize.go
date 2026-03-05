package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/memory"
)

// SummarizeConfig configures conversation summarization behavior
type SummarizeConfig struct {
	// Trigger: summarize when message count exceeds this
	TriggerMessages int
	// How many recent turns to keep in full (never summarize)
	KeepRecentTurns int
	// Model to use for summarization (empty = same as agent)
	Model string
	// Max tokens for the summary output
	MaxSummaryTokens int
	// Save full conversation to memory before summarizing
	SaveToMemory bool
}

// DefaultSummarizeConfig returns defaults based on agent role
func DefaultSummarizeConfig(role AgentRole) SummarizeConfig {
	switch role {
	case RoleOrchestrator:
		return SummarizeConfig{
			TriggerMessages:  80, // ~40 turns
			KeepRecentTurns:  15,
			MaxSummaryTokens: 1024,
			SaveToMemory:     true,
		}
	case RoleCoder:
		return SummarizeConfig{
			TriggerMessages:  40, // ~20 turns
			KeepRecentTurns:  8,
			MaxSummaryTokens: 800,
			SaveToMemory:     true,
		}
	default:
		return SummarizeConfig{
			TriggerMessages:  60,
			KeepRecentTurns:  12,
			MaxSummaryTokens: 1024,
			SaveToMemory:     true,
		}
	}
}

// SummarizeResult describes what happened during summarization
type SummarizeResult struct {
	Summarized       bool
	OriginalMessages int
	KeptMessages     int
	SummarizedTurns  int
	SummaryTokens    int
	MemorySaved      bool
	MemoryID         string
}

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
	memStore *memory.Store,
	sessionID string,
	cfg SummarizeConfig,
	model string,
) ([]anthropic.MessageParam, *SummarizeResult, error) {
	result := &SummarizeResult{
		OriginalMessages: len(messages),
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
	oldTokens := inbercontext.EstimateTokens(oldText)

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
		// Fallback: mechanical summary (no LLM call)
		summary = mechanicalSummary(oldMessages)
	}

	result.Summarized = true
	result.SummaryTokens = inbercontext.EstimateTokens(summary)
	result.KeptMessages = len(recentMessages) + 2 // +2 for summary user+assistant pair

	// Build new message list: summary + recent messages
	// The summary goes as a user message with assistant acknowledgment
	// to maintain valid message alternation
	summaryBlock := fmt.Sprintf("[Conversation Summary — %d earlier turns condensed]\n\n%s\n\n[End of summary. Recent conversation follows.]", result.SummarizedTurns, summary)

	var newMessages []anthropic.MessageParam

	// If recent messages start with assistant, we need user→assistant→(recent...)
	// If recent messages start with user, we need user→assistant→user→...
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

// findTurnBoundary finds the message index where we should split.
// Counts N turns from the end, returns the index of where "old" messages end.
// Ensures we don't split between a tool_use and its tool_result.
func findTurnBoundary(messages []anthropic.MessageParam, keepTurns int) int {
	turns := 0
	splitAt := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == anthropic.MessageParamRoleUser {
			turns++
			if turns >= keepTurns {
				splitAt = i
				break
			}
		}
	}

	// Now verify: check if any message in the "keep" section (splitAt onward)
	// has a tool_result whose tool_use is in the "old" section (before splitAt).
	// If so, move the split point back to include the tool_use message.
	for {
		toolUseIDs := collectToolUseIDs(messages[:splitAt])
		orphanedResults := findOrphanedToolResults(messages[splitAt:], toolUseIDs)
		if len(orphanedResults) == 0 {
			break
		}
		// Move split back — find the earliest tool_use that's needed
		if splitAt <= 0 {
			break
		}
		splitAt--
		// Back up to the previous user message boundary
		for splitAt > 0 && messages[splitAt].Role != anthropic.MessageParamRoleUser {
			splitAt--
		}
	}

	return splitAt
}

// collectToolUseIDs returns all tool_use IDs in the given messages
func collectToolUseIDs(messages []anthropic.MessageParam) map[string]bool {
	ids := make(map[string]bool)
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolUse != nil {
				ids[block.OfToolUse.ID] = true
			}
		}
	}
	return ids
}

// findOrphanedToolResults checks if any tool_result in messages references
// a tool_use ID that's NOT in the provided set (meaning the tool_use was removed)
func findOrphanedToolResults(messages []anthropic.MessageParam, removedToolUseIDs map[string]bool) []string {
	// First collect all tool_use IDs in these messages
	localToolUseIDs := collectToolUseIDs(messages)

	var orphaned []string
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				id := block.OfToolResult.ToolUseID
				// Orphaned if: the tool_use is in the removed set AND not in the local set
				if removedToolUseIDs[id] && !localToolUseIDs[id] {
					orphaned = append(orphaned, id)
				}
			}
		}
	}
	return orphaned
}

// countTurns counts user messages (each = one turn)
func countTurns(messages []anthropic.MessageParam) int {
	turns := 0
	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleUser {
			turns++
		}
	}
	return turns
}

// messagesToText converts messages to a readable text format for summarization
func messagesToText(messages []anthropic.MessageParam) string {
	var sb strings.Builder
	for _, msg := range messages {
		role := "User"
		if msg.Role == anthropic.MessageParamRoleAssistant {
			role = "Assistant"
		}
		sb.WriteString(fmt.Sprintf("[%s]\n", role))
		for _, block := range msg.Content {
			if block.OfText != nil {
				sb.WriteString(block.OfText.Text)
				sb.WriteString("\n")
			} else if block.OfToolUse != nil {
				inputJSON, _ := json.Marshal(block.OfToolUse.Input)
				inputStr := string(inputJSON)
				if len(inputStr) > 200 {
					inputStr = inputStr[:200] + "..."
				}
				sb.WriteString(fmt.Sprintf("[Tool: %s(%s)]\n", block.OfToolUse.Name, inputStr))
			} else if block.OfToolResult != nil {
				content := extractToolResultText(block)
				if len(content) > 300 {
					content = content[:300] + "..."
				}
				sb.WriteString(fmt.Sprintf("[Tool Result: %s]\n", content))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// extractToolResultText gets text content from a tool result block
func extractToolResultText(block anthropic.ContentBlockParamUnion) string {
	if block.OfToolResult == nil {
		return ""
	}
	var texts []string
	for _, content := range block.OfToolResult.Content {
		if content.OfText != nil {
			texts = append(texts, content.OfText.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// generateSummary uses the LLM to create a concise summary
func generateSummary(
	ctx context.Context,
	client *anthropic.Client,
	conversationText string,
	model string,
	maxTokens int,
) (string, error) {
	// Truncate input if too long (keep under ~4K tokens for the summary call)
	maxInputChars := 12000 // ~4K tokens
	if len(conversationText) > maxInputChars {
		conversationText = conversationText[:maxInputChars] + "\n...[truncated]"
	}

	prompt := fmt.Sprintf(`Summarize this conversation concisely. Capture:
1. What was discussed/decided (key topics and outcomes)
2. What was built/changed (files, features, fixes)
3. Current state (what's working, what's pending)
4. Any important context for continuing the work

Be direct and dense — this replaces the conversation history.
Skip greetings, confirmations, and routine exchanges.

Conversation:
%s`, conversationText)

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("summarization API call failed: %w", err)
	}

	var summary string
	for _, block := range resp.Content {
		if block.Type == "text" {
			summary += block.Text
		}
	}

	if summary == "" {
		return "", fmt.Errorf("empty summary response")
	}

	return summary, nil
}

// mechanicalSummary creates a summary without LLM (fallback)
func mechanicalSummary(messages []anthropic.MessageParam) string {
	var sb strings.Builder
	sb.WriteString("Previous conversation covered:\n")

	toolsUsed := make(map[string]int)
	topics := []string{}
	turns := 0

	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleUser {
			turns++
			// Extract first line of user messages as topic hints
			for _, block := range msg.Content {
				if block.OfText != nil {
					firstLine := strings.SplitN(block.OfText.Text, "\n", 2)[0]
					if len(firstLine) > 100 {
						firstLine = firstLine[:100] + "..."
					}
					if len(firstLine) > 10 { // skip very short messages
						topics = append(topics, firstLine)
					}
				}
			}
		}
		for _, block := range msg.Content {
			if block.OfToolUse != nil {
				toolsUsed[block.OfToolUse.Name]++
			}
		}
	}

	sb.WriteString(fmt.Sprintf("- %d conversation turns\n", turns))

	if len(toolsUsed) > 0 {
		sb.WriteString("- Tools used: ")
		var toolList []string
		for name, count := range toolsUsed {
			toolList = append(toolList, fmt.Sprintf("%s(%d)", name, count))
		}
		sb.WriteString(strings.Join(toolList, ", "))
		sb.WriteString("\n")
	}

	// Show up to 10 topic hints
	maxTopics := 10
	if len(topics) < maxTopics {
		maxTopics = len(topics)
	}
	if maxTopics > 0 {
		sb.WriteString("- Topics discussed:\n")
		for _, topic := range topics[:maxTopics] {
			sb.WriteString(fmt.Sprintf("  • %s\n", topic))
		}
		if len(topics) > maxTopics {
			sb.WriteString(fmt.Sprintf("  • ...and %d more exchanges\n", len(topics)-maxTopics))
		}
	}

	return sb.String()
}

// fixAlternation ensures messages alternate user/assistant correctly
func fixAlternation(messages []anthropic.MessageParam) []anthropic.MessageParam {
	if len(messages) <= 1 {
		return messages
	}

	var fixed []anthropic.MessageParam
	fixed = append(fixed, messages[0])

	for i := 1; i < len(messages); i++ {
		prev := fixed[len(fixed)-1]
		curr := messages[i]

		if prev.Role == curr.Role {
			if curr.Role == anthropic.MessageParamRoleUser {
				// Two user messages in a row — merge
				fixed[len(fixed)-1].Content = append(fixed[len(fixed)-1].Content, curr.Content...)
			} else {
				// Two assistant messages — insert a placeholder user message
				fixed = append(fixed, anthropic.NewUserMessage(
					anthropic.NewTextBlock("[continued]"),
				))
				fixed = append(fixed, curr)
			}
		} else {
			fixed = append(fixed, curr)
		}
	}

	return fixed
}

// stripOrphanedToolResults removes tool_result blocks that don't have
// a matching tool_use in the message set. This happens when summarization
// removes messages containing tool_use but keeps the tool_result response.
func stripOrphanedToolResults(messages []anthropic.MessageParam) []anthropic.MessageParam {
	// Collect all tool_use IDs
	toolUseIDs := collectToolUseIDs(messages)

	var result []anthropic.MessageParam
	for _, msg := range messages {
		hasContent := false
		var cleanedContent []anthropic.ContentBlockParamUnion
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				if !toolUseIDs[block.OfToolResult.ToolUseID] {
					// Orphaned — skip this block
					continue
				}
			}
			cleanedContent = append(cleanedContent, block)
			hasContent = true
		}
		if hasContent {
			msg.Content = cleanedContent
			result = append(result, msg)
		}
	}
	return result
}

// ConversationCheckpoint represents a saved conversation state
type ConversationCheckpoint struct {
	SessionID   string    `json:"session_id"`
	Timestamp   time.Time `json:"timestamp"`
	TurnCount   int       `json:"turn_count"`
	SummaryText string    `json:"summary_text"`
	MemoryID    string    `json:"memory_id,omitempty"`
}
