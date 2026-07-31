package conversation

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// interruptedToolResultText is what the model is told became of a tool call
// that was never answered. Both passes below synthesise the same block, so the
// sentence lives here rather than being written out at each of them: it is the
// one thing a resumed conversation says about a call that did not finish, and
// two copies of it are two answers waiting to drift apart.
const interruptedToolResultText = "[session interrupted — tool call was not completed]"

// repairDanglingToolUse fixes messages where a tool_use block has no
// corresponding tool_result in the next message. This happens when a
// session is interrupted mid-tool-call. The Anthropic API requires
// every tool_use to be followed by a tool_result.
//
// Fix: append a synthetic tool_result with an error message for each
// dangling tool_use.
func RepairDanglingToolUse(messages []anthropic.MessageParam) ([]anthropic.MessageParam, int) {
	if len(messages) == 0 {
		return messages, 0
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

		// A user message follows, so any result that is missing is missing from
		// a message that exists. RepairMissingToolResults, the second pass
		// below, both appends those and counts them — so this pass must leave
		// them alone. It used to count them here as well, and report twice as
		// many repairs as it made.
		if i+1 < len(messages) && messages[i+1].Role == anthropic.MessageParamRoleUser {
			continue
		}

		// Nothing answers this message: it is either the last one, or the next
		// one is another assistant message. Every one of its calls is dangling,
		// so insert the user message that answers them.
		var toolResults []anthropic.ContentBlockParamUnion
		for _, id := range toolUseIDs {
			toolResults = append(toolResults, anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: id,
					IsError:   anthropic.Bool(true),
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{
							Text: interruptedToolResultText,
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

	// Second pass: fix cases where next user message exists but is missing
	// some tool_results (partial interruption)
	repaired, extraRepairs := RepairMissingToolResults(repaired)
	repairsNeeded += extraRepairs

	return repaired, repairsNeeded
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

// RepairEmptyContent removes empty text blocks from messages and drops
// messages that end up with no content (e.g. summarized-away assistant turns).
// The Anthropic API rejects messages with empty text blocks.
func RepairEmptyContent(messages []anthropic.MessageParam) []anthropic.MessageParam {
	var result []anthropic.MessageParam
	for _, msg := range messages {
		// Filter out empty text blocks
		var filtered []anthropic.ContentBlockParamUnion
		for _, block := range msg.Content {
			if block.OfText != nil && block.OfText.Text == "" {
				continue // skip empty text blocks
			}
			filtered = append(filtered, block)
		}
		if len(filtered) == 0 {
			continue // drop messages with no content
		}
		msg.Content = filtered
		result = append(result, msg)
	}
	return result
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
							Text: interruptedToolResultText,
						}},
					},
				},
			})
			repairs++
		}
	}
	return messages, repairs
}

// RepairThinkingSignatures strips thinking and redacted_thinking blocks from
// assistant messages. This is needed when API credentials change mid-session,
// as thinking signatures are tied to the credential that generated them.
func RepairThinkingSignatures(messages []anthropic.MessageParam) []anthropic.MessageParam {
	var repaired []anthropic.MessageParam
	
	for _, msg := range messages {
		if msg.Role != anthropic.MessageParamRoleAssistant {
			repaired = append(repaired, msg)
			continue
		}
		
		// Filter out thinking and redacted_thinking blocks
		var filteredContent []anthropic.ContentBlockParamUnion
		for _, block := range msg.Content {
			// Check if this is a thinking or redacted_thinking block
			if block.OfThinking != nil || block.OfRedactedThinking != nil {
				continue // Skip thinking blocks
			}
			filteredContent = append(filteredContent, block)
		}
		
		// If no content blocks remain after filtering, replace with placeholder
		if len(filteredContent) == 0 {
			filteredContent = []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("[thinking redacted]"),
			}
		}
		
		msg.Content = filteredContent
		repaired = append(repaired, msg)
	}
	
	return repaired
}
