package conversation

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// The rule these tests pin was measured against the live API on 2026-08-03,
// not read off the comment in agent/agent_run.go that states it. Same three
// messages both ways round, claude-haiku-4-5, one tool:
//
//	user(tool_result, text) -> HTTP 200
//	user(text, tool_result) -> HTTP 400 invalid_request_error
//	  "messages.2: `tool_use` ids were found without `tool_result` blocks
//	   immediately after: toolu_... Each `tool_use` block must have a
//	   corresponding `tool_result` block in the next message."
//
// Note what the refusal says: the API reports the result as MISSING, not as
// misplaced. A repair that put the block in the wrong place therefore fails
// looking exactly like a repair that never ran, which is why this is worth a
// test rather than a comment.

// assertToolResultsComeFirst fails when any tool_result block sits after a
// block that is not one, which is the shape the API refuses.
func assertToolResultsComeFirst(t *testing.T, msg anthropic.MessageParam) {
	t.Helper()
	seenNonResult := false
	for i, block := range msg.Content {
		if block.OfToolResult == nil {
			seenNonResult = true
			continue
		}
		if seenNonResult {
			t.Fatalf("content block %d is a tool_result and something else precedes it: %s",
				i, describeBlocks(msg.Content))
		}
	}
}

func describeBlocks(blocks []anthropic.ContentBlockParamUnion) string {
	out := ""
	for _, block := range blocks {
		switch {
		case block.OfToolResult != nil:
			out += "tool_result(" + block.OfToolResult.ToolUseID + ") "
		case block.OfText != nil:
			out += "text "
		case block.OfToolUse != nil:
			out += "tool_use(" + block.OfToolUse.ID + ") "
		default:
			out += "other "
		}
	}
	return out
}

func assistantWithToolUses(ids ...string) anthropic.MessageParam {
	var content []anthropic.ContentBlockParamUnion
	for _, id := range ids {
		content = append(content, anthropic.ContentBlockParamUnion{
			OfToolUse: &anthropic.ToolUseBlockParam{
				ID:    id,
				Name:  "read_file",
				Input: json.RawMessage(`{}`),
			},
		})
	}
	return anthropic.MessageParam{Role: anthropic.MessageParamRoleAssistant, Content: content}
}

// TestRepairPutsSyntheticResultBeforeExistingText is the reported defect. The
// message that answers the dangling call already carries the user's own text,
// so appending the synthetic result puts text first and the whole request is
// refused.
func TestRepairPutsSyntheticResultBeforeExistingText(t *testing.T) {
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("read the file")),
		assistantWithToolUses("tool-1"),
		anthropic.NewUserMessage(anthropic.NewTextBlock("actually, never mind")),
	}

	repaired, repairs := RepairDanglingToolUse(msgs)

	if repairs != 1 {
		t.Fatalf("expected 1 repair, got %d", repairs)
	}
	if len(repaired) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(repaired))
	}
	assertToolResultsComeFirst(t, repaired[2])
}

// TestRepairFromTurnPrepareOrdering walks the sequence engine.prepareInput
// actually produces: the conversation ends on an assistant message whose tool
// call never came back, the user's next input is appended as a fresh user
// message, and only then does the repair run. That is the live path this
// defect reaches the API on.
func TestRepairFromTurnPrepareOrdering(t *testing.T) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("read the file")),
		assistantWithToolUses("tool-1"),
	}

	// engine/turn_prepare.go: the last message is an assistant message, so the
	// user's input becomes a new message rather than merging into an existing
	// one.
	messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock("stop, do this instead")))

	repaired, repairs := RepairDanglingToolUse(messages)
	if repairs != 1 {
		t.Fatalf("expected 1 repair, got %d", repairs)
	}
	assertToolResultsComeFirst(t, repaired[len(repaired)-1])

	// The user's words must survive the reordering — moving the result to the
	// head must not drop what the turn was about.
	var sawText bool
	for _, block := range repaired[len(repaired)-1].Content {
		if block.OfText != nil && block.OfText.Text == "stop, do this instead" {
			sawText = true
		}
	}
	if !sawText {
		t.Fatalf("the user's input was lost: %s", describeBlocks(repaired[len(repaired)-1].Content))
	}
}

// TestRepairKeepsRealResultsAheadOfTextToo covers a partial interruption: one
// call came back, one did not, and the message also carries text. All results
// belong ahead of the text, the real one no less than the synthetic one.
func TestRepairKeepsRealResultsAheadOfTextToo(t *testing.T) {
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("read two files")),
		assistantWithToolUses("tool-1", "tool-2"),
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{ToolUseID: "tool-1"}},
				anthropic.NewTextBlock("and then summarise them"),
			},
		},
	}

	repaired, repairs := RepairDanglingToolUse(msgs)
	if repairs != 1 {
		t.Fatalf("expected 1 repair, got %d", repairs)
	}
	assertToolResultsComeFirst(t, repaired[2])
}

// TestRepairSynthesisesResultsInToolUseOrder pins the order the synthetic
// blocks come out in. It used to be a map range, so two runs over the same
// conversation could produce two different prompts — which is a cache miss on
// a resume, and a flaky assertion for anyone testing the repair.
func TestRepairSynthesisesResultsInToolUseOrder(t *testing.T) {
	ids := []string{"tool-a", "tool-b", "tool-c", "tool-d", "tool-e"}

	for attempt := 0; attempt < 20; attempt++ {
		msgs := []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("do five things")),
			assistantWithToolUses(ids...),
			anthropic.NewUserMessage(anthropic.NewTextBlock("carry on")),
		}

		repaired, repairs := RepairDanglingToolUse(msgs)
		if repairs != len(ids) {
			t.Fatalf("expected %d repairs, got %d", len(ids), repairs)
		}

		var got []string
		for _, block := range repaired[2].Content {
			if block.OfToolResult != nil {
				got = append(got, block.OfToolResult.ToolUseID)
			}
		}
		if len(got) != len(ids) {
			t.Fatalf("expected %d tool_results, got %d", len(ids), len(got))
		}
		for i, id := range ids {
			if got[i] != id {
				t.Fatalf("attempt %d: tool_result %d is %s, want %s (order: %v)", attempt, i, got[i], id, got)
			}
		}
	}
}
