package conversation

import (
	"context"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"
)

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