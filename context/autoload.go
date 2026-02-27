package context

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AutoLoadConfig configures what context to load automatically
type AutoLoadConfig struct {
	RootDir         string        // Repository root directory
	IdentityFile    string        // Path to agent identity/system prompt file (optional)
	IdentityText    string        // Direct identity text (used if IdentityFile is empty)
	AgentName       string        // Agent name for identity chunk
	RepoMapEnabled  bool          // Whether to build repo map
	RecencyWindow   time.Duration // How far back to look for recent files (e.g., 24h)
	IgnorePatterns  []string      // Patterns to ignore in repo map
}

// DefaultAutoLoadConfig returns sensible defaults
func DefaultAutoLoadConfig(rootDir string) AutoLoadConfig {
	return AutoLoadConfig{
		RootDir:        rootDir,
		AgentName:      "inber",
		RepoMapEnabled: true,
		RecencyWindow:  24 * time.Hour,
		IgnorePatterns: []string{
			"*.log",
			"*.tmp",
			".git/*",
			"vendor/*",
			"node_modules/*",
			".openclaw/*",
			"logs/*",
		},
	}
}

// AutoLoad builds initial context chunks for the agent
// Returns a populated store with identity, repo map, and recent files
func AutoLoad(cfg AutoLoadConfig) (*Store, error) {
	store := NewStore()
	
	// 1. Load agent identity
	if err := loadIdentity(store, cfg); err != nil {
		return nil, fmt.Errorf("failed to load identity: %w", err)
	}
	
	// 2. Memory usage instructions (always loaded; tools may or may not be present)
	loadMemoryInstructions(store)

	// 3. Build repo map (was 2)
	if cfg.RepoMapEnabled {
		if err := loadRepoMap(store, cfg); err != nil {
			return nil, fmt.Errorf("failed to build repo map: %w", err)
		}
	}
	
	// 4. Find recently modified files
	if cfg.RecencyWindow > 0 {
		if err := loadRecentFiles(store, cfg); err != nil {
			// Don't fail if recency detection fails, just log a warning
			fmt.Fprintf(os.Stderr, "warning: failed to detect recent files: %v\n", err)
		}
	}
	
	return store, nil
}

// loadIdentity loads agent identity/purpose into the store
func loadIdentity(store *Store, cfg AutoLoadConfig) error {
	var identityText string
	
	// Try to load from file first
	if cfg.IdentityFile != "" {
		content, err := os.ReadFile(cfg.IdentityFile)
		if err != nil {
			return err
		}
		identityText = string(content)
	} else if cfg.IdentityText != "" {
		identityText = cfg.IdentityText
	} else {
		// Default identity
		identityText = fmt.Sprintf("You are %s, a helpful coding assistant with access to file operations and shell commands.", cfg.AgentName)
	}
	
	// Create identity chunk
	chunk := Chunk{
		ID:     "identity",
		Text:   identityText,
		Tags:   []string{"identity", "always", "system"},
		Source: "system",
	}
	
	return store.Add(chunk)
}

// loadRepoMap builds and stores the repository structure map
func loadRepoMap(store *Store, cfg AutoLoadConfig) error {
	repoMap, err := BuildRepoMap(cfg.RootDir, cfg.IgnorePatterns)
	if err != nil {
		return err
	}
	
	// Create repo map chunk
	chunk := Chunk{
		ID:     "repo-map",
		Text:   repoMap,
		Tags:   []string{"repo-map", "structure", "always", "code"},
		Source: "system",
	}
	
	return store.Add(chunk)
}

// loadRecentFiles detects and stores information about recently modified files
func loadRecentFiles(store *Store, cfg AutoLoadConfig) error {
	recentFiles, err := FindRecentlyModified(cfg.RootDir, cfg.RecencyWindow)
	if err != nil {
		return err
	}
	
	if len(recentFiles) == 0 {
		return nil // No recent files, nothing to add
	}
	
	summary := FormatRecentFiles(recentFiles)
	
	// Create recent files chunk
	chunk := Chunk{
		ID:     "recent-files",
		Text:   summary,
		Tags:   []string{"recent", "high-priority", "files"},
		Source: "system",
	}
	
	// Also tag with individual file names for better matching
	for _, file := range recentFiles {
		filename := filepath.Base(file.RelativePath)
		chunk.Tags = append(chunk.Tags, filename)
	}
	
	return store.Add(chunk)
}

// MemoryInstructions is the default system prompt text for memory tool usage.
const MemoryInstructions = `You have persistent memory across sessions via these tools:
- memory_search: Search your memories before answering questions about past work, preferences, or decisions
- memory_save: Save important information — decisions made, user preferences, project context, lessons learned
- memory_forget: Mark outdated or incorrect memories as forgotten

Guidelines:
- Search memory at the start of conversations about ongoing projects
- Save key decisions and their reasoning
- Save user preferences when explicitly stated
- Don't save trivial or temporary information
- Review and forget outdated memories when you notice them`

// loadMemoryInstructions adds a context chunk with memory usage guidelines.
func loadMemoryInstructions(store *Store) {
	store.Add(Chunk{
		ID:     "memory-instructions",
		Text:   MemoryInstructions,
		Tags:   []string{"identity", "always", "memory"},
		Source: "system",
	})
}

// LoadProjectContext is a convenience function that loads context from project markers
// It looks for .openclaw/AGENTS.md, .openclaw/TOOLS.md, .inber/project.md, README.md, etc.
func LoadProjectContext(store *Store, rootDir string) error {
	projectFiles := []struct {
		Path string
		Tags []string
	}{
		{".openclaw/AGENTS.md", []string{"agents", "architecture", "always", "docs"}},
		{".openclaw/TOOLS.md", []string{"tools", "setup", "docs"}},
		{".inber/project.md", []string{"project", "always", "config", "deploy", "tests"}},
		{"README.md", []string{"readme", "docs", "overview"}},
		{"DESIGN.md", []string{"design", "architecture", "docs"}},
		{"ARCHITECTURE.md", []string{"architecture", "docs"}},
	}
	
	for _, pf := range projectFiles {
		fullPath := filepath.Join(rootDir, pf.Path)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			// File doesn't exist, skip
			continue
		}
		
		chunk := Chunk{
			ID:     fmt.Sprintf("project-%s", filepath.Base(pf.Path)),
			Text:   string(content),
			Tags:   pf.Tags,
			Source: "system",
		}
		
		if err := store.Add(chunk); err != nil {
			return err
		}
	}
	
	return nil
}
