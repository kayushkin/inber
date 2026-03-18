package conversation

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"
)

// AgentRole represents different agent roles with different management needs
type AgentRole string

const (
	RoleOrchestrator AgentRole = "orchestrator"
	RoleCoder        AgentRole = "coder"
	RoleTester       AgentRole = "tester"
	RoleDefault      AgentRole = "default"
)

// ManagementConfig configures conversation management behavior (pruning + stashing)
type ManagementConfig struct {
	Role                    AgentRole // Agent role determines strategy
	KeepRecentTurns         int       // Keep last N conversation turns in full
	AssistantTruncateAfter  int       // Truncate assistant messages older than N turns
	ToolResultKeepFull      int       // Keep tool results in full for last N turns
	ToolResultSummary       int       // Summarize tool results N to ToolResultKeepFull turns ago
	ToolResultDrop          int       // Drop tool results older than N turns
	ToolCallKeepFull        int       // Keep tool call inputs in full for last N turns
	AutoSaveThreshold       int       // Token count threshold for auto-saving to memory
	AggressiveTruncation    bool      // Legacy field for backwards compatibility
	MemorySaveThreshold     int       // Auto-save to memory if pruning would remove this many turns
	TokenBudget             int       // Target token budget for managed conversation
	MinimumImportance       float64   // Minimum importance score for auto-saving memories
	
	// Stashing configuration
	Stash                   StashConfig // Configuration for stashing large content
}

// PruneConfig is an alias for backward compatibility
type PruneConfig = ManagementConfig

// OrchestratorManagementConfig returns config optimized for orchestrator agents
func OrchestratorManagementConfig() ManagementConfig {
	return ManagementConfig{
		Role:                   RoleOrchestrator,
		KeepRecentTurns:        40,
		AssistantTruncateAfter: 8,
		ToolResultKeepFull:     3,
		ToolResultSummary:      8,
		ToolResultDrop:         8,
		ToolCallKeepFull:       5,
		AutoSaveThreshold:      500,
		AggressiveTruncation:   true,
		MemorySaveThreshold:    10,
		TokenBudget:            50000,
		MinimumImportance:      0.3,
		Stash: StashConfig{
			Enabled:               true,
			UserMessageThreshold:  1000,
			AssistantThreshold:    1500,
			MinBlockSize:         1000,
			DefaultImportance:    0.6,
		},
	}
}

// CoderManagementConfig returns config optimized for coder/implementer agents
func CoderManagementConfig() ManagementConfig {
	return ManagementConfig{
		Role:                   RoleCoder,
		KeepRecentTurns:        20,
		AssistantTruncateAfter: 15,
		ToolResultKeepFull:     10,
		ToolResultSummary:      20,
		ToolResultDrop:         20,
		ToolCallKeepFull:       5,
		AutoSaveThreshold:      1000,
		AggressiveTruncation:   true,
		MemorySaveThreshold:    10,
		TokenBudget:            50000,
		MinimumImportance:      0.3,
		Stash: StashConfig{
			Enabled:               true,
			UserMessageThreshold:  800,
			AssistantThreshold:    1200,
			MinBlockSize:         800,
			DefaultImportance:    0.6,
		},
	}
}

// TesterManagementConfig returns config optimized for tester/validator agents
func TesterManagementConfig() ManagementConfig {
	return ManagementConfig{
		Role:                   RoleTester,
		KeepRecentTurns:        20,
		AssistantTruncateAfter: 10,
		ToolResultKeepFull:     15, // Testers need test output
		ToolResultSummary:      25,
		ToolResultDrop:         25,
		ToolCallKeepFull:       5,
		AutoSaveThreshold:      1000,
		AggressiveTruncation:   true,
		MemorySaveThreshold:    10,
		TokenBudget:            50000,
		MinimumImportance:      0.3,
		Stash: StashConfig{
			Enabled:               true,
			UserMessageThreshold:  1000,
			AssistantThreshold:    1500,
			MinBlockSize:         1000,
			DefaultImportance:    0.6,
		},
	}
}

// DefaultManagementConfig returns sensible defaults for conversation management
func DefaultManagementConfig() ManagementConfig {
	return ManagementConfig{
		Role:                   RoleDefault,
		KeepRecentTurns:        35,
		AssistantTruncateAfter: 10,
		ToolResultKeepFull:     3,
		ToolResultSummary:      10,
		ToolResultDrop:         10,
		ToolCallKeepFull:       5,
		AutoSaveThreshold:      500,
		AggressiveTruncation:   true,
		MemorySaveThreshold:    10,
		TokenBudget:            50000,
		MinimumImportance:      0.3,
		Stash: StashConfig{
			Enabled:               true,
			UserMessageThreshold:  1000,
			AssistantThreshold:    1500,
			MinBlockSize:         1000,
			DefaultImportance:    0.6,
		},
	}
}

// Backward compatibility functions
func OrchestratorPruneConfig() PruneConfig { return OrchestratorManagementConfig() }
func CoderPruneConfig() PruneConfig { return CoderManagementConfig() }
func TesterPruneConfig() PruneConfig { return TesterManagementConfig() }
func DefaultPruneConfig() PruneConfig { return DefaultManagementConfig() }
// Removed: Use DefaultStashConfig() from stash.go instead

// ManagementConfigForRole returns the appropriate config for the given role string
func ManagementConfigForRole(roleStr string) ManagementConfig {
	lower := strings.ToLower(roleStr)
	
	// Check for orchestrator patterns
	if strings.Contains(lower, "orchestrat") || strings.Contains(lower, "coordinat") || 
	   strings.Contains(lower, "dispatch") || strings.Contains(lower, "delegat") {
		return OrchestratorManagementConfig()
	}
	
	// Check for coder patterns
	if strings.Contains(lower, "code") || strings.Contains(lower, "implement") || 
	   strings.Contains(lower, "scholar") || strings.Contains(lower, "develop") {
		return CoderManagementConfig()
	}
	
	// Check for tester patterns
	if strings.Contains(lower, "test") || strings.Contains(lower, "validat") || 
	   strings.Contains(lower, "sentinel") {
		return TesterManagementConfig()
	}
	
	return DefaultManagementConfig()
}

// Backward compatibility function
func PruneConfigForRole(roleStr string) PruneConfig {
	return ManagementConfigForRole(roleStr)
}

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
}

// PruneResult is an alias for backward compatibility
type PruneResult = ManagementResult

// Types moved to stash.go

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
// === Pruning utility functions (moved from prune.go) ===

// pruneToolResult applies age-based pruning to a tool result block
func pruneToolResult(block anthropic.ContentBlockParamUnion, age int, cfg ManagementConfig) (anthropic.ContentBlockParamUnion, bool) {
	if block.OfToolResult == nil {
		return block, false
	}

	toolResult := block.OfToolResult

	// Drop entirely if too old
	if age >= cfg.ToolResultDrop {
		return anthropic.ContentBlockParamUnion{
			OfToolResult: &anthropic.ToolResultBlockParam{
				ToolUseID: toolResult.ToolUseID,
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{
						OfText: &anthropic.TextBlockParam{
							Text: "[result dropped - too old]",
						},
					},
				},
			},
		}, true
	}

	// Keep full if recent
	if age < cfg.ToolResultKeepFull {
		return block, false
	}

	// Summarize if in middle range
	originalContent := extractToolResultContent(toolResult.Content)
	if originalContent == "" || len(originalContent) < 100 {
		return block, false // Keep short results as-is
	}

	// Create summary based on tool type (we don't have direct tool name, use heuristics)
	summary := summarizeToolResultByContent(originalContent)
	
	return anthropic.ContentBlockParamUnion{
		OfToolResult: &anthropic.ToolResultBlockParam{
			ToolUseID: toolResult.ToolUseID,
			Content: []anthropic.ToolResultBlockParamContentUnion{
				{
					OfText: &anthropic.TextBlockParam{
						Text: summary,
					},
				},
			},
		},
	}, true
}

// truncateToolCall summarizes a tool call input
func truncateToolCall(block anthropic.ContentBlockParamUnion) anthropic.ContentBlockParamUnion {
	if block.OfToolUse == nil {
		return block
	}

	toolUse := block.OfToolUse
	inputStr := fmt.Sprintf("%v", toolUse.Input)
	
	// Create brief summary
	summary := fmt.Sprintf("%s: %s", toolUse.Name, truncateToOneLine(inputStr, 60))
	
	// Return simplified version (keep structure but summarize input)
	return anthropic.ContentBlockParamUnion{
		OfToolUse: &anthropic.ToolUseBlockParam{
			ID:    toolUse.ID,
			Name:  toolUse.Name,
			Input: map[string]interface{}{"_summary": summary},
		},
	}
}

// extractToolResultContent extracts text from tool result content blocks
func extractToolResultContent(content []anthropic.ToolResultBlockParamContentUnion) string {
	var parts []string
	for _, block := range content {
		if block.OfText != nil {
			parts = append(parts, block.OfText.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// summarizeToolResultByContent creates a one-line summary of tool result content
func summarizeToolResultByContent(content string) string {
	lines := strings.Split(content, "\n")
	lineCount := len(lines)

	// Detect tool type by content patterns
	lower := strings.ToLower(content)
	
	// Shell command output
	if strings.Contains(content, "exit code") || strings.Contains(lower, "error:") || 
	   strings.Contains(lower, "warning:") || len(lines) > 5 {
		firstLine := ""
		if len(lines) > 0 {
			firstLine = truncateToOneLine(lines[0], 80)
		}
		return fmt.Sprintf("[shell: %d lines] %s", lineCount, firstLine)
	}
	
	// File read
	if lineCount > 20 && !strings.Contains(lower, "exit") {
		return fmt.Sprintf("[read file: %d lines, %d bytes]", lineCount, len(content))
	}
	
	// File write
	if strings.Contains(lower, "wrote") || strings.Contains(lower, "written") {
		return fmt.Sprintf("[wrote file: %d bytes]", len(content))
	}
	
	// List files
	if strings.Contains(content, "/") && lineCount > 3 {
		return fmt.Sprintf("[listed %d files]", lineCount)
	}
	
	// Memory search
	if strings.Contains(lower, "found") || strings.Contains(lower, "results") {
		return fmt.Sprintf("[search: %d results]", lineCount)
	}
	
	// Generic: first line + byte count
	firstLine := truncateToOneLine(lines[0], 80)
	return fmt.Sprintf("[%d bytes] %s", len(content), firstLine)
}

// truncateToOneLine truncates text to a single line with max length
func truncateToOneLine(text string, maxLen int) string {
	// Remove newlines
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.TrimSpace(text)
	
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// truncateToSummary extracts first 2-3 sentences from text
func truncateToSummary(text string) string {
	// Split into sentences (simple approach)
	sentences := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})
	
	var result []string
	charCount := 0
	for i, sentence := range sentences {
		if i >= 3 { // Max 3 sentences
			break
		}
		sentence = strings.TrimSpace(sentence)
		if len(sentence) < 10 { // Skip very short fragments
			continue
		}
		result = append(result, sentence)
		charCount += len(sentence)
		if charCount > 300 { // Max ~300 chars
			break
		}
	}
	
	if len(result) == 0 {
		// Fallback: just take first 300 chars
		if len(text) <= 300 {
			return text
		}
		return text[:300] + "..."
	}
	
	return strings.Join(result, ". ") + "."
}

// autoSaveToMemory extracts key decisions and facts from messages and saves them to memory
func autoSaveToMemory(
	ctx context.Context,
	messages []anthropic.MessageParam,
	memStore memory.MemoryStore,
	sessionID string,
	cfg ManagementConfig,
	messageAges []int,
) (int, error) {
	saved := 0

	// Only save assistant messages that will be truncated and are above threshold
	for i, msg := range messages {
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}
		
		age := messageAges[i]
		if age <= cfg.AssistantTruncateAfter {
			continue // Won't be truncated
		}

		content := extractTextContent(msg.Content)
		if content == "" {
			continue
		}

		tokens := memory.EstimateTokens(content)
		if tokens < cfg.AutoSaveThreshold {
			continue // Too short to bother saving
		}

		// Check for decision/fact indicators
		lowerContent := strings.ToLower(content)
		decisionPatterns := []string{
			"decided to", "choosing", "will use", "plan is to",
			"implemented", "created", "built", "fixed",
			"important:", "note:", "remember:",
		}

		hasDecision := false
		for _, pattern := range decisionPatterns {
			if strings.Contains(lowerContent, pattern) {
				hasDecision = true
				break
			}
		}

		if !hasDecision {
			continue
		}

		// Extract key sentences
		sentences := extractKeySentences(content, 3)
		if len(sentences) == 0 {
			continue
		}

		fact := strings.Join(sentences, " ")
		importance := 0.5
		if strings.Contains(lowerContent, "important") {
			importance = 0.7
		}

		if importance >= cfg.MinimumImportance {
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

// truncateOldToolResults applies aggressive truncation to tool results in old messages (legacy)
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
				
				// Get original content
				originalContent := extractToolResultContent(toolResult.Content)

				if originalContent != "" && len(originalContent) > 200 {
					// Apply summarization
					summarized := summarizeToolResultByContent(originalContent)
					
					// Update the content
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
	return memory.EstimateTokens(content)
}

// PruningStrategy describes how messages were managed (backward compatibility)
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
	StrategyRoleBased = PruningStrategy{
		Name:        "role-based",
		Description: "Apply role-specific pruning rules based on message type and age",
	}
	StrategyUnified = PruningStrategy{
		Name:        "unified",
		Description: "Apply unified management including stashing and pruning",
	}
)