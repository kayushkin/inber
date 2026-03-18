package conversation

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/kayushkin/inber/memory"
)

// ContentType represents the type of large content detected (for stashing)
type ContentType string

const (
	ContentTypeLargeText   ContentType = "large-text"
	ContentTypeCodeBlock   ContentType = "code-block"
	ContentTypeLogOutput   ContentType = "log-output"
	ContentTypeFileContent ContentType = "file-content"
	ContentTypeErrorDump   ContentType = "error-dump"
)

// StashResult describes what was stashed
type StashResult struct {
	MemoryID    string      // ID of the memory where content was saved
	ContentType ContentType // Type of content that was detected
	Tokens      int         // Estimated token count of original content
	Summary     string      // Brief summary to replace the original content
}

// StashConfig contains configuration for stashing large content
type StashConfig struct {
	Enabled                bool    // Enable stashing of large content blocks
	UserMessageThreshold   int     // Stash user messages larger than this (tokens)
	AssistantThreshold     int     // Stash assistant responses larger than this (tokens)
	MinBlockSize          int     // Minimum block size to consider stashing (tokens)
	DefaultImportance     float64 // Default importance for stashed content
}

// DefaultStashConfig returns sensible defaults for stashing
func DefaultStashConfig() StashConfig {
	return StashConfig{
		Enabled:               true,
		UserMessageThreshold:  2000,
		AssistantThreshold:    3000,
		MinBlockSize:         1000,
		DefaultImportance:    0.3,
	}
}

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

// StashLargeContent saves large content to memory and returns a summary
func StashLargeContent(
	content string,
	sessionID string,
	memStore memory.MemoryStore,
	cfg StashConfig,
) (*StashResult, error) {
	tokens := memory.EstimateTokens(content)

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
func DetectAndStashLargeBlocks(
	text string,
	sessionID string,
	memStore memory.MemoryStore,
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

		tokens := memory.EstimateTokens(codeContent)
		if tokens >= cfg.MinBlockSize {
			result, err := StashLargeContent(codeContent, sessionID, memStore, cfg)
			if err != nil {
				log.Printf("[warn] failed to stash code block: %v", err)
				continue
			}
			if result != nil {
				stashed = append(stashed, *result)
				modifiedText = strings.Replace(modifiedText, fullMatch, result.Summary, 1)
			}
		}
	}

	// 2. Check overall text size after code block stashing
	remainingTokens := memory.EstimateTokens(modifiedText)
	if remainingTokens >= cfg.MinBlockSize {
		// The remaining text is still large - check if it's a single large block
		// Split by double newlines to detect large paragraphs
		paragraphs := strings.Split(modifiedText, "\n\n")
		
		var rebuiltText []string
		for _, para := range paragraphs {
			paraTokens := memory.EstimateTokens(para)
			if paraTokens >= cfg.MinBlockSize {
				result, err := StashLargeContent(para, sessionID, memStore, cfg)
				if err != nil {
					log.Printf("[warn] failed to stash paragraph: %v", err)
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

	// 3. Fallback: if total message is still large and nothing was stashed,
	// stash the entire message (e.g., repetitive error dumps with small paragraphs)
	if len(stashed) == 0 {
		totalTokens := memory.EstimateTokens(modifiedText)
		if totalTokens >= cfg.UserMessageThreshold {
			result, err := StashLargeContent(modifiedText, sessionID, memStore, cfg)
			if err != nil {
				log.Printf("[warn] failed to stash entire large message: %v", err)
			} else if result != nil {
				stashed = append(stashed, *result)
				modifiedText = result.Summary
			}
		}
	}

	return modifiedText, stashed, nil
}

// ApplyStashing applies stashing logic to conversation messages
func ApplyStashing(
	messages []anthropic.MessageParam,
	sessionID string,
	memStore memory.MemoryStore,
	cfg StashConfig,
) ([]anthropic.MessageParam, int, error) {
	if !cfg.Enabled || memStore == nil {
		return messages, 0, nil
	}

	totalStashed := 0
	result := make([]anthropic.MessageParam, len(messages))
	copy(result, messages)

	for i := range result {
		msg := &result[i]
		stashedInMsg := false

		// Process each content block
		for j, block := range msg.Content {
			if block.OfText != nil {
				text := block.OfText.Text
				tokens := memory.EstimateTokens(text)

				// Check if this text block should be stashed
				shouldStash := false
				if msg.Role == anthropic.MessageParamRoleUser && tokens > cfg.UserMessageThreshold {
					shouldStash = true
				} else if msg.Role == anthropic.MessageParamRoleAssistant && tokens > cfg.AssistantThreshold {
					shouldStash = true
				} else if tokens > cfg.MinBlockSize*3 { // Very large blocks regardless of role
					shouldStash = true
				}

				if shouldStash {
					// Stash large content
					modifiedText, stashResults, err := DetectAndStashLargeBlocks(text, sessionID, memStore, cfg)
					if err != nil {
						log.Printf("[warn] stashing failed: %v", err)
						continue
					} else if len(stashResults) > 0 {
						// Replace with stashed summary
						newTextBlock := anthropic.ContentBlockParamUnion{
							OfText: &anthropic.TextBlockParam{Text: modifiedText},
						}
						msg.Content[j] = newTextBlock
						stashedInMsg = true
						totalStashed += len(stashResults)
					}
				}
			}
		}

		if stashedInMsg {
			// Message was modified, store it back
			result[i] = *msg
		}
	}

	return result, totalStashed, nil
}