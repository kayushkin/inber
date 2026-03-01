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

// EstimateTokens provides a simple token count approximation
// Uses the rule: ~4 characters per token (conservative estimate)
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return len(text) / 4
}
