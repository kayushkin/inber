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

		fragments := substantialLeadingFragments(content, 3)
		if len(fragments) == 0 {
			// This loop only reaches messages that are about to be
			// truncated, so an extraction that found nothing is content
			// leaving the process in both directions at once: the detail
			// goes out of the live conversation and no memory row replaces
			// it. Say so rather than moving on in silence — the same rule
			// the failed Save below already follows.
			//
			// Whether an empty extraction should also BLOCK the truncation
			// in manage.go, rather than only report it, is open and is not
			// decided here: blocking makes pruning refuse to free the tokens
			// the caller asked for. Todo 86347015-9924-40f4-99db-1d79c1e767ca.
			log.Printf("[warn] auto-save extracted nothing from a %d-char assistant message for session %s; it is still being truncated, so the detail is lost", len(content), sessionID)
			continue
		}

		fact := strings.Join(fragments, " ")
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

// substantialLeadingFragments splits text on sentence terminators, looks at
// only the first maxFragmentsScanned pieces, and returns those longer than 20
// bytes once trimmed.
//
// The name says "fragments" and not "sentences" because FieldsFunc splits on
// every '.', so "1. First item." and "v2.1.4" are three and four pieces, not
// one. It says "scanned" because the budget is spent on pieces this function
// then rejects: a message opening with a numbered list burns the budget on
// "1" and "2" and can return nothing at all. The old name, extractKeySentences,
// promised "the first N sentences from text" and delivered none of those three
// words; the caller reads an empty result as "nothing worth keeping" when it
// often means "the budget went on digits".
//
// The 20-byte floor disagrees with the 10 in manage_text_utils.go, so a
// fragment of 11-20 bytes is kept by the truncator and refused here. Which
// threshold is right, and whether these two functions are one question or two,
// is open — todo 86347015-9924-40f4-99db-1d79c1e767ca. Nothing about the
// behaviour below has changed.
func substantialLeadingFragments(text string, maxFragmentsScanned int) []string {
	fragments := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})

	var result []string
	for i, fragment := range fragments {
		if i >= maxFragmentsScanned {
			break
		}
		fragment = strings.TrimSpace(fragment)
		if len(fragment) > 20 { // Skip very short fragments
			result = append(result, fragment)
		}
	}

	return result
}