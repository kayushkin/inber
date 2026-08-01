package conversation

import (
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// toolNamesByUseID pairs each tool_use id with the name of the tool that was
// called. A tool_result block carries only the id, so a summary of that result
// can name its tool only by looking the id up here.
func toolNamesByUseID(messages []anthropic.MessageParam) map[string]string {
	names := make(map[string]string)
	for _, msg := range messages {
		for _, block := range msg.Content {
			// An id-less call is skipped: its entry would answer to every
			// result that also has no id, and name it wrongly.
			if block.OfToolUse != nil && block.OfToolUse.ID != "" {
				names[block.OfToolUse.ID] = block.OfToolUse.Name
			}
		}
	}
	return names
}

// pruneToolResult applies age-based pruning to a tool result block. toolName is
// the name of the tool that produced the result, from toolNamesByUseID; pass
// "" when the pairing is missing.
func pruneToolResult(block anthropic.ContentBlockParamUnion, age int, cfg ManagementConfig, toolName string) (anthropic.ContentBlockParamUnion, bool) {
	if block.OfToolResult == nil {
		return block, false
	}

	toolResult := block.OfToolResult
	originalContent := extractToolResultContent(toolResult.Content)

	// Drop entirely if too old. The marker still names the tool and the size,
	// so the transcript records what was lost rather than only that something was.
	if age >= cfg.ToolResultDrop {
		return replaceToolResultText(block,
			fmt.Sprintf("[%s: dropped - too old, %d bytes]", toolLabel(toolName), len(originalContent))), true
	}

	// Keep full if recent
	if age < cfg.ToolResultKeepFull {
		return block, false
	}

	// Summarize if in middle range
	if originalContent == "" || len(originalContent) < 100 {
		return block, false // Keep short results as-is
	}

	return replaceToolResultText(block, summarizeToolResult(toolName, originalContent)), true
}

// replaceToolResultText swaps a tool result's content for text and leaves every
// other field of the result exactly as it was.
//
// Pruning rewrites what a result SAYS. It must not rewrite what the result IS,
// and building a fresh ToolResultBlockParam does precisely that: it silently
// drops every field the literal forgets. The field that matters is is_error.
// The engine sets it on every result it appends (agent/agent_run.go,
// engine/turn_openai.go), so a rebuild turned a call that FAILED into a call
// that succeeded the moment it aged past ToolResultKeepFull — and the drop path
// deleted the error text as well, leaving nothing at all to say it had failed.
// Copying the struct also means a field added to the SDK later is carried
// through rather than quietly lost here.
func replaceToolResultText(block anthropic.ContentBlockParamUnion, text string) anthropic.ContentBlockParamUnion {
	replaced := *block.OfToolResult
	replaced.Content = []anthropic.ToolResultBlockParamContentUnion{
		{OfText: &anthropic.TextBlockParam{Text: text}},
	}
	return anthropic.ContentBlockParamUnion{OfToolResult: &replaced}
}

// truncateToolCall summarizes a tool call input
func truncateToolCall(block anthropic.ContentBlockParamUnion) anthropic.ContentBlockParamUnion {
	if block.OfToolUse == nil {
		return block
	}

	toolUse := block.OfToolUse

	// Create brief summary. The input has to be rendered as JSON text: it is
	// usually a json.RawMessage, and %v on those bytes prints decimal byte
	// codes instead of the arguments.
	summary := fmt.Sprintf("%s: %s", toolUse.Name, truncateToOneLine(ToolInputText(toolUse.Input), 60))
	
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

// toolLabel names the tool a pruned result came from. An unpaired result says
// so; it never guesses a tool from the content.
func toolLabel(toolName string) string {
	if toolName == "" {
		return "tool result"
	}
	return toolName
}

// summarizeToolResult creates a one-line summary of a tool result. It names the
// tool that produced the result and keeps both the line count and the byte
// count, so the transcript always records how much was dropped.
func summarizeToolResult(toolName, content string) string {
	lines := strings.Split(content, "\n")
	summary := fmt.Sprintf("[%s: %d lines, %d bytes] %s",
		toolLabel(toolName), len(lines), len(content), truncateToOneLine(lines[0], 80))
	return strings.TrimRight(summary, " ")
}

// truncateOldToolResults applies aggressive truncation to tool results in old messages (legacy)
func truncateOldToolResults(messages []anthropic.MessageParam) int {
	truncated := 0
	toolNames := toolNamesByUseID(messages)

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
					summarized := summarizeToolResult(toolNames[toolResult.ToolUseID], originalContent)
					
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