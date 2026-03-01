package context

import (
	"fmt"
	"strings"
	"time"
)

// ConversationSummarizer manages conversation history summarization
type ConversationSummarizer struct {
	store            *Store
	turnsBeforeSummary int // Summarize after this many turns
	turnCount        int
}

// NewConversationSummarizer creates a summarizer that compacts conversation history
func NewConversationSummarizer(store *Store, turnsBeforeSummary int) *ConversationSummarizer {
	if turnsBeforeSummary <= 0 {
		turnsBeforeSummary = 10 // Default: summarize every 10 turns
	}
	return &ConversationSummarizer{
		store:            store,
		turnsBeforeSummary: turnsBeforeSummary,
		turnCount:        0,
	}
}

// RecordTurn increments the turn counter
func (s *ConversationSummarizer) RecordTurn() {
	s.turnCount++
}

// ShouldSummarize returns true if it's time to summarize conversation history
func (s *ConversationSummarizer) ShouldSummarize() bool {
	return s.turnCount > 0 && s.turnCount%s.turnsBeforeSummary == 0
}

// GetConversationChunks retrieves all conversation chunks (user/assistant messages, tool results)
func (s *ConversationSummarizer) GetConversationChunks() []Chunk {
	allChunks := s.store.ListAll()
	var conversation []Chunk
	
	for _, chunk := range allChunks {
		// Include user messages, assistant responses, and tool results
		if chunk.Source == "user" || chunk.Source == "assistant" || chunk.Source == "tool-result" {
			// Skip chunks already marked as summaries
			isAlreadySummary := false
			for _, tag := range chunk.Tags {
				if tag == "conversation-summary" {
					isAlreadySummary = true
					break
				}
			}
			if !isAlreadySummary {
				conversation = append(conversation, chunk)
			}
		}
	}
	
	return conversation
}

// CreateSummaryStub creates a summary stub chunk (for LLM to fill in later)
// This approach lets us defer the actual summarization to when we have LLM access
func (s *ConversationSummarizer) CreateSummaryStub(oldestTurnTime, newestTurnTime time.Time, turnCount int) Chunk {
	stubText := fmt.Sprintf(
		"[Conversation summary needed: %d turns from %s to %s]",
		turnCount,
		oldestTurnTime.Format("15:04:05"),
		newestTurnTime.Format("15:04:05"),
	)
	
	return Chunk{
		ID:        fmt.Sprintf("summary-stub:%d", oldestTurnTime.Unix()),
		Text:      stubText,
		Tags:      []string{"conversation-summary", "stub"},
		Source:    "system",
		CreatedAt: time.Now(),
	}
}

// CompactConversationHistory removes old conversation chunks and creates a summary marker
// The actual summarization should happen via LLM when needed
func (s *ConversationSummarizer) CompactConversationHistory() error {
	chunks := s.GetConversationChunks()
	
	if len(chunks) == 0 {
		return nil
	}
	
	// Keep the last N turns (e.g., last 5 user+assistant pairs = 10 chunks)
	keepLast := 10
	
	if len(chunks) <= keepLast {
		return nil // Not enough to compact
	}
	
	// Find oldest and newest chunks in the "to be summarized" group
	toSummarize := chunks[:len(chunks)-keepLast]
	var oldest, newest time.Time
	
	for i, chunk := range toSummarize {
		if i == 0 || chunk.CreatedAt.Before(oldest) {
			oldest = chunk.CreatedAt
		}
		if i == 0 || chunk.CreatedAt.After(newest) {
			newest = chunk.CreatedAt
		}
	}
	
	// Create summary stub
	summaryStub := s.CreateSummaryStub(oldest, newest, len(toSummarize))
	
	// Remove old chunks
	for _, chunk := range toSummarize {
		s.store.Delete(chunk.ID)
	}
	
	// Add summary stub
	return s.store.Add(summaryStub)
}

// BuildConversationSummaryPrompt creates a prompt for LLM to summarize conversation
func BuildConversationSummaryPrompt(chunks []Chunk) string {
	var builder strings.Builder
	
	builder.WriteString("Please summarize the following conversation turns into a concise summary ")
	builder.WriteString("that captures key decisions, context, and outcomes. Be specific and factual.\n\n")
	
	for i, chunk := range chunks {
		builder.WriteString(fmt.Sprintf("--- Turn %d (%s) ---\n", i+1, chunk.Source))
		builder.WriteString(chunk.Text)
		builder.WriteString("\n\n")
	}
	
	builder.WriteString("Summary:")
	
	return builder.String()
}

// ReplaceSummaryStubWithContent replaces a summary stub with actual summary content
func (s *ConversationSummarizer) ReplaceSummaryStubWithContent(stubID, summaryText string) error {
	// Get the stub
	stub, exists := s.store.Get(stubID)
	if !exists {
		return fmt.Errorf("stub not found: %s", stubID)
	}
	
	// Create real summary chunk
	summary := Chunk{
		ID:        strings.Replace(stubID, "stub:", "", 1), // Remove "stub:" prefix
		Text:      summaryText,
		Tags:      []string{"conversation-summary"},
		Source:    "system",
		CreatedAt: stub.CreatedAt,
	}
	
	// Delete stub, add summary
	s.store.Delete(stubID)
	return s.store.Add(summary)
}
