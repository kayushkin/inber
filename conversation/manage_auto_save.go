package conversation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"
)

// autoSaveMemoryID names an auto-saved fact by the session it came from and the
// fact itself.
//
// It has to be deterministic and it has to be present. memory-store's Save
// upserts on the id and defaults everything about a row except that one field,
// so a Memory saved with no ID is written under the key "" — and this loop can
// save several facts in a single pruning pass. Every one of them landed on that
// same row, the last overwrote the rest, and the count returned to the caller
// reported all of them as saved.
//
// Hashing the fact rather than numbering the loop makes the repeat case right
// as well: pruning runs again on a conversation it has already pruned, finds
// the same assistant message, and updates the row it wrote last time instead of
// leaving a second copy behind. That is the dedup rule the rest of the store
// already follows — "recent:<path>" and "tool-usage:<tool>" key on the thing
// they describe for the same reason.
func autoSaveMemoryID(sessionID, fact string) string {
	sum := sha256.Sum256([]byte(fact))
	return fmt.Sprintf("auto-saved:%s:%s", sessionID, hex.EncodeToString(sum[:])[:16])
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

		content := extractTextBlockContent(msg.Content)
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
				ID:         autoSaveMemoryID(sessionID, fact),
				Content:    fact,
				Tags:       []string{"auto-saved", "decision", sessionID},
				Importance: importance,
				Source:     "pruning",
			})
			if err != nil {
				// The fact is about to be pruned out of the conversation, so a
				// save that failed is content leaving the process for good.
				// Say so rather than counting it and moving on.
				log.Printf("[warn] failed to auto-save a memory for session %s: %v", sessionID, err)
				continue
			}
			saved++
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