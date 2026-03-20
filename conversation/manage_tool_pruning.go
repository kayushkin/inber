package conversation

import (
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// pruneToolResult applies age-based pruning to a tool result block
func pruneToolResult(block anthropic.ContentBlockParamUnion, age int, cfg ManagementConfig) (anthropic.ContentBlockParamUnion, bool) {
	if block.OfToolResult == nil {
		return block, false
	}

	toolResult := block.OfToolResult

	// Drop entirely if too old
	if age >= cfg.ToolResultDrop {
		return anthropic.ContentBlockParamUnion{
			OfToolResult: &anthropic.ToolResultBlockParam{
				ToolUseID: toolResult.ToolUseID,
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{
						OfText: &anthropic.TextBlockParam{
							Text: "[result dropped - too old]",
						},
					},
				},
			},
		}, true
	}

	// Keep full if recent
	if age < cfg.ToolResultKeepFull {
		return block, false
	}

	// Summarize if in middle range
	originalContent := extractToolResultContent(toolResult.Content)
	if originalContent == "" || len(originalContent) < 100 {
		return block, false // Keep short results as-is
	}

	// Create summary based on tool type (we don't have direct tool name, use heuristics)
	summary := summarizeToolResultByContent(originalContent)
	
	return anthropic.ContentBlockParamUnion{
		OfToolResult: &anthropic.ToolResultBlockParam{
			ToolUseID: toolResult.ToolUseID,
			Content: []anthropic.ToolResultBlockParamContentUnion{
				{
					OfText: &anthropic.TextBlockParam{
						Text: summary,
					},
				},
			},
		},
	}, true
}

// truncateToolCall summarizes a tool call input
func truncateToolCall(block anthropic.ContentBlockParamUnion) anthropic.ContentBlockParamUnion {
	if block.OfToolUse == nil {
		return block
	}

	toolUse := block.OfToolUse
	inputStr := fmt.Sprintf("%v", toolUse.Input)
	
	// Create brief summary
	summary := fmt.Sprintf("%s: %s", toolUse.Name, truncateToOneLine(inputStr, 60))
	
	// Return simplified version (keep structure but summarize input)
	return anthropic.ContentBlockParamUnion{
		OfToolUse: &anthropic.ToolUseBlockParam{
			ID:    toolUse.ID,
			Name:  toolUse.Name,
			Input: map[string]interface{}{"_summary": summary},
		},
	}
}

// extractToolResultContent extracts text from tool result content blocks
func extractToolResultContent(content []anthropic.ToolResultBlockParamContentUnion) string {
	var parts []string
	for _, block := range content {
		if block.OfText != nil {
			parts = append(parts, block.OfText.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// summarizeToolResultByContent creates a one-line summary of tool result content
func summarizeToolResultByContent(content string) string {
	lines := strings.Split(content, "\n")
	lineCount := len(lines)

	// Detect tool type by content patterns
	lower := strings.ToLower(content)
	
	// Shell command output
	if strings.Contains(content, "exit code") || strings.Contains(lower, "error:") || 
	   strings.Contains(lower, "warning:") || len(lines) > 5 {
		firstLine := ""
		if len(lines) > 0 {
			firstLine = truncateToOneLine(lines[0], 80)
		}
		return fmt.Sprintf("[shell: %d lines] %s", lineCount, firstLine)
	}
	
	// File read
	if lineCount > 20 && !strings.Contains(lower, "exit") {
		return fmt.Sprintf("[read file: %d lines, %d bytes]", lineCount, len(content))
	}
	
	// File write
	if strings.Contains(lower, "wrote") || strings.Contains(lower, "written") {
		return fmt.Sprintf("[wrote file: %d bytes]", len(content))
	}
	
	// List files
	if strings.Contains(content, "/") && lineCount > 3 {
		return fmt.Sprintf("[listed %d files]", lineCount)
	}
	
	// Memory search
	if strings.Contains(lower, "found") || strings.Contains(lower, "results") {
		return fmt.Sprintf("[search: %d results]", lineCount)
	}
	
	// Generic: first line + byte count
	firstLine := truncateToOneLine(lines[0], 80)
	return fmt.Sprintf("[%d bytes] %s", len(content), firstLine)
}

// truncateOldToolResults applies aggressive truncation to tool results in old messages (legacy)
func truncateOldToolResults(messages []anthropic.MessageParam) int {
	truncated := 0

	for i := range messages {
		if messages[i].Role != anthropic.MessageParamRoleUser {
			continue
		}

		// Check if message contains tool results
		for j := range messages[i].Content {
			if messages[i].Content[j].OfToolResult != nil {
				toolResult := messages[i].Content[j].OfToolResult
				
				// Get original content
				originalContent := extractToolResultContent(toolResult.Content)

				if originalContent != "" && len(originalContent) > 200 {
					// Apply summarization
					summarized := summarizeToolResultByContent(originalContent)
					
					// Update the content
					toolResult.Content = []anthropic.ToolResultBlockParamContentUnion{
						{
							OfText: &anthropic.TextBlockParam{
								Text: summarized,
							},
						},
					}
					truncated++
				}
			}
		}
	}

	return truncated
}