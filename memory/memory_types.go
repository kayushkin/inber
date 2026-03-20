// Package memory provides persistent, searchable memory across sessions.
package memory

import (
	"database/sql"
	"time"
)

// Memory represents a single persistent memory entry.
type Memory struct {
	ID           string    // unique identifier
	Content      string    // the actual text
	Summary      string    // compressed version (nullable, for future compaction)
	OriginalID   string    // pointer to parent memory if compacted (nullable)
	Tags         []string  // tags for categorization
	Importance   float64   // 0-1, how important this memory is
	AccessCount  int       // how many times it's been retrieved
	LastAccessed time.Time // timestamp of last retrieval
	CreatedAt    time.Time // when it was stored
	Source       string    // "user", "agent", "reflection", "compaction", "system"
	Embedding    []float64 // vector for semantic search
	
	// New fields for context unification
	AlwaysLoad   bool       // if true, always include in context (e.g., identity)
	ExpiresAt    *time.Time // optional expiration (for ephemeral content like recent files)
	Tokens       int        // pre-computed token count for budget management
	
	// Reference fields for lazy loading
	RefType      string     // "memory" (default), "file", "identity", "repo-map", "tools", "web"
	RefTarget    string     // file path, URL, or empty for pure memories
	IsLazy       bool       // if true, load content on-demand instead of from DB
}

// Store handles persistent memory storage via SQLite.
type Store struct {
	db       *sql.DB
	embedder *Embedder
}

// CompactionResult describes what was compacted.
type CompactionResult struct {
	OriginalIDs []string
	NewID       string
	Tags        []string
	Count       int
}

// Session represents a tracked agent session.
type Session struct {
	ID           string
	AgentName    string
	Model        string
	StartedAt    time.Time
	EndedAt      time.Time
	InputTokens  int
	OutputTokens int
	Cost         float64
	Summary      string
	Tags         []string
}