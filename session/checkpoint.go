package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// Checkpoint represents a saved conversation state.
//
// Two different turn counters run side by side in this process, and this record
// is stamped with the one the checkpoint gate actually used. UserTurn counts
// user messages (the engine's Turn.Counter, one per RunTurn); Session.turn
// counts API round-trips, of which a single user message can cost many. The
// gate that fires a checkpoint reads the user-turn counter, so a checkpoint
// stamped with the round-trip counter named a turn that nothing had decided
// anything about — a session checkpointed at user turn 20 recorded 87.
type Checkpoint struct {
	SessionID   string                   `json:"session_id"`
	UserTurn    int                      `json:"user_turn"`    // User-message turn the checkpoint gate fired on
	CreatedAt   time.Time                `json:"created_at"`
	Summary     string                   `json:"summary"`      // Brief summary of conversation so far
	KeyFacts    []string                 `json:"key_facts"`    // Important facts to remember
	Messages    []anthropic.MessageParam `json:"messages"`     // Last N messages (full)
	TotalTokens int                      `json:"total_tokens"` // Total tokens consumed so far
	ModelInfo   string                   `json:"model"`        // Model used
}

// CheckpointConfig configures checkpointing behavior.
//
// There is no retention setting because there is nothing to retain: a session
// writes one checkpoint.json and every later checkpoint overwrites it. This
// used to carry a MaxCheckpoints of 5, enforced by a pruneOldCheckpoints that
// was a bare `return nil` — a retention policy that read as if four older
// checkpoints were being kept, when no second file was ever created. Rotation
// belongs with a reader that needs it; today LoadCheckpoint has no callers.
type CheckpointConfig struct {
	Enabled      bool // Enable checkpointing
	Interval     int  // Create checkpoint every N user turns (default: 20)
	KeepMessages int  // Number of recent messages to keep in checkpoint (default: 30)
}

// DefaultCheckpointConfig returns sensible defaults
func DefaultCheckpointConfig() CheckpointConfig {
	return CheckpointConfig{
		Enabled:      true,
		Interval:     20,
		KeepMessages: 30,
	}
}

// SaveCheckpoint writes the current session state to checkpoint.json beside the
// session log, overwriting any earlier checkpoint.
//
// userTurn is supplied by the caller because the session cannot know it: the
// counter this session owns advances once per API round-trip, and the gate that
// decides when to checkpoint counts user messages. Passing it in keeps the two
// clocks from being confused for each other, which is exactly what the record
// used to do.
func (s *Session) SaveCheckpoint(userTurn int, messages []anthropic.MessageParam, summary string, keyFacts []string) error {
	if s.sessionID == "" {
		return fmt.Errorf("session ID not set")
	}

	cfg := DefaultCheckpointConfig()

	// Determine messages to keep
	messagesToKeep := messages
	if len(messages) > cfg.KeepMessages {
		messagesToKeep = messages[len(messages)-cfg.KeepMessages:]
	}

	// The token totals are written by LogAssistant under the mutex, so they
	// have to be read under it too — a checkpoint runs while the turn that
	// produced those tokens is still logging.
	s.mu.Lock()
	totalTokens := s.totalIn + s.totalOut
	s.mu.Unlock()

	// Create checkpoint
	checkpoint := Checkpoint{
		SessionID:   s.sessionID,
		UserTurn:    userTurn,
		CreatedAt:   time.Now(),
		Summary:     summary,
		KeyFacts:    keyFacts,
		Messages:    messagesToKeep,
		TotalTokens: totalTokens,
		ModelInfo:   s.model,
	}

	// Save to file
	checkpointPath := s.checkpointPath()
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	if err := os.WriteFile(checkpointPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint: %w", err)
	}

	return nil
}

// LoadCheckpoint loads the most recent checkpoint for this session
func (s *Session) LoadCheckpoint() (*Checkpoint, error) {
	checkpointPath := s.checkpointPath()
	
	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No checkpoint exists
		}
		return nil, fmt.Errorf("failed to read checkpoint: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// ShouldCheckpoint determines whether a checkpoint should be created at this
// user turn. The caller must pass the same user-turn counter it later hands to
// SaveCheckpoint, not the session's API round-trip counter.
func ShouldCheckpoint(userTurn int, cfg CheckpointConfig) bool {
	if !cfg.Enabled {
		return false
	}
	return userTurn > 0 && userTurn%cfg.Interval == 0
}

// checkpointPath returns the path to the checkpoint file for this session
func (s *Session) checkpointPath() string {
	sessionDir := filepath.Dir(s.file.Name())
	return filepath.Join(sessionDir, "checkpoint.json")
}

// ListCheckpoints lists all available checkpoints for a session directory
func ListCheckpoints(sessionDir string) ([]string, error) {
	checkpointPath := filepath.Join(sessionDir, "checkpoint.json")
	
	if _, err := os.Stat(checkpointPath); err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to stat checkpoint file %s: %w", checkpointPath, err)
	}

	return []string{checkpointPath}, nil
}

// ExtractKeyFacts extracts important facts from conversation history
// This is a simple heuristic-based extraction; could be enhanced with LLM summarization
func ExtractKeyFacts(messages []anthropic.MessageParam, maxFacts int) []string {
	var facts []string
	
	// Look for messages containing key indicators
	keywordPatterns := []string{
		"important:", "note:", "remember:", "decided to",
		"implemented", "created", "fixed", "changed",
	}

	for _, msg := range messages {
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}

		content := extractTextFromMessage(msg)
		if content == "" {
			continue
		}

		lowerContent := strings.ToLower(content)
		
		// Check for key patterns
		for _, pattern := range keywordPatterns {
			if strings.Contains(lowerContent, pattern) {
				// Extract the sentence containing the pattern
				sentences := splitSentences(lowerContent)
				for _, sentence := range sentences {
					if strings.Contains(sentence, pattern) && len(sentence) > 20 {
						facts = append(facts, sentence)
						if len(facts) >= maxFacts {
							return facts
						}
						break
					}
				}
			}
		}
	}

	return facts
}

// extractTextFromMessage extracts text content from a message
func extractTextFromMessage(msg anthropic.MessageParam) string {
	var parts []string
	for _, block := range msg.Content {
		if block.OfText != nil {
			parts = append(parts, block.OfText.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// splitSentences splits text into sentences
func splitSentences(text string) []string {
	// Simple sentence splitter
	var sentences []string
	var current []rune
	
	for _, r := range text {
		current = append(current, r)
		if r == '.' || r == '!' || r == '?' {
			sentence := string(current)
			if len(sentence) > 10 {
				sentences = append(sentences, sentence)
			}
			current = nil
		}
	}
	
	if len(current) > 10 {
		sentences = append(sentences, string(current))
	}
	
	return sentences
}

// GenerateConversationSummary creates a brief summary of the conversation
// For now this is a simple placeholder; could be enhanced with LLM summarization
func GenerateConversationSummary(messages []anthropic.MessageParam) string {
	if len(messages) == 0 {
		return "Empty conversation"
	}

	userMsgCount := 0
	assistantMsgCount := 0
	toolCallCount := 0

	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleUser {
			userMsgCount++
		} else if msg.Role == anthropic.MessageParamRoleAssistant {
			assistantMsgCount++
		}
		
		// Count tool uses
		for _, block := range msg.Content {
			if block.OfToolUse != nil {
				toolCallCount++
			}
		}
	}

	return fmt.Sprintf("Conversation with %d user messages, %d assistant responses, %d tool calls",
		userMsgCount, assistantMsgCount, toolCallCount)
}
