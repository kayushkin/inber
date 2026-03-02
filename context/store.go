package context

import (
	"fmt"
	"sync"
	"time"
)

// Chunk represents a piece of context with metadata
type Chunk struct {
	ID        string
	Text      string
	Tokens    int        // pre-counted token size
	Tags      []string   // e.g. "identity", "error-log", "deploy", "ssh"
	Source    string     // "user", "assistant", "tool-result", "memory", "system"
	CreatedAt time.Time
	ExpiresAt *time.Time // optional TTL
	IsStub    bool       // if true, this is a lazy-loadable reference, not full content
	StubPath  string     // file path for lazy loading (if IsStub=true)
}

// Store is a thread-safe in-memory chunk store
type Store struct {
	mu     sync.RWMutex
	chunks map[string]Chunk
}

// NewStore creates a new chunk store
func NewStore() *Store {
	return &Store{
		chunks: make(map[string]Chunk),
	}
}

// Add adds or updates a chunk in the store
func (s *Store) Add(chunk Chunk) error {
	if chunk.ID == "" {
		return fmt.Errorf("chunk ID cannot be empty")
	}
	
	// If tokens not set, estimate them
	if chunk.Tokens == 0 {
		chunk.Tokens = EstimateTokens(chunk.Text)
	}
	
	// Set created time if not set
	if chunk.CreatedAt.IsZero() {
		chunk.CreatedAt = time.Now()
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.chunks[chunk.ID] = chunk
	return nil
}

// Get retrieves a chunk by ID
func (s *Store) Get(id string) (Chunk, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	chunk, ok := s.chunks[id]
	if !ok {
		return Chunk{}, false
	}
	
	// Check if expired
	if chunk.ExpiresAt != nil && time.Now().After(*chunk.ExpiresAt) {
		return Chunk{}, false
	}
	
	return chunk, true
}

// Delete removes a chunk by ID
func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	_, ok := s.chunks[id]
	if ok {
		delete(s.chunks, id)
	}
	return ok
}

// ListByTags returns all non-expired chunks that have at least one of the given tags
func (s *Store) ListByTags(tags []string) []Chunk {
	if len(tags) == 0 {
		return s.ListAll()
	}
	
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	now := time.Now()
	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}
	
	var result []Chunk
	for _, chunk := range s.chunks {
		// Skip expired chunks
		if chunk.ExpiresAt != nil && now.After(*chunk.ExpiresAt) {
			continue
		}
		
		// Check if chunk has any matching tag
		for _, chunkTag := range chunk.Tags {
			if tagSet[chunkTag] {
				result = append(result, chunk)
				break
			}
		}
	}
	
	return result
}

// ListAll returns all non-expired chunks
func (s *Store) ListAll() []Chunk {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	now := time.Now()
	var result []Chunk
	
	for _, chunk := range s.chunks {
		// Skip expired chunks
		if chunk.ExpiresAt != nil && now.After(*chunk.ExpiresAt) {
			continue
		}
		result = append(result, chunk)
	}
	
	return result
}

// Count returns the number of non-expired chunks in the store
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	now := time.Now()
	count := 0
	
	for _, chunk := range s.chunks {
		if chunk.ExpiresAt == nil || now.Before(*chunk.ExpiresAt) {
			count++
		}
	}
	
	return count
}

// EstimateTokens provides a token count approximation.
// Uses ~3 characters per token for general text, which accounts for:
// - English text averages ~3.5 chars/token with Claude's tokenizer
// - Code tends to be ~3 chars/token (more symbols, short identifiers)
// - JSON structure overhead in API calls isn't counted here but is
//   accounted for by EstimateMessageOverhead and EstimateToolSchemaTokens
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	// Base estimate: ~3 chars per token
	tokens := (len(text) + 2) / 3
	return tokens
}

// EstimateMessageOverhead returns the approximate token overhead for a single
// message in the Anthropic API (role markers, JSON framing, content blocks).
// Each message adds roughly 10-15 tokens of structure.
func EstimateMessageOverhead() int {
	return 12
}

// EstimateToolSchemaTokens estimates tokens used by a tool's JSON schema
// definition. Tool schemas are sent with every request and can be significant.
// A typical tool with 3-5 parameters uses ~100-200 tokens.
func EstimateToolSchemaTokens(name, description string, paramCount int) int {
	// Base overhead for tool JSON structure
	base := 30
	// Name and description
	base += EstimateTokens(name) + EstimateTokens(description)
	// Each parameter adds ~25-40 tokens (name, type, description, required)
	base += paramCount * 35
	return base
}
