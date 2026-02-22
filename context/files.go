package context

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileLoader loads files from a workspace directory as chunks
type FileLoader struct {
	workspaceDir string
	tagger       Tagger
	ignoreList   *IgnoreList
}

// IgnoreList holds .gitignore patterns
type IgnoreList struct {
	patterns []string
}

// NewFileLoader creates a new file loader for a workspace directory
func NewFileLoader(workspaceDir string, tagger Tagger) (*FileLoader, error) {
	absPath, err := filepath.Abs(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("invalid workspace path: %w", err)
	}
	
	ignoreList, err := loadGitignore(absPath)
	if err != nil {
		// If no .gitignore, just use empty list
		ignoreList = &IgnoreList{patterns: []string{}}
	}
	
	return &FileLoader{
		workspaceDir: absPath,
		tagger:       tagger,
		ignoreList:   ignoreList,
	}, nil
}

// LoadFiles scans the workspace and returns chunks for all files
func (fl *FileLoader) LoadFiles() ([]Chunk, error) {
	var chunks []Chunk
	
	err := filepath.WalkDir(fl.workspaceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		
		// Get relative path
		relPath, err := filepath.Rel(fl.workspaceDir, path)
		if err != nil {
			return err
		}
		
		// Skip root directory
		if relPath == "." {
			return nil
		}
		
		// Skip hidden directories and .git
		if d.IsDir() {
			baseName := filepath.Base(path)
			if strings.HasPrefix(baseName, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		
		// Skip if ignored
		if fl.ignoreList.IsIgnored(relPath) {
			return nil
		}
		
		// Skip hidden files
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		
		// Try to load the file
		chunk, err := fl.loadFile(path, relPath)
		if err != nil {
			// Skip files we can't read (binary, permission denied, etc.)
			return nil
		}
		
		chunks = append(chunks, chunk)
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("error walking workspace: %w", err)
	}
	
	return chunks, nil
}

// loadFile loads a single file as a chunk
func (fl *FileLoader) loadFile(absPath, relPath string) (Chunk, error) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return Chunk{}, err
	}
	
	// Skip binary files (heuristic: contains null bytes)
	if isBinary(content) {
		return Chunk{}, fmt.Errorf("binary file")
	}
	
	text := string(content)
	tokens := EstimateTokens(text)
	
	// Generate tags
	tags := fl.generateFileTags(relPath, text)
	
	return Chunk{
		ID:        "file:" + relPath,
		Text:      text,
		Tokens:    tokens,
		Tags:      tags,
		Source:    "file",
		CreatedAt: time.Now(),
	}, nil
}

// generateFileTags creates tags for a file based on its path and content
func (fl *FileLoader) generateFileTags(relPath, content string) []string {
	tags := []string{"file"}
	
	filename := filepath.Base(relPath)
	ext := strings.TrimPrefix(filepath.Ext(filename), ".")
	
	// Add filename tag
	tags = append(tags, "filename:"+filename)
	
	// Add extension tag
	if ext != "" {
		tags = append(tags, ext)
	}
	
	// Categorize by file type
	switch ext {
	case "go":
		tags = append(tags, "code")
		// Check if test file
		if strings.HasSuffix(filename, "_test.go") {
			tags = append(tags, "test")
		}
	case "py":
		tags = append(tags, "code")
		if strings.HasPrefix(filename, "test_") || strings.Contains(filename, "_test.py") {
			tags = append(tags, "test")
		}
	case "js", "ts", "jsx", "tsx":
		tags = append(tags, "code")
		if strings.Contains(filename, ".test.") || strings.Contains(filename, ".spec.") {
			tags = append(tags, "test")
		}
	case "java", "c", "cpp", "cc", "h", "hpp", "rs", "rb", "php", "swift", "kt", "cs":
		tags = append(tags, "code")
		if strings.HasPrefix(filename, "test") || strings.Contains(strings.ToLower(filename), "test") {
			tags = append(tags, "test")
		}
	case "toml", "yaml", "yml", "json", "ini", "conf", "config":
		tags = append(tags, "config")
	case "md", "txt", "rst":
		tags = append(tags, "doc")
	case "html", "css", "scss", "sass", "less":
		tags = append(tags, "code")
	case "sh", "bash", "zsh", "fish":
		tags = append(tags, "code", "script")
	}
	
	// Special filenames
	lowerName := strings.ToLower(filename)
	if lowerName == "readme.md" || lowerName == "readme" {
		tags = append(tags, "readme", "doc")
	}
	if lowerName == "makefile" || lowerName == "dockerfile" {
		tags = append(tags, "config")
	}
	
	// Use tagger for content-based tags
	if fl.tagger != nil {
		contentTags := fl.tagger.Tag(content, "file")
		tags = append(tags, contentTags...)
	}
	
	return deduplicateTags(tags)
}

// LoadAndUpdate loads files and updates them in the store
// Returns the number of files loaded
func (fl *FileLoader) LoadAndUpdate(store *Store) (int, error) {
	chunks, err := fl.LoadFiles()
	if err != nil {
		return 0, err
	}
	
	for _, chunk := range chunks {
		if err := store.Add(chunk); err != nil {
			return 0, fmt.Errorf("error adding chunk %s: %w", chunk.ID, err)
		}
	}
	
	return len(chunks), nil
}

// loadGitignore reads .gitignore from the workspace
func loadGitignore(workspaceDir string) (*IgnoreList, error) {
	gitignorePath := filepath.Join(workspaceDir, ".gitignore")
	
	file, err := os.Open(gitignorePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	var patterns []string
	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		patterns = append(patterns, line)
	}
	
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	
	return &IgnoreList{patterns: patterns}, nil
}

// IsIgnored checks if a path should be ignored based on .gitignore patterns
func (il *IgnoreList) IsIgnored(relPath string) bool {
	for _, pattern := range il.patterns {
		if matchPattern(pattern, relPath) {
			return true
		}
	}
	return false
}

// matchPattern performs simple glob matching for .gitignore patterns
func matchPattern(pattern, path string) bool {
	// Simple implementation - handles basic patterns
	// Does not support full .gitignore spec (negation, complex globs, etc.)
	
	// Directory pattern (ends with /)
	if strings.HasSuffix(pattern, "/") {
		dirPattern := strings.TrimSuffix(pattern, "/")
		return strings.HasPrefix(path, dirPattern+"/") || path == dirPattern
	}
	
	// Exact match
	if pattern == path {
		return true
	}
	
	// Wildcard matching
	if strings.Contains(pattern, "*") {
		matched, _ := filepath.Match(pattern, path)
		if matched {
			return true
		}
		// Also check if any path component matches
		matched, _ = filepath.Match(pattern, filepath.Base(path))
		return matched
	}
	
	// Check if path starts with pattern (prefix match)
	if strings.HasPrefix(path, pattern+"/") {
		return true
	}
	
	// Check if any directory component matches
	parts := strings.Split(path, string(filepath.Separator))
	for _, part := range parts {
		if part == pattern {
			return true
		}
	}
	
	return false
}

// isBinary checks if content contains null bytes (heuristic for binary files)
func isBinary(content []byte) bool {
	// Check first 512 bytes for null bytes
	checkLen := 512
	if len(content) < checkLen {
		checkLen = len(content)
	}
	
	for i := 0; i < checkLen; i++ {
		if content[i] == 0 {
			return true
		}
	}
	
	return false
}
