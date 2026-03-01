package context

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

// SmartTagger implements enhanced pattern-based tagging with Go AST analysis
type SmartTagger struct {
	errorPatterns    []*regexp.Regexp
	codePatterns     []*regexp.Regexp
	filePathPatterns []*regexp.Regexp
}

// NewSmartTagger creates a new smart tagger with enhanced detection
func NewSmartTagger() *SmartTagger {
	return &SmartTagger{
		errorPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\berror\b`),
			regexp.MustCompile(`(?i)\bpanic\b`),
			regexp.MustCompile(`(?i)\bfatal\b`),
			regexp.MustCompile(`(?i)\bstack trace\b`),
			regexp.MustCompile(`(?i)\bexception\b`),
			regexp.MustCompile(`at \w+\.\w+\([^)]+:\d+\)`), // Stack trace line
		},
		codePatterns: []*regexp.Regexp{
			regexp.MustCompile("```"),                    // Code blocks
			regexp.MustCompile(`(?m)^\s*func\s+\w+`),    // Go functions
			regexp.MustCompile(`(?m)^\s*type\s+\w+`),    // Go types
			regexp.MustCompile(`(?m)^\s*package\s+\w+`), // Go package
			regexp.MustCompile(`(?m)^\s*import\s+`),     // Imports
		},
		filePathPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?:^|[\s])([a-zA-Z0-9_\-./]+\.[a-z]{1,4})(?:[\s]|$)`), // file.ext
			regexp.MustCompile(`(?:^|[\s])(/[a-zA-Z0-9_\-./]+)(?:[\s]|$)`),            // /absolute/path
		},
	}
}

// Tag generates tags for the given text and source
func (t *SmartTagger) Tag(text string, source string) []string {
	tags := make(map[string]bool)
	
	// Source-based tags
	if source != "" {
		tags[source] = true
	}
	
	// Error detection
	for _, pattern := range t.errorPatterns {
		if pattern.MatchString(text) {
			tags["error"] = true
			break
		}
	}
	
	// Code detection
	for _, pattern := range t.codePatterns {
		if pattern.MatchString(text) {
			tags["code"] = true
			break
		}
	}
	
	// File path extraction with file: prefix
	for _, pattern := range t.filePathPatterns {
		matches := pattern.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) > 1 {
				path := strings.TrimSpace(match[1])
				if isValidFilePath(path) {
					tags["file:"+path] = true
					// Also tag the extension
					ext := filepath.Ext(path)
					if ext != "" {
						tags["ext:"+ext] = true
					}
				}
			}
		}
	}
	
	// Extract package names from Go import statements
	importTags := extractImportTags(text)
	for _, tag := range importTags {
		tags[tag] = true
	}
	
	// Extract function call tags (pkg.Function or method calls)
	functionTags := extractFunctionCallTags(text)
	for _, tag := range functionTags {
		tags[tag] = true
	}
	
	// If this looks like Go code, try AST parsing for deeper analysis
	if tags["code"] && strings.Contains(text, "package ") {
		astTags := extractASTTags(text)
		for _, tag := range astTags {
			tags[tag] = true
		}
	}
	
	// Identity detection
	identityKeywords := []string{"you are", "your role", "your name", "identity", "system prompt"}
	lowerText := strings.ToLower(text)
	for _, keyword := range identityKeywords {
		if strings.Contains(lowerText, keyword) {
			tags["identity"] = true
			break
		}
	}
	
	// Convert map to slice
	result := make([]string, 0, len(tags))
	for tag := range tags {
		result = append(result, tag)
	}
	
	return result
}

// extractImportTags extracts package names from Go import statements
func extractImportTags(text string) []string {
	var tags []string
	
	// Match import statements: import "pkg" or import ("pkg1" "pkg2")
	importPattern := regexp.MustCompile(`import\s+(?:\(([^)]+)\)|"([^"]+)")`)
	matches := importPattern.FindAllStringSubmatch(text, -1)
	
	for _, match := range matches {
		var importPaths string
		if match[1] != "" {
			// Multi-line import
			importPaths = match[1]
		} else if match[2] != "" {
			// Single import
			importPaths = match[2]
		}
		
		// Extract package paths
		pathPattern := regexp.MustCompile(`"([^"]+)"`)
		paths := pathPattern.FindAllStringSubmatch(importPaths, -1)
		
		for _, pathMatch := range paths {
			if len(pathMatch) > 1 {
				pkgPath := pathMatch[1]
				// Get package name from path (last segment)
				parts := strings.Split(pkgPath, "/")
				pkgName := parts[len(parts)-1]
				
				// Tag with both full path and package name
				tags = append(tags, "import:"+pkgPath)
				tags = append(tags, "pkg:"+pkgName)
			}
		}
	}
	
	return tags
}

// extractFunctionCallTags extracts function and method call tags
func extractFunctionCallTags(text string) []string {
	var tags []string
	
	// Match function calls: pkg.Function( or obj.Method(
	callPattern := regexp.MustCompile(`(\w+)\.(\w+)\s*\(`)
	matches := callPattern.FindAllStringSubmatch(text, -1)
	
	for _, match := range matches {
		if len(match) > 2 {
			pkg := match[1]
			fn := match[2]
			
			// Skip common stdlib packages to reduce noise
			if !isCommonStdlib(pkg) {
				tags = append(tags, "call:"+pkg+"."+fn)
				tags = append(tags, "pkg:"+pkg)
			}
		}
	}
	
	return tags
}

// extractASTTags uses Go AST parsing to extract structural tags
func extractASTTags(text string) []string {
	var tags []string
	
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "", text, parser.ImportsOnly)
	if err != nil {
		return tags // Return empty if parsing fails
	}
	
	// Package name
	if node.Name != nil {
		tags = append(tags, "pkg:"+node.Name.Name)
	}
	
	// Imports
	for _, imp := range node.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		tags = append(tags, "import:"+path)
		
		// Extract package name
		parts := strings.Split(path, "/")
		pkgName := parts[len(parts)-1]
		tags = append(tags, "pkg:"+pkgName)
	}
	
	return tags
}

// isValidFilePath checks if a string looks like a valid file path
func isValidFilePath(path string) bool {
	// Must have a file extension
	if filepath.Ext(path) == "" {
		return false
	}
	
	// Common false positives
	if strings.Contains(path, "http://") || strings.Contains(path, "https://") {
		return false
	}
	
	return true
}

// isCommonStdlib checks if a package is likely stdlib (to reduce noise)
func isCommonStdlib(pkg string) bool {
	common := []string{
		"fmt", "os", "io", "strings", "bytes", "time", "context",
		"http", "net", "sync", "errors", "log", "path", "filepath",
		"encoding", "json", "xml", "strconv", "sort", "math", "rand",
	}
	
	for _, stdlib := range common {
		if pkg == stdlib {
			return true
		}
	}
	
	return false
}
