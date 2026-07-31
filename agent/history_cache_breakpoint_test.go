package agent

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func transcript(n int) []anthropic.MessageParam {
	var messages []anthropic.MessageParam
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock("u")))
		} else {
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock("a")))
		}
	}
	return messages
}

// breakpointIndex returns the index of the single message carrying a history
// cache breakpoint, or -1. Fails the test if more than one carries it.
func breakpointIndex(t *testing.T, messages []anthropic.MessageParam) int {
	t.Helper()
	found := -1
	for i := range messages {
		for j := range messages[i].Content {
			b := messages[i].Content[j]
			marked := (b.OfText != nil && b.OfText.CacheControl.Type != "") ||
				(b.OfToolUse != nil && b.OfToolUse.CacheControl.Type != "") ||
				(b.OfToolResult != nil && b.OfToolResult.CacheControl.Type != "")
			if !marked {
				continue
			}
			if found != -1 {
				t.Fatalf("more than one cache breakpoint: %d and %d", found, i)
			}
			found = i
		}
	}
	return found
}

// A resumed session's restored history is frozen, so the breakpoint belongs on
// its last message — the boundary the prefix was cached at. Before the resume
// path froze anything, frozenIdx was 0 here and the breakpoint fell on the
// legacy second-to-last slot, so the whole restored prefix was re-paid for.
func TestAddHistoryCacheBreakpoint_LandsOnFrozenBoundary(t *testing.T) {
	// 10 restored messages, then this turn's new user input.
	messages := append(transcript(10), anthropic.NewUserMessage(anthropic.NewTextBlock("new")))

	addHistoryCacheBreakpoint(messages, 10)

	if got := breakpointIndex(t, messages); got != 9 {
		t.Fatalf("breakpoint at message %d, want 9 (last frozen message)", got)
	}
}

func TestAddHistoryCacheBreakpoint_ZeroFallsBackToSecondToLast(t *testing.T) {
	messages := transcript(11)

	addHistoryCacheBreakpoint(messages, 0)

	if got := breakpointIndex(t, messages); got != 9 {
		t.Fatalf("breakpoint at message %d, want 9 (len-2 legacy fallback)", got)
	}
}

// Repeated calls must not accumulate breakpoints — the request has a hard
// four-block limit and the boundary moves every flush.
func TestAddHistoryCacheBreakpoint_ClearsPreviousPlacement(t *testing.T) {
	messages := transcript(12)

	addHistoryCacheBreakpoint(messages, 4)
	if got := breakpointIndex(t, messages); got != 3 {
		t.Fatalf("first placement at %d, want 3", got)
	}
	addHistoryCacheBreakpoint(messages, 9)
	if got := breakpointIndex(t, messages); got != 8 {
		t.Fatalf("second placement at %d, want 8", got)
	}
}
