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
	Enabled              bool    // Enable stashing of large content blocks
	UserMessageThreshold int     // Stash user messages larger than this (tokens)
	AssistantThreshold   int     // Stash assistant responses larger than this (tokens)
	MinBlockSize         int     // Minimum block size to consider stashing (tokens)
	DefaultImportance    float64 // Default importance for stashed content

	// RecallToolNames are the tool names this session actually puts on the wire.
	// The stash reads it to answer one question: can the model get a stashed
	// block back, and by which tool.
	//
	// Nothing else in this config can answer it. Enabled and the thresholds are
	// about the write; the memory store being non-nil is about the write. The
	// read is a tool call, and whether the model holds that tool is decided by
	// the agent's configured tool list and by SetDisabledTools, neither of which
	// this package can see. So the caller fills it from the tools it just sent —
	// engine.stashConfigForTurn reads EnabledToolNames — and a caller that
	// leaves it empty is saying the model has no way back, which turns stashing
	// off rather than performing it silently.
	RecallToolNames []string
}

// recallInstruction returns the sentence that tells the model how to get a
// stashed block back, and whether there is any way back at all.
//
// The pointer used to name memory_search and memory_expand unconditionally,
// which made it a claim about this repository rather than about the session:
// both tools exist here, and an agent configured without them holds neither.
// For an agent with memory_search but no memory_expand the pointer named a tool
// it could not call; for an agent with no memory tools at all the stash was a
// deletion with a receipt, since lifting the block out of the message is what
// makes the stash worth doing and the memory row is then unreachable.
//
// This is the same pair conversation.summaryFooter fixed for the compaction
// archive — write happened, and the read is on the wire — arriving at the other
// writer that takes content out of a conversation.
func (c StashConfig) recallInstruction(memoryID string) (string, bool) {
	hasSearch, hasExpand := false, false
	for _, name := range c.RecallToolNames {
		switch name {
		case memory.ToolNameMemorySearch:
			hasSearch = true
		case memory.ToolNameMemoryExpand:
			hasExpand = true
		}
	}

	switch {
	case hasSearch && hasExpand:
		return fmt.Sprintf("Use memory_search or memory_expand(id=%q) to recall full content", memoryID), true
	case hasExpand:
		return fmt.Sprintf("Use memory_expand(id=%q) to recall full content", memoryID), true
	case hasSearch:
		// No id, deliberately: memory_search takes a query, and an id the model
		// cannot pass anywhere is an invitation to hallucinate a call for it.
		return "Use memory_search to recall full content", true
	}
	return "", false
}

// CanRecallStashedContent reports whether this session holds a tool that reaches
// a stashed block. When it does not, nothing may be stashed: the block would
// leave the conversation and no read would exist that brings it back.
func (c StashConfig) CanRecallStashedContent() bool {
	_, ok := c.recallInstruction("")
	return ok
}

// DefaultStashConfig returns sensible defaults for stashing
func DefaultStashConfig() StashConfig {
	return StashConfig{
		Enabled:              true,
		UserMessageThreshold: 2000,
		AssistantThreshold:   3000,
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
		regexp.MustCompile(`\d{4}-\d{2}-\d{2}`),                  // 2024-01-15
		regexp.MustCompile(`\d{2}:\d{2}:\d{2}`),                  // 12:34:56
		regexp.MustCompile(`\[.*?\]`),                            // [INFO], [ERROR]
		regexp.MustCompile(`(?i)(DEBUG|INFO|WARN|ERROR|FATAL):`), // Log levels
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

	// Nothing on the wire reaches a stashed block, so stashing one would delete
	// it. Refused here and not only at the caller because this function is
	// exported and the memory row is written below: a guard at one call site
	// leaves the other call sites free to write an unreachable memory.
	if !cfg.CanRecallStashedContent() {
		return nil, nil
	}

	// Detect content type
	contentType := DetectContentType(content)

	// Generate tags
	tags := StashedContentTags(sessionID, contentType)

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

	// Generate summary. The id is named in full: memory-store resolves a prefix
	// onto a row, but two of 35,000 uuids sharing their first eight characters is
	// likelier than not, and that read answers with whichever row SQLite reaches
	// first rather than failing.
	recall, _ := cfg.recallInstruction(memID)
	summary := fmt.Sprintf("[Large content stashed — %s, ~%d tokens. %s]",
		contentType, tokens, recall)

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

	// The write is enabled and the store is there; the read is the third gate,
	// and it is the one that decides whether the model ever sees this content
	// again. Answered before the scan so a session that cannot recall does not
	// pay to detect blocks it must leave alone.
	if !cfg.CanRecallStashedContent() {
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
