package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/memory"
)

// PruneConfig configures conversation pruning behavior
type PruneConfig struct {
	KeepRecentTurns      int     // Keep last N conversation turns in full (default: 35)
	AggressiveTruncation bool    // Apply aggressive truncation to old tool results
	MemorySaveThreshold  int     // Auto-save to memory if pruning would remove this many turns (default: 10)
	TokenBudget          int     // Target token budget for pruned conversation
	MinimumImportance    float64 // Minimum importance score for auto-saving memories (default: 0.3)
}

// DefaultPruneConfig returns sensible defaults for conversation pruning
func DefaultPruneConfig() PruneConfig {
	return PruneConfig{
		KeepRecentTurns:      35,
		AggressiveTruncation: true,
		MemorySaveThreshold:  10,
		TokenBudget:          50000, // Conservative default
		MinimumImportance:    0.3,
	}
}

// PruneResult contains statistics about what was pruned
type PruneResult struct {
	OriginalMessages   int
	PrunedMessages     int
	TokensFreed        int
	MemoriesSaved      int
	Strategy           string
	TruncatedToolCalls int
}

// PruneConversation intelligently prunes conversation history while preserving important information.
// It keeps the last N turns in full and summarizes/removes older content.
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
		Strategy:         "keep-recent-turns",
	}

	// If we have fewer messages than the threshold, no pruning needed
	if len(messages) <= cfg.KeepRecentTurns {
		return messages, result, nil
	}

	// Split into old (to be pruned/summarized) and recent (keep as-is)
	splitPoint := len(messages) - cfg.KeepRecentTurns
	oldMessages := messages[:splitPoint]
	recentMessages := messages[splitPoint:]

	// Auto-save important content to memory before pruning
	if memStore != nil && len(oldMessages) >= cfg.MemorySaveThreshold {
		saved, err := autoSaveToMemory(ctx, oldMessages, memStore, sessionID, cfg.MinimumImportance)
		if err != nil {
			// Log error but continue with pruning
			fmt.Printf("warning: failed to auto-save memories: %v\n", err)
		} else {
			result.MemoriesSaved = saved
		}
	}

	// Apply aggressive truncation to old tool results
	if cfg.AggressiveTruncation {
		truncated := truncateOldToolResults(oldMessages)
		result.TruncatedToolCalls = truncated
	}

	// Estimate tokens freed
	tokensFreed := 0
	for _, msg := range oldMessages {
		tokensFreed += estimateMessageTokens(msg)
	}
	result.TokensFreed = tokensFreed

	// Create summarized version of old messages (optional)
	// For now, we just drop them since they're in memory
	// Future: could create a summary message

	result.PrunedMessages = len(oldMessages)
	return recentMessages, result, nil
}

// autoSaveToMemory extracts key decisions and facts from old messages and saves them to memory
func autoSaveToMemory(
	ctx context.Context,
	messages []anthropic.MessageParam,
	memStore *memory.Store,
	sessionID string,
	minImportance float64,
) (int, error) {
	saved := 0

	// Extract content from messages
	var userMessages []string
	var assistantMessages []string

	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleUser {
			content := extractTextContent(msg.Content)
			if content != "" {
				userMessages = append(userMessages, content)
			}
		} else if msg.Role == anthropic.MessageParamRoleAssistant {
			content := extractTextContent(msg.Content)
			if content != "" {
				assistantMessages = append(assistantMessages, content)
			}
		}
	}

	// Look for decision indicators
	decisionPatterns := []string{
		"decided to", "choosing", "will use", "plan is to",
		"implemented", "created", "built", "fixed",
		"important:", "note:", "remember:",
	}

	// Scan assistant messages for decisions/key facts
	for _, content := range assistantMessages {
		lowerContent := strings.ToLower(content)
		
		// Check for decision/fact indicators
		hasDecision := false
		for _, pattern := range decisionPatterns {
			if strings.Contains(lowerContent, pattern) {
				hasDecision = true
				break
			}
		}

		if hasDecision && len(content) > 50 { // Ignore trivial messages
			// Extract key sentences (simple heuristic)
			sentences := extractKeySentences(content, 3)
			if len(sentences) > 0 {
				fact := strings.Join(sentences, " ")
				
				// Save to memory with appropriate importance
				importance := 0.5 // Default for auto-saved facts
				if strings.Contains(lowerContent, "important") {
					importance = 0.7
				}
				
				if importance >= minImportance {
					err := memStore.Save(memory.Memory{
						Content:    fact,
						Tags:       []string{"auto-saved", "decision", sessionID},
						Importance: importance,
						Source:     "pruning",
					})
					if err == nil {
						saved++
					}
				}
			}
		}
	}

	// Also save user requests/preferences
	for _, content := range userMessages {
		lowerContent := strings.ToLower(content)
		
		// Look for preference indicators
		if strings.Contains(lowerContent, "prefer") ||
			strings.Contains(lowerContent, "always") ||
			strings.Contains(lowerContent, "never") ||
			strings.Contains(lowerContent, "remember") {
			
			if len(content) > 30 && len(content) < 500 { // Reasonable size for a preference
				err := memStore.Save(memory.Memory{
					Content:    content,
					Tags:       []string{"auto-saved", "preference", sessionID},
					Importance: 0.6,
					Source:     "pruning",
				})
				if err == nil {
					saved++
				}
			}
		}
	}

	return saved, nil
}

// extractKeySentences extracts the first N sentences from text
func extractKeySentences(text string, maxSentences int) []string {
	// Simple sentence splitter (not perfect but good enough)
	sentences := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})

	var result []string
	for i, sentence := range sentences {
		if i >= maxSentences {
			break
		}
		sentence = strings.TrimSpace(sentence)
		if len(sentence) > 20 { // Skip very short fragments
			result = append(result, sentence)
		}
	}

	return result
}

// truncateOldToolResults applies aggressive truncation to tool results in old messages
func truncateOldToolResults(messages []anthropic.MessageParam) int {
	truncated := 0

	for i := range messages {
		if messages[i].Role != anthropic.MessageParamRoleUser {
			continue
		}

		// Check if message contains tool results
		for j := range messages[i].Content {
			if messages[i].Content[j].OfToolResult != nil {
				toolResult := messages[i].Content[j].OfToolResult
				
				// Get original content - toolResult.Content is []ToolResultBlockParamContentUnion
				originalContent := ""
				for _, block := range toolResult.Content {
					if block.OfText != nil {
						originalContent += block.OfText.Text
					}
				}

				if originalContent != "" && len(originalContent) > 200 {
					// Apply summarization
					toolName := toolResult.ToolUseID // Not ideal but we don't have tool name here
					summarized := summarizeOldToolResult(string(toolName), originalContent)
					
					// Update the content - replace with a single text block
					toolResult.Content = []anthropic.ToolResultBlockParamContentUnion{
						{
							OfText: &anthropic.TextBlockParam{
								Text: summarized,
							},
						},
					}
					truncated++
				}
			}
		}
	}

	return truncated
}

// extractTextContent extracts text from a message's content blocks
func extractTextContent(content []anthropic.ContentBlockParamUnion) string {
	var parts []string
	for _, block := range content {
		if block.OfText != nil {
			parts = append(parts, block.OfText.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// estimateMessageTokens estimates token count for a message
func estimateMessageTokens(msg anthropic.MessageParam) int {
	content := extractTextContent(msg.Content)
	return inbercontext.EstimateTokens(content)
}

// ShouldPrune determines if conversation should be pruned based on current size
func ShouldPrune(messages []anthropic.MessageParam, cfg PruneConfig) bool {
	if len(messages) <= cfg.KeepRecentTurns {
		return false
	}

	// Also check token count
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += estimateMessageTokens(msg)
	}

	return totalTokens > cfg.TokenBudget
}

// summarizeOldToolResult creates a brief summary of an old tool result.
func summarizeOldToolResult(toolName, fullOutput string) string {
	lines := strings.Split(fullOutput, "\n")
	lineCount := len(lines)

	// For very short results, just return as-is
	if lineCount <= 3 {
		return fullOutput
	}

	// Extract key information
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("[%s result, %d lines]", toolName, lineCount))

	// Generic: first + last line
	if lineCount > 0 {
		summary.WriteString("\n")
		summary.WriteString(lines[0])
		if lineCount > 1 {
			summary.WriteString("\n[...]")
			summary.WriteString("\n")
			summary.WriteString(lines[lineCount-1])
		}
	}

	return summary.String()
}

// PruningStrategy describes how messages were pruned
type PruningStrategy struct {
	Name        string
	Description string
}

var (
	StrategyKeepRecent = PruningStrategy{
		Name:        "keep-recent",
		Description: "Keep last N turns in full, remove older turns",
	}
	StrategyAggressiveTruncate = PruningStrategy{
		Name:        "aggressive-truncate",
		Description: "Keep recent turns, truncate old tool results",
	}
	StrategySummarize = PruningStrategy{
		Name:        "summarize",
		Description: "Keep recent turns, summarize old conversation with LLM",
	}
)
