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

// breakpointIndices returns the indices of every message carrying a history
// cache breakpoint, in order.
func breakpointIndices(messages []anthropic.MessageParam) []int {
	var found []int
	for i := range messages {
		for j := range messages[i].Content {
			b := messages[i].Content[j]
			marked := (b.OfText != nil && b.OfText.CacheControl.Type != "") ||
				(b.OfToolUse != nil && b.OfToolUse.CacheControl.Type != "") ||
				(b.OfToolResult != nil && b.OfToolResult.CacheControl.Type != "")
			if marked {
				found = append(found, i)
			}
		}
	}
	return found
}

// breakpointIndex returns the index of the single message carrying a history
// cache breakpoint, or -1. Fails the test if more than one carries it.
func breakpointIndex(t *testing.T, messages []anthropic.MessageParam) int {
	t.Helper()
	found := breakpointIndices(messages)
	if len(found) > 1 {
		t.Fatalf("more than one cache breakpoint: %v", found)
	}
	if len(found) == 0 {
		return -1
	}
	return found[0]
}

// A resumed session's restored history is frozen, so the breakpoint belongs on
// its last message — the boundary the prefix was cached at. Before the resume
// path froze anything, frozenIdx was 0 here and the breakpoint fell on the
// legacy second-to-last slot, so the whole restored prefix was re-paid for.
func TestHistoryCacheBreakpoints_LandsOnFrozenBoundary(t *testing.T) {
	// 10 restored messages, then this turn's new user input.
	messages := append(transcript(10), anthropic.NewUserMessage(anthropic.NewTextBlock("new")))

	placeHistoryCacheBreakpoints(messages, 10, -1)

	if got := breakpointIndex(t, messages); got != 9 {
		t.Fatalf("breakpoint at message %d, want 9 (last frozen message)", got)
	}
}

func TestHistoryCacheBreakpoints_ZeroFallsBackToSecondToLast(t *testing.T) {
	messages := transcript(11)

	placeHistoryCacheBreakpoints(messages, 0, -1)

	if got := breakpointIndex(t, messages); got != 9 {
		t.Fatalf("breakpoint at message %d, want 9 (len-2 legacy fallback)", got)
	}
}

// Repeated calls must not accumulate breakpoints — the request has a hard
// four-block limit and the boundary moves every flush.
func TestHistoryCacheBreakpoints_ClearsPreviousPlacement(t *testing.T) {
	messages := transcript(12)

	placeHistoryCacheBreakpoints(messages, 4, -1)
	if got := breakpointIndex(t, messages); got != 3 {
		t.Fatalf("first placement at %d, want 3", got)
	}
	placeHistoryCacheBreakpoints(messages, 9, -1)
	if got := breakpointIndex(t, messages); got != 8 {
		t.Fatalf("second placement at %d, want 8", got)
	}
}

// The frozen boundary and the turn anchor are two different points and both are
// worth an entry: the frozen one outlives many turns, the anchor makes this
// turn's tool loop a read. Spending two of the four cache_control blocks here is
// the deliberate budget.
func TestHistoryCacheBreakpoints_MarksBothTheFrozenBoundaryAndTheTurnAnchor(t *testing.T) {
	// 10 frozen, 6 staging, then this turn's input at index 16.
	messages := append(transcript(16), anthropic.NewUserMessage(anthropic.NewTextBlock("go")))

	placeHistoryCacheBreakpoints(messages, 10, 16)

	got := breakpointIndices(messages)
	want := []int{9, 16}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("breakpoints at %v, want %v", got, want)
	}
}

// A request may carry four cache_control blocks in total, and the tools array
// and the system prefix already hold one each. History never gets a third.
func TestHistoryCacheBreakpoints_NeverSpendMoreThanTwoOfTheFourBlocks(t *testing.T) {
	for _, tc := range []struct{ frozen, anchor int }{
		{0, -1}, {10, -1}, {0, 16}, {10, 16}, {10, 9}, {10, 3}, {99, 99},
	} {
		messages := append(transcript(16), anthropic.NewUserMessage(anthropic.NewTextBlock("go")))
		placeHistoryCacheBreakpoints(messages, tc.frozen, tc.anchor)
		if got := breakpointIndices(messages); len(got) > 2 {
			t.Fatalf("frozen=%d anchor=%d placed %d breakpoints (%v), want at most 2",
				tc.frozen, tc.anchor, len(got), got)
		}
	}
}

// An anchor at or behind the frozen boundary is the same prefix twice. One entry
// covers it; a second would buy nothing and cost a block.
func TestHistoryCacheBreakpoints_AnchorBehindTheFrozenBoundaryCollapsesToOne(t *testing.T) {
	messages := transcript(17)

	placeHistoryCacheBreakpoints(messages, 10, 4)

	if got := breakpointIndex(t, messages); got != 9 {
		t.Fatalf("breakpoint at %d, want 9 (the frozen boundary alone)", got)
	}
}

// Across one turn's tool loop the anchor must not move: that is the entire win.
// The frozen boundary does not move either, so both indices stay where the turn
// started while the loop appends.
func TestHistoryCacheBreakpoints_StayPutWhileAToolLoopAppends(t *testing.T) {
	const frozen, anchor = 10, 16
	for round := 0; round < 5; round++ {
		messages := append(transcript(17), transcript(round*2)...)
		placeHistoryCacheBreakpoints(messages, frozen, anchor)
		got := breakpointIndices(messages)
		if len(got) != 2 || got[0] != 9 || got[1] != anchor {
			t.Fatalf("round %d: breakpoints at %v, want [9 %d]", round, got, anchor)
		}
	}
}

// Pruning is the only thing that shortens the conversation and it drops whole
// messages off the head, so both indices have to move back by that many or the
// breakpoints land on the wrong messages for the rest of the turn.
func TestShiftAfterHeadDropKeepsBreakpointsOnTheSameMessages(t *testing.T) {
	a := New(nil, "")
	a.FrozenIdx = 10
	a.turnAnchorIdx = 16

	full := append(transcript(16), anthropic.NewUserMessage(anthropic.NewTextBlock("go")))
	dropped := 6
	kept := full[dropped:]

	a.shiftBreakpointIndicesAfterHeadDrop(dropped)
	placeHistoryCacheBreakpoints(kept, a.FrozenIdx, a.turnAnchorIdx)

	got := breakpointIndices(kept)
	want := []int{9 - dropped, 16 - dropped}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("after dropping %d from the head, breakpoints at %v, want %v", dropped, got, want)
	}
}

// A drop deep enough to swallow the turn's own input leaves no anchor to aim at.
// Placing one anyway would put it on a message the turn never started from.
func TestShiftPastTheAnchorStopsPlacingIt(t *testing.T) {
	a := New(nil, "")
	a.FrozenIdx = 4
	a.turnAnchorIdx = 6

	a.shiftBreakpointIndicesAfterHeadDrop(9)

	if a.turnAnchorIdx >= 0 {
		t.Fatalf("anchor is %d after being dropped, want negative", a.turnAnchorIdx)
	}
	if a.FrozenIdx != 0 {
		t.Fatalf("frozen boundary is %d after being dropped, want 0", a.FrozenIdx)
	}

	messages := transcript(11)
	placeHistoryCacheBreakpoints(messages, a.FrozenIdx, a.turnAnchorIdx)
	if got := breakpointIndex(t, messages); got != 9 {
		t.Fatalf("breakpoint at %d, want 9 (the no-anchor fallback)", got)
	}
}
