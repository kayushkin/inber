package context

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FileImportance represents a file's importance score
type FileImportance struct {
	Path            string
	RelativePath    string
	Score           float64 // 0.0 to 1.0
	RecencyScore    float64
	SizeScore       float64
	FrequencyScore  float64
	DependencyScore float64
}

// ImportanceScorer calculates file importance based on multiple factors
type ImportanceScorer struct {
	rootDir        string
	recencyWindow  time.Duration
	gitAvailable   bool
	dependencyMap  map[string][]string // file -> files that import it
}

// NewImportanceScorer creates a new importance scorer
func NewImportanceScorer(rootDir string, recencyWindow time.Duration) *ImportanceScorer {
	scorer := &ImportanceScorer{
		rootDir:       rootDir,
		recencyWindow: recencyWindow,
		dependencyMap: make(map[string][]string),
	}
	
	// Check if git is available
	gitDir := filepath.Join(rootDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		scorer.gitAvailable = true
	}
	
	return scorer
}

// ScoreFile calculates importance score for a single file
func (s *ImportanceScorer) ScoreFile(filePath string) (FileImportance, error) {
	relPath, err := filepath.Rel(s.rootDir, filePath)
	if err != nil {
		return FileImportance{}, err
	}
	
	info, err := os.Stat(filePath)
	if err != nil {
		return FileImportance{}, err
	}
	
	result := FileImportance{
		Path:         filePath,
		RelativePath: relPath,
	}
	
	// 1. Recency score (0.0 to 1.0)
	result.RecencyScore = s.calculateRecencyScore(info.ModTime())
	
	// 2. Size score (0.0 to 1.0, smaller = better for context)
	result.SizeScore = s.calculateSizeScore(info.Size())
	
	// 3. Frequency score (how often file is modified)
	result.FrequencyScore = s.calculateFrequencyScore(filePath)
	
	// 4. Dependency score (how many files import this)
	result.DependencyScore = s.calculateDependencyScore(relPath)
	
	// Combined weighted score
	result.Score = (
		result.RecencyScore*0.4 + // Recency is most important
		result.SizeScore*0.2 + // Smaller files preferred
		result.FrequencyScore*0.2 + // Frequently modified = active development
		result.DependencyScore*0.2) // Important if many files depend on it
	
	return result, nil
}

// ScoreRecentFiles scores all recently modified files
func (s *ImportanceScorer) ScoreRecentFiles(since time.Duration) ([]FileImportance, error) {
	// First, build dependency map for Go files
	if err := s.buildDependencyMap(); err != nil {
		// Non-fatal, just log and continue
		fmt.Fprintf(os.Stderr, "Warning: failed to build dependency map: %v\n", err)
	}
	
	// Find recently modified files
	recentFiles, err := FindRecentlyModified(s.rootDir, since)
	if err != nil {
		return nil, err
	}
	
	var scored []FileImportance
	for _, file := range recentFiles {
		importance, err := s.ScoreFile(file.Path)
		if err != nil {
			continue // Skip files we can't score
		}
		scored = append(scored, importance)
	}
	
	return scored, nil
}

// calculateRecencyScore scores based on modification time
// More recent = higher score (1.0 = just now, 0.0 = older than recencyWindow)
func (s *ImportanceScorer) calculateRecencyScore(modTime time.Time) float64 {
	age := time.Since(modTime)
	
	if age <= 0 {
		return 1.0 // Future or current
	}
	
	if age >= s.recencyWindow {
		return 0.0 // Too old
	}
	
	// Linear decay from 1.0 to 0.0 over recencyWindow
	return 1.0 - (float64(age) / float64(s.recencyWindow))
}

// calculateSizeScore scores based on file size
// Smaller files are preferred (easier to include in context)
// Uses logarithmic scale: 0-1KB = 1.0, 1KB-10KB = 0.8, 10KB-100KB = 0.5, >100KB = 0.2
func (s *ImportanceScorer) calculateSizeScore(size int64) float64 {
	kb := float64(size) / 1024.0
	
	if kb < 1 {
		return 1.0
	} else if kb < 10 {
		return 0.8
	} else if kb < 100 {
		return 0.5
	} else if kb < 1000 {
		return 0.3
	} else {
		return 0.1 // Very large files get low score
	}
}

// calculateFrequencyScore scores based on git commit frequency
// More commits in recent history = higher score
func (s *ImportanceScorer) calculateFrequencyScore(filePath string) float64 {
	if !s.gitAvailable {
		return 0.5 // Neutral if git not available
	}
	
	relPath, err := filepath.Rel(s.rootDir, filePath)
	if err != nil {
		return 0.5
	}
	
	// Count commits in last 30 days for this file
	sinceArg := time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02")
	
	cmd := exec.Command("git", "log", "--oneline", "--since", sinceArg, "--", relPath)
	cmd.Dir = s.rootDir
	
	output, err := cmd.Output()
	if err != nil {
		return 0.5 // Neutral on error
	}
	
	lines := strings.Count(string(output), "\n")
	
	// Score based on commit count
	// 0 commits = 0.0, 1-2 = 0.3, 3-5 = 0.5, 6-10 = 0.7, 10+ = 1.0
	if lines == 0 {
		return 0.0
	} else if lines <= 2 {
		return 0.3
	} else if lines <= 5 {
		return 0.5
	} else if lines <= 10 {
		return 0.7
	} else {
		return 1.0
	}
}

// calculateDependencyScore scores based on how many files import this one
// More dependents = higher score (central to codebase)
func (s *ImportanceScorer) calculateDependencyScore(relPath string) float64 {
	dependents, exists := s.dependencyMap[relPath]
	if !exists {
		return 0.3 // Default neutral score
	}
	
	count := len(dependents)
	
	// Score based on dependent count
	// 0 = 0.0, 1-2 = 0.4, 3-5 = 0.6, 6-10 = 0.8, 10+ = 1.0
	if count == 0 {
		return 0.0
	} else if count <= 2 {
		return 0.4
	} else if count <= 5 {
		return 0.6
	} else if count <= 10 {
		return 0.8
	} else {
		return 1.0
	}
}

// buildDependencyMap analyzes Go imports to build dependency graph
func (s *ImportanceScorer) buildDependencyMap() error {
	s.dependencyMap = make(map[string][]string)
	
	// Walk all Go files
	return filepath.Walk(s.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if info.IsDir() {
			// Skip common directories
			baseName := filepath.Base(path)
			if baseName == ".git" || baseName == "vendor" || baseName == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		
		relPath, _ := filepath.Rel(s.rootDir, path)
		
		// Read file and extract imports
		content, err := os.ReadFile(path)
		if err != nil {
			return nil // Skip files we can't read
		}
		
		imports := extractLocalImports(string(content), s.rootDir)
		
		// For each import, record that relPath depends on it
		for _, importPath := range imports {
			s.dependencyMap[importPath] = append(s.dependencyMap[importPath], relPath)
		}
		
		return nil
	})
}

// extractLocalImports extracts import paths that point to files in the project
func extractLocalImports(content, rootDir string) []string {
	var imports []string
	
	// Match import statements
	importPattern := regexp.MustCompile(`import\s+(?:\(([^)]+)\)|"([^"]+)")`)
	matches := importPattern.FindAllStringSubmatch(content, -1)
	
	for _, match := range matches {
		var importPaths string
		if match[1] != "" {
			importPaths = match[1]
		} else if match[2] != "" {
			importPaths = match[2]
		}
		
		// Extract package paths
		pathPattern := regexp.MustCompile(`"([^"]+)"`)
		paths := pathPattern.FindAllStringSubmatch(importPaths, -1)
		
		for _, pathMatch := range paths {
			if len(pathMatch) > 1 {
				pkgPath := pathMatch[1]
				
				// Check if it's a local import (contains project structure)
				// For now, we'll just track all imports
				// TODO: filter to only local packages
				imports = append(imports, pkgPath)
			}
		}
	}
	
	return imports
}

// FormatImportanceReport creates a human-readable report of file importance
func FormatImportanceReport(files []FileImportance) string {
	var builder strings.Builder
	
	builder.WriteString("# File Importance Scores\n\n")
	builder.WriteString(fmt.Sprintf("Total files: %d\n\n", len(files)))
	
	for i, file := range files {
		builder.WriteString(fmt.Sprintf("%d. %s (score: %.2f)\n", i+1, file.RelativePath, file.Score))
		builder.WriteString(fmt.Sprintf("   Recency: %.2f | Size: %.2f | Frequency: %.2f | Dependencies: %.2f\n",
			file.RecencyScore, file.SizeScore, file.FrequencyScore, file.DependencyScore))
		builder.WriteString("\n")
	}
	
	return builder.String()
}

// SortByImportance sorts files by importance score (highest first)
func SortByImportance(files []FileImportance) {
	// Use a simple bubble sort for small lists
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[j].Score > files[i].Score {
				files[i], files[j] = files[j], files[i]
			}
		}
	}
}
