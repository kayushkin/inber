package conversation

import (
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/internal/textutil"
)

// StartsUserTurn reports whether a message begins a new conversation turn.
//
// Role alone does not answer this. A tool round-trip also appends a user-role
// message — agent.Run builds one with anthropic.NewUserMessage(toolResults...)
// after every batch of tool calls — so counting user-role messages counts tool
// round-trips as turns. Measured over the transcripts on this host that is a
// 3.3x overcount, and 6x on the most tool-heavy one, which is how KeepRecentTurns
// of 8 came to retain roughly two of the user's turns.
//
// A user-role message that carries anything other than tool_result blocks came
// from the user, or is one of the synthetic user-role injections the model reads
// as a user turn ("[New message from user while you were working]",
// "[BUDGET LIMIT REACHED]", the summary hand-off). Those do start a turn.
//
// The empty and unrecognised cases answer true on purpose. Overcounting turns
// makes the caller retain less than it was configured to, which drops real
// conversation; undercounting only retains more. When the shape is unclear,
// err toward keeping.
func StartsUserTurn(msg anthropic.MessageParam) bool {
	if msg.Role != anthropic.MessageParamRoleUser {
		return false
	}
	if len(msg.Content) == 0 {
		return true
	}
	for _, block := range msg.Content {
		if block.OfToolResult == nil {
			return true
		}
	}
	return false
}

// findTurnBoundary locates where to split messages to keep the specified number of recent turns.
// It works backward from the end of messages to find turn boundaries (user→assistant pairs)
// and ensures tool_use and tool_result pairs remain intact.
func findTurnBoundary(messages []anthropic.MessageParam, keepTurns int) int {
	if len(messages) == 0 {
		return 0
	}

	turns := 0
	splitAt := 0

	// Count turns from the end. A turn starts at a user message that is not a
	// batch of tool results — see StartsUserTurn.
	for i := len(messages) - 1; i >= 0; i-- {
		if StartsUserTurn(messages[i]) {
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
		// Back up to the previous turn boundary. Stopping at any user-role
		// message reaches the same index — a batch of tool results left at the
		// front of the kept slice orphans on the next pass and this loop backs
		// up again — but it says what the loop is looking for, and gets there
		// without the extra pass. Checked equal on 200,000 generated
		// (transcript, keepTurns) pairs.
		for splitAt > 0 && !StartsUserTurn(messages[splitAt]) {
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

// countTurns counts the number of user→assistant turn pairs in messages.
// It counts the same thing findTurnBoundary splits on, so the "%d turns" a
// summary reports is the number of turns that were actually summarized away.
func countTurns(messages []anthropic.MessageParam) int {
	turns := 0
	for _, msg := range messages {
		if StartsUserTurn(msg) {
			turns++
		}
	}
	return turns
}

// messagesToText converts messages to plain text for summarization.
//
// This is the only thing the summarizing model ever sees of the turns being
// compacted, and the summary replaces them, so whatever this rendering leaves
// out leaves the conversation with it — and the memory archive too.
//
// So a failed tool call has to arrive marked as one. is_error is the single
// structured signal that a call failed; the text alone cannot carry it, because
// the output is truncated to the first 200 characters and a command that prints
// progress and then dies puts its error message last. Rendered without the
// flag, `make: *** [all] Error 2` after forty ok lines reads to the summarizer
// as a clean build, and the summary then states it as one for the rest of the
// session. pruneToolResult already carries is_error through for the same reason
// (see replaceToolResultText); this path is where it was still being dropped.
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
				label := "tool_result"
				if block.OfToolResult.IsError.Or(false) {
					label = "tool_result failed"
				}
				contentParts = append(contentParts, fmt.Sprintf("[%s: %s]", label, extractToolResultText(block)))
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
	return textutil.TruncateWith(result, 200, "...")
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