package conversation

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
)

// A tool call that came off the wire carries its arguments as a
// json.RawMessage. Rendering that with fmt's %v prints decimal byte codes, so
// the pruned summary has to marshal instead.
func TestTruncateToolCallRendersRawMessageInputAsText(t *testing.T) {
	block := anthropic.ContentBlockParamUnion{
		OfToolUse: &anthropic.ToolUseBlockParam{
			ID:    "toolu_01",
			Name:  "spawn_agent",
			Input: json.RawMessage(`{"agent":"brigid","task":"summarise the release notes"}`),
		},
	}

	pruned := truncateToolCall(block)
	summary := prunedSummary(t, pruned)

	if strings.Contains(summary, "[123 34") {
		t.Fatalf("summary rendered the raw bytes as decimal codes: %q", summary)
	}
	if !strings.Contains(summary, "spawn_agent: ") {
		t.Fatalf("summary lost the tool name: %q", summary)
	}
	if !strings.Contains(summary, `{"agent":"brigid"`) {
		t.Fatalf("summary is not a prefix of the actual arguments: %q", summary)
	}
}

// The pruning paths themselves store a map, and a re-pruned block must stay
// readable too.
func TestTruncateToolCallRendersMapInput(t *testing.T) {
	block := anthropic.ContentBlockParamUnion{
		OfToolUse: &anthropic.ToolUseBlockParam{
			ID:    "toolu_02",
			Name:  "read_files",
			Input: map[string]interface{}{"_deduped": true},
		},
	}

	summary := prunedSummary(t, truncateToolCall(block))
	if !strings.Contains(summary, `{"_deduped":true}`) {
		t.Fatalf("map input did not render as JSON: %q", summary)
	}
}

func TestTruncateToolCallKeepsSummaryWithinTheWindow(t *testing.T) {
	long := strings.Repeat("a", 500)
	block := anthropic.ContentBlockParamUnion{
		OfToolUse: &anthropic.ToolUseBlockParam{
			ID:    "toolu_03",
			Name:  "shell_commands",
			Input: json.RawMessage(`{"command":"` + long + `"}`),
		},
	}

	summary := prunedSummary(t, truncateToolCall(block))
	if !strings.HasSuffix(summary, "...") {
		t.Fatalf("an over-long input was not truncated: %q", summary)
	}
	// tool name + ": " + 60 characters + the ellipsis
	if want := len("shell_commands") + 2 + 60 + 3; len(summary) != want {
		t.Fatalf("summary length = %d, want %d: %q", len(summary), want, summary)
	}
}

func TestToolInputTextOnNil(t *testing.T) {
	if got := ToolInputText(nil); got != "" {
		t.Fatalf("ToolInputText(nil) = %q, want empty", got)
	}
}

func TestToolInputTextReportsUnrenderableInput(t *testing.T) {
	got := ToolInputText(make(chan int))
	if !strings.HasPrefix(got, "[unrenderable tool input:") {
		t.Fatalf("an unmarshalable input rendered as %q, want a visible marker", got)
	}
}

func prunedSummary(t *testing.T, block anthropic.ContentBlockParamUnion) string {
	t.Helper()
	if block.OfToolUse == nil {
		t.Fatal("truncateToolCall dropped the tool_use block")
	}
	input, ok := block.OfToolUse.Input.(map[string]interface{})
	if !ok {
		t.Fatalf("pruned input is %T, want a summary map", block.OfToolUse.Input)
	}
	summary, ok := input["_summary"].(string)
	if !ok {
		t.Fatalf("pruned input has no _summary string: %v", input)
	}
	return summary
}

// A pruned tool call's arguments go into the next request verbatim. The cut is
// a byte budget, so a multibyte rune sitting on the boundary used to be split
// in half and the prompt carried invalid UTF-8. The padding is swept rather
// than guessed: one of these lengths puts a four-byte rune across the cut.
func TestTruncateToolCallCutsArgumentsOnARuneBoundary(t *testing.T) {
	for pad := 40; pad <= 70; pad++ {
		task := strings.Repeat("a", pad) + "\U0001F980" + strings.Repeat("b", 40)
		input, err := json.Marshal(map[string]string{"task": task})
		if err != nil {
			t.Fatal(err)
		}
		block := anthropic.ContentBlockParamUnion{
			OfToolUse: &anthropic.ToolUseBlockParam{
				ID:    "toolu_02",
				Name:  "spawn_agent",
				Input: json.RawMessage(input),
			},
		}

		summary := prunedSummary(t, truncateToolCall(block))
		if !utf8.ValidString(summary) {
			t.Fatalf("pad %d: pruned summary is not valid UTF-8: %q", pad, summary)
		}
	}
}
