package conversation

import (
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// toolUsingTurn builds one user turn: the user's own message, then toolCalls
// round-trips, each of which appends a user-role message carrying only tool
// results — the shape agent.Run produces where it appends a user message of
// tool results after the tool-use branch of its stop-reason switch.
func toolUsingTurn(label string, toolCalls int) []anthropic.MessageParam {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("request " + label)),
	}
	for i := 0; i < toolCalls; i++ {
		id := fmt.Sprintf("%s-%d", label, i)
		messages = append(messages,
			anthropic.NewAssistantMessage(anthropic.NewToolUseBlock(id, map[string]string{"path": "f"}, "read")),
			anthropic.NewUserMessage(anthropic.NewToolResultBlock(id, "contents", false)),
		)
	}
	messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock("answer "+label)))
	return messages
}

func toolUsingTranscript(turns, toolCallsPerTurn int) []anthropic.MessageParam {
	var messages []anthropic.MessageParam
	for i := 0; i < turns; i++ {
		messages = append(messages, toolUsingTurn(string(rune('a'+i)), toolCallsPerTurn)...)
	}
	return messages
}

func TestStartsUserTurn(t *testing.T) {
	cases := []struct {
		name string
		msg  anthropic.MessageParam
		want bool
	}{
		{"user text", anthropic.NewUserMessage(anthropic.NewTextBlock("hello")), true},
		{"tool result batch", anthropic.NewUserMessage(anthropic.NewToolResultBlock("t1", "out", false)), false},
		{
			"two tool results in one message",
			anthropic.NewUserMessage(
				anthropic.NewToolResultBlock("t1", "out", false),
				anthropic.NewToolResultBlock("t2", "out", false),
			),
			false,
		},
		{
			"tool result plus the user typing while tools ran",
			anthropic.NewUserMessage(
				anthropic.NewToolResultBlock("t1", "out", false),
				anthropic.NewTextBlock("[New message from user while you were working] stop"),
			),
			true,
		},
		{"assistant", anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")), false},
		{"empty user message counts as a turn", anthropic.MessageParam{Role: anthropic.MessageParamRoleUser}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StartsUserTurn(tc.msg); got != tc.want {
				t.Fatalf("StartsUserTurn = %v, want %v", got, tc.want)
			}
		})
	}
}

// The defect: with four tool calls per turn, counting user-role messages made
// each turn look like five, so keeping "4 turns" kept less than one of them.
func TestFindTurnBoundary_KeepsRequestedUserTurnsNotToolRoundTrips(t *testing.T) {
	messages := toolUsingTranscript(6, 4)

	splitAt := findTurnBoundary(messages, 4)

	kept := countTurns(messages[splitAt:])
	if kept != 4 {
		t.Fatalf("kept %d turns, want 4 (split at %d of %d messages)", kept, splitAt, len(messages))
	}
	if !StartsUserTurn(messages[splitAt]) {
		t.Fatalf("split at %d, which is not the start of a user turn", splitAt)
	}
}

// The boundary must land on the user's own message, never inside the turn: a
// split between a tool_use and its tool_result is the orphan case the integrity
// loop exists to undo.
func TestFindTurnBoundary_LandsOnAUserRequestWithNoOrphans(t *testing.T) {
	messages := toolUsingTranscript(5, 3)

	for keep := 1; keep <= 4; keep++ {
		splitAt := findTurnBoundary(messages, keep)
		if splitAt == 0 {
			continue
		}
		if !StartsUserTurn(messages[splitAt]) {
			t.Fatalf("keep=%d: split at %d is not a turn start", keep, splitAt)
		}
		orphans := findOrphanedToolResults(messages[splitAt:], collectToolUseIDs(messages[:splitAt]))
		if len(orphans) != 0 {
			t.Fatalf("keep=%d: split at %d orphaned %d tool results", keep, splitAt, len(orphans))
		}
	}
}

// Asking for more turns than exist keeps everything, rather than splitting at
// some tool round-trip that happens to be the Nth user-role message.
func TestFindTurnBoundary_KeepsWholeTranscriptWhenTurnsAreFewer(t *testing.T) {
	messages := toolUsingTranscript(2, 5)

	if splitAt := findTurnBoundary(messages, 8); splitAt != 0 {
		t.Fatalf("splitAt = %d, want 0 (only 2 turns exist in %d messages)", splitAt, len(messages))
	}
}

// countTurns reports what was summarized away, and that number reaches the user
// through the memory row's summary text.
func TestCountTurns_IgnoresToolRoundTrips(t *testing.T) {
	messages := toolUsingTranscript(3, 6)

	if got := countTurns(messages); got != 3 {
		t.Fatalf("countTurns = %d, want 3 (%d messages, 6 tool calls per turn)", got, len(messages))
	}
}

// A complement: on a transcript with no tool calls at all, a turn is still a
// user-role message, so the old and new counts must agree. Without this, a
// fixture that stopped producing tool results would leave the tests above
// passing for the wrong reason.
func TestCountTurns_UnchangedWithoutTools(t *testing.T) {
	var messages []anthropic.MessageParam
	for i := 0; i < 7; i++ {
		messages = append(messages,
			anthropic.NewUserMessage(anthropic.NewTextBlock("ask")),
			anthropic.NewAssistantMessage(anthropic.NewTextBlock("reply")),
		)
	}

	if got := countTurns(messages); got != 7 {
		t.Fatalf("countTurns = %d, want 7", got)
	}
	if got := findTurnBoundary(messages, 3); got != 8 {
		t.Fatalf("findTurnBoundary = %d, want 8", got)
	}
}
