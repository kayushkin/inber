package conversation

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// repairDanglingToolUse fixes messages where a tool_use block has no
// corresponding tool_result in the next message. This happens when a
// session is interrupted mid-tool-call. The Anthropic API requires
// every tool_use to be followed by a tool_result.
//
// Fix: append a synthetic tool_result with an error message for each
// dangling tool_use.
// repairCount tracks how many repairs were made in the last call
var LastRepairCount int

func RepairDanglingToolUse(messages []anthropic.MessageParam) []anthropic.MessageParam {
	LastRepairCount = 0
	if len(messages) == 0 {
		return messages
	}

	// Collect tool_use IDs from each assistant message, check if next
	// user message has matching tool_results
	var repaired []anthropic.MessageParam
	repairsNeeded := 0

	for i := 0; i < len(messages); i++ {
		repaired = append(repaired, messages[i])
		msg := messages[i]

		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}

		// Collect tool_use IDs from this assistant message
		var toolUseIDs []string
		for _, block := range msg.Content {
			if block.OfToolUse != nil {
				toolUseIDs = append(toolUseIDs, block.OfToolUse.ID)
			}
		}

		if len(toolUseIDs) == 0 {
			continue
		}

		// Check if next message is user with matching tool_results
		if i+1 < len(messages) && messages[i+1].Role == anthropic.MessageParamRoleUser {
			nextMsg := messages[i+1]
			resultIDs := make(map[string]bool)
			for _, block := range nextMsg.Content {
				if block.OfToolResult != nil {
					resultIDs[block.OfToolResult.ToolUseID] = true
				}
			}

			// Find missing tool_results
			var missing []string
			for _, id := range toolUseIDs {
				if !resultIDs[id] {
					missing = append(missing, id)
				}
			}

			if len(missing) > 0 {
				// Append missing tool_results to the next user message
				// We'll modify in place since we haven't appended it yet
				repairsNeeded += len(missing)
			}
		} else {
			// No user message follows — this is the last message and it has
			// dangling tool_use blocks. Insert a synthetic user message with
			// tool_results.
			var toolResults []anthropic.ContentBlockParamUnion
			for _, id := range toolUseIDs {
				toolResults = append(toolResults, anthropic.ContentBlockParamUnion{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: id,
						IsError:   anthropic.Bool(true),
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{
								Text: "[session interrupted — tool call was not completed]",
							}},
						},
					},
				})
			}
			repaired = append(repaired, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleUser,
				Content: toolResults,
			})
			repairsNeeded += len(toolUseIDs)
		}
	}

	// Second pass: fix cases where next user message exists but is missing
	// some tool_results (partial interruption)
	repaired, extraRepairs := RepairMissingToolResults(repaired)
	repairsNeeded += extraRepairs

	LastRepairCount = repairsNeeded
	return repaired
}

// repairAlternation fixes consecutive messages with the same role.
// Merges consecutive user messages and inserts placeholder assistant
// messages between consecutive assistant messages.
func RepairAlternation(messages []anthropic.MessageParam) []anthropic.MessageParam {
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
				// Merge consecutive user messages
				fixed[len(fixed)-1].Content = append(fixed[len(fixed)-1].Content, curr.Content...)
			} else {
				// Insert placeholder user message between consecutive assistant messages
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

// repairMissingToolResults adds missing tool_result blocks to user messages
// that follow assistant messages with tool_use.
func RepairMissingToolResults(messages []anthropic.MessageParam) ([]anthropic.MessageParam, int) {
	repairs := 0
	for i := 0; i < len(messages)-1; i++ {
		if messages[i].Role != anthropic.MessageParamRoleAssistant {
			continue
		}
		if messages[i+1].Role != anthropic.MessageParamRoleUser {
			continue
		}

		// Collect tool_use IDs
		toolUseIDs := make(map[string]bool)
		for _, block := range messages[i].Content {
			if block.OfToolUse != nil {
				toolUseIDs[block.OfToolUse.ID] = true
			}
		}
		if len(toolUseIDs) == 0 {
			continue
		}

		// Collect existing tool_result IDs
		for _, block := range messages[i+1].Content {
			if block.OfToolResult != nil {
				delete(toolUseIDs, block.OfToolResult.ToolUseID)
			}
		}

		// Add missing tool_results
		for id := range toolUseIDs {
			messages[i+1].Content = append(messages[i+1].Content, anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: id,
					IsError:   anthropic.Bool(true),
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{
							Text: "[session interrupted — tool call was not completed]",
						}},
					},
				},
			})
			repairs++
		}
	}
	return messages, repairs
}
