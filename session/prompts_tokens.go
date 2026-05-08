package session

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
)

// estimateTokens is a coarse char-to-token approximation (~3 chars/token)
// used only to render token counts inside human-readable prompts/*.md files.
// Kept local so the session package does not depend on the memory layer;
// budget enforcement uses memory-store's estimator at call sites that matter.
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + 2) / 3
}

// estimateToolTokens estimates the token count for tool definitions.
func estimateToolTokens(tools []anthropic.ToolUnionParam) int {
	total := 0
	for _, tool := range tools {
		total += 50
		if tool.OfTool != nil {
			desc := tool.OfTool.Description.Or("")
			total += estimateTokens(desc)
			// Rough schema estimate from JSON size
			if data, err := json.Marshal(tool.OfTool.InputSchema); err == nil {
				total += estimateTokens(string(data))
			}
		}
	}
	return total
}

// estimateSystemTokens estimates the token count for system prompt blocks.
func estimateSystemTokens(blocks []anthropic.TextBlockParam) int {
	total := 0
	for _, b := range blocks {
		total += estimateTokens(b.Text)
	}
	return total
}

// estimateMessageTokens estimates the token count for message content.
func estimateMessageTokens(messages []anthropic.MessageParam) int {
	total := 0
	for _, msg := range messages {
		total += 4
		for _, block := range msg.Content {
			if block.OfText != nil {
				total += estimateTokens(block.OfText.Text)
			} else if block.OfToolUse != nil || block.OfToolResult != nil {
				total += 50
			}
		}
	}
	return total
}