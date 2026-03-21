package session

import (
	"encoding/json"

	"github.com/kayushkin/inber/memory"

	"github.com/anthropics/anthropic-sdk-go"
)

// estimateToolTokens estimates the token count for tool definitions.
func estimateToolTokens(tools []anthropic.ToolUnionParam) int {
	total := 0
	for _, tool := range tools {
		total += 50
		if tool.OfTool != nil {
			desc := tool.OfTool.Description.Or("")
			total += memory.EstimateTokens(desc)
			// Rough schema estimate from JSON size
			if data, err := json.Marshal(tool.OfTool.InputSchema); err == nil {
				total += memory.EstimateTokens(string(data))
			}
		}
	}
	return total
}

// estimateSystemTokens estimates the token count for system prompt blocks.
func estimateSystemTokens(blocks []anthropic.TextBlockParam) int {
	total := 0
	for _, b := range blocks {
		total += memory.EstimateTokens(b.Text)
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
				total += memory.EstimateTokens(block.OfText.Text)
			} else if block.OfToolUse != nil || block.OfToolResult != nil {
				total += 50
			}
		}
	}
	return total
}