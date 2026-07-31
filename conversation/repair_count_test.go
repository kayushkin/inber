package conversation

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// RepairDanglingToolUse returns how many calls it answered, and both resume
// paths log that number ("repaired %d dangling tool_use blocks"). It used to
// report a partial interruption twice: the first pass worked out which results
// were missing from a user message that already existed, added them to the
// count and then did nothing about them, and the second pass — which is what
// actually appends them — counted the same ones again.

func assistantWithCalls(ids ...string) anthropic.MessageParam {
	var blocks []anthropic.ContentBlockParamUnion
	for _, id := range ids {
		blocks = append(blocks, anthropic.ContentBlockParamUnion{
			OfToolUse: &anthropic.ToolUseBlockParam{ID: id, Name: "read_files"},
		})
	}
	return anthropic.MessageParam{Role: anthropic.MessageParamRoleAssistant, Content: blocks}
}

func userWithResults(ids ...string) anthropic.MessageParam {
	var blocks []anthropic.ContentBlockParamUnion
	for _, id := range ids {
		blocks = append(blocks, anthropic.NewToolResultBlock(id, "ok", false))
	}
	return anthropic.MessageParam{Role: anthropic.MessageParamRoleUser, Content: blocks}
}

func TestAPartialInterruptionIsCountedOnce(t *testing.T) {
	// Two calls in one turn, one result logged.
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("go")),
		assistantWithCalls("toolu_a", "toolu_b"),
		userWithResults("toolu_a"),
	}

	repaired, repairs := RepairDanglingToolUse(messages)

	if repairs != 1 {
		t.Errorf("repairs = %d, want 1 — one call was answered, so one repair", repairs)
	}
	if unanswered := unansweredIDs(repaired); len(unanswered) != 0 {
		t.Errorf("still unanswered after the repair: %v", unanswered)
	}
}

func TestAFullyInterruptedTurnIsCountedOnce(t *testing.T) {
	// The log ends on the calls — nothing answers either of them.
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("go")),
		assistantWithCalls("toolu_a", "toolu_b"),
	}

	repaired, repairs := RepairDanglingToolUse(messages)

	if repairs != 2 {
		t.Errorf("repairs = %d, want 2 — neither call was answered", repairs)
	}
	if unanswered := unansweredIDs(repaired); len(unanswered) != 0 {
		t.Errorf("still unanswered after the repair: %v", unanswered)
	}
}

func TestACompleteTranscriptNeedsNoRepair(t *testing.T) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("go")),
		assistantWithCalls("toolu_a"),
		userWithResults("toolu_a"),
	}

	if _, repairs := RepairDanglingToolUse(messages); repairs != 0 {
		t.Errorf("repairs = %d, want 0 — every call was answered", repairs)
	}
}

func unansweredIDs(messages []anthropic.MessageParam) []string {
	answered := map[string]bool{}
	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfToolResult != nil {
				answered[block.OfToolResult.ToolUseID] = true
			}
		}
	}
	var unanswered []string
	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfToolUse != nil && !answered[block.OfToolUse.ID] {
				unanswered = append(unanswered, block.OfToolUse.ID)
			}
		}
	}
	return unanswered
}
