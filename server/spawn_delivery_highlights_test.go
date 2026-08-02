package server

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func assistantToolCalls(names ...string) []anthropic.MessageParam {
	var msgs []anthropic.MessageParam
	for _, n := range names {
		msgs = append(msgs, anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{{OfToolUse: &anthropic.ToolUseBlockParam{Name: n}}},
		})
	}
	return msgs
}

func repeatNames(name string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = name
	}
	return out
}

// TestFormatTranscriptHighlightsReportsHowManyItOmitted pins the tail line.
//
// The count used to be read off the slice after it had already been cut to ten,
// so it always answered ten minus ten. A line that exists only to say how much
// is missing reported that nothing was, on every transcript long enough to
// trigger it. The text becomes the spawn's saved memory, which the parent reads
// when deciding whether to spawn again.
func TestFormatTranscriptHighlightsReportsHowManyItOmitted(t *testing.T) {
	got := formatTranscriptHighlights(assistantToolCalls(repeatNames("read_files", 14)...))

	if strings.Contains(got, "and 0 more") {
		t.Fatalf("14 tool calls, 10 shown, tail claims 0 omitted:\n%s", got)
	}
	if !strings.Contains(got, "- ... and 4 more") {
		t.Fatalf("want the tail to name 4 omitted calls, got:\n%s", got)
	}
	if lines := strings.Count(got, "\n") + 1; lines != 11 {
		t.Fatalf("want 10 highlights plus one tail line, got %d lines:\n%s", lines, got)
	}
}

// TestFormatTranscriptHighlightsKeepsEveryCallUnderTheCap is the complement: a
// transcript that fits must not grow a tail claiming an omission that did not
// happen, and must not lose an entry to an off-by-one at the boundary.
func TestFormatTranscriptHighlightsKeepsEveryCallUnderTheCap(t *testing.T) {
	for _, n := range []int{0, 1, 9, 10} {
		got := formatTranscriptHighlights(assistantToolCalls(repeatNames("ripgrep", n)...))
		if strings.Contains(got, "more") {
			t.Fatalf("%d calls fit under the cap but the render claims an omission:\n%s", n, got)
		}
		if want := strings.Count(got, "- ripgrep"); want != n {
			t.Fatalf("%d calls in, %d rendered:\n%s", n, want, got)
		}
	}
}
