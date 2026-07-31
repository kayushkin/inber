package conversation

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestFreezePoint(t *testing.T) {
	user := anthropic.NewUserMessage(anthropic.NewTextBlock("u"))
	assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock("a"))

	cases := []struct {
		name     string
		messages []anthropic.MessageParam
		want     int
	}{
		{"empty", nil, 0},
		{"lone user message stays in staging", []anthropic.MessageParam{user}, 0},
		{"lone assistant message freezes", []anthropic.MessageParam{assistant}, 1},
		{
			"assistant-terminated transcript freezes whole",
			[]anthropic.MessageParam{user, assistant, user, assistant},
			4,
		},
		{
			"trailing user message held back",
			[]anthropic.MessageParam{user, assistant, user},
			2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FreezePoint(tc.messages); got != tc.want {
				t.Fatalf("FreezePoint = %d, want %d", got, tc.want)
			}
		})
	}
}

// A frozen boundary computed by FreezePoint must leave nothing for
// ManageStaging to mutate: that is the property the restore path relies on.
func TestFreezePoint_LeavesNothingStaged(t *testing.T) {
	messages := longToolTranscript(12)
	before := renderToolResults(messages)

	deduped := ManageStaging(messages, FreezePoint(messages), DefaultManagementConfig())
	if deduped != 0 {
		t.Fatalf("deduped %d refs in a fully frozen transcript, want 0", deduped)
	}
	if after := renderToolResults(messages); !equalStrings(before, after) {
		t.Fatalf("frozen transcript was mutated:\n before %v\n after  %v", before, after)
	}
}

// The complement: at FrozenIdx 0 the same transcript IS rewritten. Without
// this the test above could pass against a ManageStaging that does nothing.
func TestManageStaging_AtZeroRewritesWholeTranscript(t *testing.T) {
	messages := longToolTranscript(12)
	before := renderToolResults(messages)

	ManageStaging(messages, 0, DefaultManagementConfig())

	if after := renderToolResults(messages); equalStrings(before, after) {
		t.Fatal("ManageStaging at frozenIdx 0 left the transcript untouched; " +
			"the fixture no longer reaches the prune thresholds")
	}
}

// longToolTranscript builds `turns` user/assistant pairs where every user
// message carries a tool result old enough to be dropped once it ages past
// ToolResultDrop. It ends on an assistant message, like a persisted session.
func longToolTranscript(turns int) []anthropic.MessageParam {
	var messages []anthropic.MessageParam
	for i := 0; i < turns; i++ {
		id := string(rune('a' + i))
		messages = append(messages,
			anthropic.NewUserMessage(anthropic.NewToolResultBlock(id, longResultText(i), false)),
			anthropic.NewAssistantMessage(anthropic.NewTextBlock("reply")),
		)
	}
	return messages
}

func longResultText(i int) string {
	text := "tool result " + string(rune('a'+i)) + ": "
	for len(text) < 400 {
		text += "the quick brown fox jumps over the lazy dog. "
	}
	return text
}

func renderToolResults(messages []anthropic.MessageParam) []string {
	var out []string
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				out = append(out, extractToolResultContent(block.OfToolResult.Content))
			}
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
