package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/memory"
)

// StashConfig configures large message stashing behavior
type StashConfig struct {
	Enabled              bool
	UserMessageThreshold int     // Stash user messages larger than this (tokens)
	AssistantThreshold   int     // Stash assistant responses larger than this (tokens)
	MinBlockSize         int     // Minimum block size to consider stashing (tokens)
	DefaultImportance    float64 // Default importance for stashed content
}

// DefaultStashConfig returns sensible defaults for message stashing
func DefaultStashConfig() StashConfig {
	return StashConfig{
		Enabled:              true,
		UserMessageThreshold: 1000,
		AssistantThreshold:   1500,
		MinBlockSize:         1000,
		DefaultImportance:    0.6,
	}
}

// ContentType represents the type of large content detected
type ContentType string

const (
	ContentTypeErrorDump   ContentType = "error-dump"
	ContentTypeCodeBlock   ContentType = "code-block"
	ContentTypeLogOutput   ContentType = "log-output"
	ContentTypeFileContent ContentType = "file-contents"
	ContentTypeLargeText   ContentType = "large-text"
)

// DetectContentType identifies what type of content a large block contains
func DetectContentType(content string) ContentType {
	lower := strings.ToLower(content)

	// Error dump detection
	errorPatterns := []string{
		"error:", "exception:", "panic:", "traceback", "stack trace",
		"fatal:", "segmentation fault", "core dumped",
	}
	for _, pattern := range errorPatterns {
		if strings.Contains(lower, pattern) {
			return ContentTypeErrorDump
		}
	}

	// Code block detection (lots of code fences)
	codeFenceCount := strings.Count(content, "```")
	if codeFenceCount >= 4 { // At least 2 code blocks
		return ContentTypeCodeBlock
	}

	// Log output detection (timestamps + repeated patterns)
	timestampPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\d{4}-\d{2}-\d{2}`),                    // 2024-01-15
		regexp.MustCompile(`\d{2}:\d{2}:\d{2}`),                    // 12:34:56
		regexp.MustCompile(`\[.*?\]`),                              // [INFO], [ERROR]
		regexp.MustCompile(`(?i)(DEBUG|INFO|WARN|ERROR|FATAL):`),  // Log levels
	}
	timestampMatches := 0
	for _, pattern := range timestampPatterns {
		if pattern.MatchString(content) {
			timestampMatches++
		}
	}
	if timestampMatches >= 2 {
		return ContentTypeLogOutput
	}

	// File contents detection (file paths + line numbers)
	hasFilePaths := strings.Contains(content, "/") || strings.Contains(content, "\\")
	lineNumberPattern := regexp.MustCompile(`:\d+:`)
	hasLineNumbers := lineNumberPattern.MatchString(content)
	if hasFilePaths && hasLineNumbers {
		return ContentTypeFileContent
	}

	return ContentTypeLargeText
}

// StashResult describes what was stashed
type StashResult struct {
	MemoryID    string
	ContentType ContentType
	Tokens      int
	Summary     string
}

// StashLargeContent saves large content to memory and returns a summary
func StashLargeContent(
	content string,
	sessionID string,
	memStore *memory.Store,
	cfg StashConfig,
) (*StashResult, error) {
	tokens := inbercontext.EstimateTokens(content)

	if tokens < cfg.MinBlockSize {
		return nil, nil // Too small to stash
	}

	// Detect content type
	contentType := DetectContentType(content)

	// Generate tags
	tags := []string{"large-input", "stashed", sessionID, string(contentType)}

	// Save to memory
	memID := uuid.New().String()
	mem := memory.Memory{
		ID:         memID,
		Content:    content,
		Tags:       tags,
		Importance: cfg.DefaultImportance,
		Source:     "system",
	}

	if err := memStore.Save(mem); err != nil {
		return nil, fmt.Errorf("save stashed content: %w", err)
	}

	// Generate summary
	summary := fmt.Sprintf("[Large content stashed — %s, ~%d tokens. Use memory_search or memory_expand(id=\"%s\") to recall full content]",
		contentType, tokens, memID[:8])

	return &StashResult{
		MemoryID:    memID,
		ContentType: contentType,
		Tokens:      tokens,
		Summary:     summary,
	}, nil
}

// DetectAndStashLargeBlocks scans text for large blocks and stashes them
// Returns modified text with summaries in place of large blocks
func DetectAndStashLargeBlocks(
	text string,
	sessionID string,
	memStore *memory.Store,
	cfg StashConfig,
) (string, []StashResult, error) {
	if !cfg.Enabled || memStore == nil {
		return text, nil, nil
	}

	var stashed []StashResult

	// Strategy: detect code blocks first (they're explicit)
	// Then detect large paragraphs

	// 1. Extract code blocks
	codeBlockPattern := regexp.MustCompile("(?s)```[a-z]*\n(.*?)```")
	matches := codeBlockPattern.FindAllStringSubmatch(text, -1)
	
	modifiedText := text
	for _, match := range matches {
		fullMatch := match[0]
		codeContent := match[1]

		tokens := inbercontext.EstimateTokens(codeContent)
		if tokens >= cfg.MinBlockSize {
			result, err := StashLargeContent(codeContent, sessionID, memStore, cfg)
			if err != nil {
				Log.Warn("failed to stash code block: %v", err)
				continue
			}
			if result != nil {
				stashed = append(stashed, *result)
				modifiedText = strings.Replace(modifiedText, fullMatch, result.Summary, 1)
			}
		}
	}

	// 2. Check overall text size after code block stashing
	remainingTokens := inbercontext.EstimateTokens(modifiedText)
	if remainingTokens >= cfg.MinBlockSize {
		// The remaining text is still large - check if it's a single large block
		// Split by double newlines to detect large paragraphs
		paragraphs := strings.Split(modifiedText, "\n\n")
		
		var rebuiltText []string
		for _, para := range paragraphs {
			paraTokens := inbercontext.EstimateTokens(para)
			if paraTokens >= cfg.MinBlockSize {
				result, err := StashLargeContent(para, sessionID, memStore, cfg)
				if err != nil {
					Log.Warn("failed to stash paragraph: %v", err)
					rebuiltText = append(rebuiltText, para)
					continue
				}
				if result != nil {
					stashed = append(stashed, *result)
					rebuiltText = append(rebuiltText, result.Summary)
				} else {
					rebuiltText = append(rebuiltText, para)
				}
			} else {
				rebuiltText = append(rebuiltText, para)
			}
		}
		modifiedText = strings.Join(rebuiltText, "\n\n")
	}

	return modifiedText, stashed, nil
}
