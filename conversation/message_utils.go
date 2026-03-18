package conversation

import (
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// findTurnBoundary locates where to split messages to keep the specified number of recent turns.
// It works backward from the end of messages to find turn boundaries (user→assistant pairs)
// and ensures tool_use and tool_result pairs remain intact.
func findTurnBoundary(messages []anthropic.MessageParam, keepTurns int) int {
	if len(messages) == 0 {
		return 0
	}

	turns := 0
	splitAt := 0
	
	// Count turns from the end (each user message = 1 turn)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == anthropic.MessageParamRoleUser {
			turns++
			if turns >= keepTurns {
				splitAt = i
				break
			}
		}
	}

	// Now verify tool integrity: check if any message in the "keep" section (splitAt onward)
	// has a tool_result whose tool_use is in the "old" section (before splitAt).
	// If so, move the split point back to include the tool_use message.
	for {
		toolUseIDs := collectToolUseIDs(messages[:splitAt])
		orphanedResults := findOrphanedToolResults(messages[splitAt:], toolUseIDs)
		if len(orphanedResults) == 0 {
			break
		}
		// Move split back — find the earliest tool_use that's needed
		if splitAt <= 0 {
			break
		}
		splitAt--
		// Back up to the previous user message boundary
		for splitAt > 0 && messages[splitAt].Role != anthropic.MessageParamRoleUser {
			splitAt--
		}
	}

	return splitAt
}

// collectToolUseIDs extracts all tool use IDs from messages that will be removed during summarization
func collectToolUseIDs(messages []anthropic.MessageParam) map[string]bool {
	toolUseIDs := make(map[string]bool)
	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleAssistant {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					toolUseIDs[block.OfToolUse.ID] = true
				}
			}
		}
	}
	return toolUseIDs
}

// findOrphanedToolResults identifies tool results that reference removed tool uses
func findOrphanedToolResults(messages []anthropic.MessageParam, removedToolUseIDs map[string]bool) []string {
	var orphans []string
	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleUser {
			for _, block := range msg.Content {
				if block.OfToolResult != nil {
					if removedToolUseIDs[block.OfToolResult.ToolUseID] {
						orphans = append(orphans, block.OfToolResult.ToolUseID)
					}
				}
			}
		}
	}
	return orphans
}

// countTurns counts the number of user→assistant turn pairs in messages
func countTurns(messages []anthropic.MessageParam) int {
	turns := 0
	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleUser {
			turns++
		}
	}
	return turns
}

// messagesToText converts messages to plain text for summarization
func messagesToText(messages []anthropic.MessageParam) string {
	var lines []string
	
	for _, msg := range messages {
		role := string(msg.Role)
		
		// Extract text content from each message
		var contentParts []string
		for _, block := range msg.Content {
			if block.OfText != nil {
				contentParts = append(contentParts, block.OfText.Text)
			} else if block.OfToolUse != nil {
				contentParts = append(contentParts, fmt.Sprintf("[tool_use: %s]", block.OfToolUse.Name))
			} else if block.OfToolResult != nil {
				contentParts = append(contentParts, fmt.Sprintf("[tool_result: %s]", extractToolResultText(block)))
			}
		}
		
		if len(contentParts) > 0 {
			content := strings.Join(contentParts, "\n")
			lines = append(lines, fmt.Sprintf("%s: %s", role, content))
		}
	}
	
	return strings.Join(lines, "\n\n")
}

// extractToolResultText extracts text content from a tool result block
func extractToolResultText(block anthropic.ContentBlockParamUnion) string {
	if block.OfToolResult == nil {
		return ""
	}
	
	var texts []string
	for _, content := range block.OfToolResult.Content {
		if content.OfText != nil {
			texts = append(texts, content.OfText.Text)
		}
	}
	result := strings.Join(texts, "\n")
	if len(result) > 200 {
		return result[:200] + "..."
	}
	return result
}

// fixAlternation ensures messages alternate properly between user and assistant roles
func fixAlternation(messages []anthropic.MessageParam) []anthropic.MessageParam {
	if len(messages) <= 1 {
		return messages
	}

	var fixed []anthropic.MessageParam
	fixed = append(fixed, messages[0])

	for i := 1; i < len(messages); i++ {
		prev := fixed[len(fixed)-1]
		curr := messages[i]

		if prev.Role == curr.Role {
			if curr.Role == anthropic.MessageParamRoleUser {
				// Two user messages in a row — merge
				fixed[len(fixed)-1].Content = append(fixed[len(fixed)-1].Content, curr.Content...)
			} else {
				// Two assistant messages — insert a placeholder user message
				fixed = append(fixed, anthropic.NewUserMessage(
					anthropic.NewTextBlock("[continued]"),
				))
				fixed = append(fixed, curr)
			}
		} else {
			fixed = append(fixed, curr)
		}
	}

	return fixed
}

// stripOrphanedToolResults removes tool result blocks that reference non-existent tool uses
func stripOrphanedToolResults(messages []anthropic.MessageParam) []anthropic.MessageParam {
	// First pass: collect all tool use IDs that exist
	existingToolUseIDs := make(map[string]bool)
	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleAssistant {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					existingToolUseIDs[block.OfToolUse.ID] = true
				}
			}
		}
	}

	// Second pass: filter out orphaned tool results
	var cleaned []anthropic.MessageParam
	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleUser {
			var keptBlocks []anthropic.ContentBlockParamUnion
			for _, block := range msg.Content {
				if block.OfToolResult != nil {
					// Only keep tool results that have corresponding tool uses
					if existingToolUseIDs[block.OfToolResult.ToolUseID] {
						keptBlocks = append(keptBlocks, block)
					}
				} else {
					keptBlocks = append(keptBlocks, block)
				}
			}
			
			// Only include the message if it has content after filtering
			if len(keptBlocks) > 0 {
				msg.Content = keptBlocks
				cleaned = append(cleaned, msg)
			}
		} else {
			cleaned = append(cleaned, msg)
		}
	}

	return cleaned
}