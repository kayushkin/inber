// Package memory provides persistent, searchable memory across sessions.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MemoryStore defines the interface for persistent memory storage.
// Different backends can implement this interface (SQLite, Redis, filesystem, in-memory, etc.).
type MemoryStore interface {
	// Core memory operations
	Save(m Memory) error
	Get(id string) (*Memory, error)
	Search(query string, limit int) ([]Memory, error)
	Forget(id string) error
	
	// Memory management
	DecayImportance() error
	ListRecent(limit int, minImportance float64) ([]Memory, error)
	Compact(minAge time.Duration, minCount int) ([]CompactionResult, error)
	
	// Context building and session preparation
	BuildContext(req BuildContextRequest) ([]Memory, int, error)
	PrepareSession(cfg PrepareSessionConfig) error
	
	// Tool registry and usage tracking
	LoadToolRegistry(tools []ToolMetadata) error
	UpdateToolUsageSummary(toolName, summary string, ttlSeconds int64) error
	
	// Session tracking
	SaveSession(sess Session) error
	TrackMemoryUsage(memoryID, sessionID string, turnNumber int, usageType string) error
	
	// Lifecycle
	Close() error
}

// OpenOrCreate opens an existing memory store or creates a new one.
//
// If AGENT_STORE_PATH is set, uses a RemoteStore backed by agent-store's database,
// with a local SQLite for search/context operations not yet supported by agent-store.
// The AGENT_SLUG env var controls which agent's memories to access (default: "inber").
//
// Otherwise, falls back to a pure local SQLite store.
func OpenOrCreate(rootDir string) (MemoryStore, error) {
	localDBPath := DefaultMemoryPath(rootDir)
	if err := os.MkdirAll(filepath.Dir(localDBPath), 0755); err != nil {
		return nil, fmt.Errorf("create memory directory: %w", err)
	}

	// Check for agent-store integration
	if agentStorePath := os.Getenv("AGENT_STORE_PATH"); agentStorePath != "" {
		agentSlug := os.Getenv("AGENT_SLUG")
		if agentSlug == "" {
			agentSlug = "inber"
		}
		return NewRemoteStore(agentStorePath, localDBPath, agentSlug)
	}

	// Future: AGENT_STORE_URL for HTTP client
	// if url := os.Getenv("AGENT_STORE_URL"); url != "" {
	//     return NewHTTPStore(url, localDBPath, agentSlug)
	// }

	return NewStore(localDBPath)
}

// NewSQLiteStore creates a new SQLite-backed memory store at the given path.
// This is the same as NewStore but with a more descriptive name.
func NewSQLiteStore(dbPath string) (MemoryStore, error) {
	return NewStore(dbPath)
}