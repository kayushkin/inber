package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

// loadLazyContent loads content for lazy-loaded references from their source
func (s *Store) loadLazyContent(m *Memory) error {
	switch m.RefType {
	case "file":
		return loadFileContent(m)
	case "identity":
		return loadIdentityContent(m)
	case "repo-map":
		// Repo maps should be generated fresh, not loaded
		return fmt.Errorf("repo-map content should be generated via tool, not loaded from memory")
	case "tools":
		// Tool registry should be generated fresh
		return fmt.Errorf("tools content should be generated via tool, not loaded from memory")
	case "web":
		return fmt.Errorf("web content loading not yet implemented")
	default:
		// "memory" type - content should already be in DB
		return nil
	}
}

// loadFileContent reads file from disk
func loadFileContent(m *Memory) error {
	if m.RefTarget == "" {
		return fmt.Errorf("file reference missing ref_target path")
	}
	
	data, err := os.ReadFile(m.RefTarget)
	if err != nil {
		return fmt.Errorf("read file %s: %w", m.RefTarget, err)
	}
	
	m.Content = string(data)
	m.Tokens = len(m.Content) / 4 // rough estimate
	return nil
}

// loadIdentityContent reads identity file (.inber/identity.md, soul.md, user.md)
func loadIdentityContent(m *Memory) error {
	if m.RefTarget == "" {
		return fmt.Errorf("identity reference missing ref_target path")
	}
	
	// Support both absolute and relative paths
	path := m.RefTarget
	if !filepath.IsAbs(path) {
		// Try .inber/ directory
		path = filepath.Join(".inber", m.RefTarget)
	}
	
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read identity file %s: %w", path, err)
	}
	
	m.Content = string(data)
	m.Tokens = len(m.Content) / 4
	return nil
}
