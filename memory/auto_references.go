package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AutoReferenceConfig controls when to auto-create references after tool calls.
type AutoReferenceConfig struct {
	// CreateOnReadFile creates file references after read_file tool calls
	CreateOnReadFile bool
	
	// CreateOnRepoMap creates repo-map references after repo_map tool calls
	CreateOnRepoMap bool
	
	// CreateOnRecent creates recent-files references after recent_files tool calls
	CreateOnRecent bool
	
	// MinFileSize is the minimum file size (bytes) to create a reference for
	MinFileSize int
	
	// ExpiresAfter is how long repo-map/recent-file references last
	ExpiresAfter time.Duration
}

// DefaultAutoReferenceConfig returns sensible defaults.
func DefaultAutoReferenceConfig() AutoReferenceConfig {
	return AutoReferenceConfig{
		CreateOnReadFile: true,
		CreateOnRepoMap:  true,
		CreateOnRecent:   true,
		MinFileSize:      100, // Don't reference tiny files
		ExpiresAfter:     10 * time.Minute,
	}
}

// AutoReferenceManager handles automatic reference creation after tool calls.
type AutoReferenceManager struct {
	store  *Store
	config AutoReferenceConfig
	
	// repoRoot is used to make file paths relative
	repoRoot string
}

// NewAutoReferenceManager creates a manager for auto-creating references.
func NewAutoReferenceManager(store *Store, repoRoot string, config AutoReferenceConfig) *AutoReferenceManager {
	return &AutoReferenceManager{
		store:    store,
		config:   config,
		repoRoot: repoRoot,
	}
}

// OnToolResult is a hook that gets called after tool execution.
// It creates references based on the tool name, input, and result.
func (m *AutoReferenceManager) OnToolResult(toolID, name, inputJSON, output string) error {
	switch name {
	case "read_file":
		if m.config.CreateOnReadFile {
			return m.createFileReference(toolID, inputJSON)
		}
	case "repo_map":
		if m.config.CreateOnRepoMap {
			return m.createRepoMapReference(toolID, output)
		}
	case "recent_files":
		if m.config.CreateOnRecent {
			return m.createRecentFilesReference(toolID, output)
		}
	}
	return nil
}

// createFileReference creates a lazy-loaded reference to a file after read_file.
func (m *AutoReferenceManager) createFileReference(toolID, inputJSON string) error {
	// Parse tool input to extract file path
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return fmt.Errorf("failed to parse read_file input: %w", err)
	}
	
	filePath := input.Path
	if filePath == "" {
		return fmt.Errorf("no file path in read_file input")
	}
	
	// Make path relative to repo root
	relPath, err := filepath.Rel(m.repoRoot, filePath)
	if err != nil {
		relPath = filePath // Use absolute if relative fails
	}
	
	// Check file size
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}
	
	if info.Size() < int64(m.config.MinFileSize) {
		return nil // Too small to reference
	}
	
	// Count lines for summary
	lineCount := countLines(filePath)
	
	summary := fmt.Sprintf("File %s (%d lines, read at %s)", 
		relPath, lineCount, time.Now().Format("15:04"))
	
	// Create lazy reference
	mem := Memory{
		ID:         uuid.New().String(),
		Content:    "", // Empty - lazy loaded
		Summary:    summary,
		Tags:       []string{"file", "read-file", filepath.Base(filePath), filepath.Ext(filePath)},
		RefType:    "file",
		RefTarget:  relPath,
		IsLazy:     true,
		Importance: 0.4, // Medium-low importance
		CreatedAt:  time.Now(),
		Tokens:     int(info.Size() / 4), // Estimate: 4 bytes per token
		Source:     "auto-reference", // Add source field
	}
	
	return m.store.Save(mem)
}

// createRepoMapReference creates a reference to repo map output.
func (m *AutoReferenceManager) createRepoMapReference(toolID, output string) error {
	lines := strings.Split(output, "\n")
	packageCount := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "pkg ") || 
		   strings.HasPrefix(strings.TrimSpace(line), "package ") {
			packageCount++
		}
	}
	
	summary := fmt.Sprintf("Repository structure (%d packages, generated %s)", 
		packageCount, time.Now().Format("15:04"))
	
	mem := Memory{
		ID:         "repo-map-" + time.Now().Format("20060102-150405"),
		Content:    output, // Store repo map content (not lazy - it's generated, not on disk)
		Summary:    summary,
		Tags:       []string{"repo-map", "structure", "code-introspection"},
		RefType:    "repo-map",
		IsLazy:     false, // Content stored because there's no file to read from
		Importance: 0.3,   // Low importance - refreshes frequently
		ExpiresAt:  timePtr(time.Now().Add(m.config.ExpiresAfter)),
		CreatedAt:  time.Now(),
		Tokens:     len(output) / 4,
		Source:     "auto-reference",
	}
	
	return m.store.Save(mem)
}

// createRecentFilesReference creates a reference to recent files output.
func (m *AutoReferenceManager) createRecentFilesReference(toolID, output string) error {
	lines := strings.Split(output, "\n")
	fileCount := 0
	for _, line := range lines {
		if strings.Contains(line, "ago)") || strings.Contains(line, "modified") {
			fileCount++
		}
	}
	
	summary := fmt.Sprintf("Recently modified files (%d files)", fileCount)
	
	mem := Memory{
		ID:         "recent-files-" + time.Now().Format("20060102-150405"),
		Content:    output,
		Summary:    summary,
		Tags:       []string{"recent-files", "code-introspection"},
		RefType:    "recent",
		IsLazy:     false,
		Importance: 0.3,
		ExpiresAt:  timePtr(time.Now().Add(m.config.ExpiresAfter)),
		CreatedAt:  time.Now(),
		Tokens:     len(output) / 4,
		Source:     "auto-reference",
	}
	
	return m.store.Save(mem)
}

// CreateIdentityReferences creates lazy references to identity files on session start.
func (m *AutoReferenceManager) CreateIdentityReferences(identityPath, soulPath, userPath string) error {
	refs := []struct {
		path    string
		refType string
		tags    []string
	}{
		{identityPath, "identity", []string{"identity", "config", "always-load"}},
		{soulPath, "soul", []string{"soul", "config", "always-load"}},
		{userPath, "user", []string{"user", "config", "always-load"}},
	}
	
	for _, ref := range refs {
		if ref.path == "" || !fileExists(ref.path) {
			continue
		}
		
		// Make path relative to repo root
		relPath := ref.path
		if m.repoRoot != "" {
			if rel, err := filepath.Rel(m.repoRoot, ref.path); err == nil {
				relPath = rel
			}
		}
		
		info, err := os.Stat(ref.path)
		if err != nil {
			continue
		}
		
		summary := fmt.Sprintf("%s config file (%s, %d bytes)", 
			strings.Title(ref.refType), relPath, info.Size())
		
		mem := Memory{
			ID:         ref.refType + "-config",
			Content:    "", // Lazy loaded
			Summary:    summary,
			Tags:       ref.tags,
			RefType:    ref.refType,
			RefTarget:  ref.path,
			IsLazy:     true,
			AlwaysLoad: true,  // Identity files always load
			Importance: 1.0,   // Highest importance
			CreatedAt:  time.Now(),
			Tokens:     int(info.Size()) / 4,
		}
		
		if err := m.store.Save(mem); err != nil {
			return fmt.Errorf("failed to save %s reference: %w", ref.refType, err)
		}
	}
	
	return nil
}

// CreateFileReferenceFromPath creates a file reference with the actual file path.
// This is meant to be called from a hook that has access to tool input.
func (m *AutoReferenceManager) CreateFileReferenceFromPath(path string) error {
	if !fileExists(path) {
		return fmt.Errorf("file not found: %s", path)
	}
	
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	
	if info.Size() < int64(m.config.MinFileSize) {
		return nil // Too small to reference
	}
	
	// Make path relative to repo root
	relPath := path
	if m.repoRoot != "" {
		if rel, err := filepath.Rel(m.repoRoot, path); err == nil {
			relPath = rel
		}
	}
	
	ext := filepath.Ext(path)
	tags := []string{"file", "lazy-loaded"}
	if ext != "" {
		tags = append(tags, "ext:"+ext)
	}
	
	// Detect file type
	dir := filepath.Dir(relPath)
	if strings.Contains(dir, "test") || strings.HasSuffix(path, "_test.go") {
		tags = append(tags, "test")
	}
	
	summary := fmt.Sprintf("%s (%d lines)", relPath, countLines(path))
	
	// Generate a stable ID from the file path
	id := "file:" + strings.ReplaceAll(relPath, "/", ":")
	
	mem := Memory{
		ID:         id,
		Content:    "", // Lazy loaded
		Summary:    summary,
		Tags:       tags,
		RefType:    "file",
		RefTarget:  path,
		IsLazy:     true,
		Importance: 0.5, // Medium importance
		CreatedAt:  time.Now(),
		Tokens:     int(info.Size()) / 4,
	}
	
	return m.store.Save(mem)
}

// ParseReadFileInput extracts the file path from read_file tool input JSON.
func ParseReadFileInput(inputJSON string) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return "", err
	}
	return input.Path, nil
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// countLines counts the number of lines in a file.
func countLines(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return len(strings.Split(string(content), "\n"))
}

func timePtr(t time.Time) *time.Time {
	return &t
}
