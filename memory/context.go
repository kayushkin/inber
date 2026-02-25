package memory

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kayushkin/inber/context"
)

// LoadIntoContext loads high-importance recent memories as context chunks.
// This is typically called during session start to populate the agent's working memory.
func LoadIntoContext(memStore *Store, ctxStore *context.Store, limit int, minImportance float64) error {
	if limit <= 0 {
		limit = 20 // default: load top 20 memories
	}
	if minImportance <= 0 {
		minImportance = 0.6 // default: only load fairly important memories
	}

	memories, err := memStore.ListRecent(limit, minImportance)
	if err != nil {
		return fmt.Errorf("failed to list memories: %w", err)
	}

	for _, m := range memories {
		// Create a context chunk from each memory
		chunk := context.Chunk{
			ID:     fmt.Sprintf("memory-%s", m.ID),
			Text:   fmt.Sprintf("[MEMORY from %s]\n%s", m.CreatedAt.Format("2006-01-02"), m.Content),
			Tags:   append([]string{"memory", m.Source}, m.Tags...),
			Source: "memory",
		}
		
		if err := ctxStore.Add(chunk); err != nil {
			// Don't fail the whole load if one memory fails
			continue
		}
	}

	return nil
}

// DefaultMemoryPath returns the default path for the memory database.
// It uses .inber/memory.db in the repo root.
func DefaultMemoryPath(rootDir string) string {
	return filepath.Join(rootDir, ".inber", "memory.db")
}

// OpenOrCreate opens an existing memory store or creates a new one at the default path.
func OpenOrCreate(rootDir string) (*Store, error) {
	dbPath := DefaultMemoryPath(rootDir)
	
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := ensureDir(dir); err != nil {
		return nil, fmt.Errorf("create memory directory: %w", err)
	}
	
	return NewStore(dbPath)
}

// ensureDir creates a directory if it doesn't exist
func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
