package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// uncachedTailTokens estimates how many message tokens fall AFTER the last cache
// breakpoint, which is what the caller pays full price for on that API call.
// Anything at or before a breakpoint is a cache read.
func uncachedTailTokens(messages []anthropic.MessageParam) int {
	last := -1
	for _, idx := range breakpointIndices(messages) {
		if idx > last {
			last = idx
		}
	}
	chars := 0
	for i := last + 1; i < len(messages); i++ {
		for _, b := range messages[i].Content {
			switch {
			case b.OfText != nil:
				chars += len(b.OfText.Text)
			case b.OfToolResult != nil:
				for _, c := range b.OfToolResult.Content {
					if c.OfText != nil {
						chars += len(c.OfText.Text)
					}
				}
			case b.OfToolUse != nil:
				chars += 32
			}
		}
	}
	return chars / 4
}

// toolLoopTranscript builds a realistic mid-session conversation: a frozen zone,
// a staging zone of completed turns, this turn's user input, and then `round`
// completed tool round-trips on top of it.
func toolLoopTranscript(frozen, staging, round int) []anthropic.MessageParam {
	fat := strings.Repeat("x", 4000) // a file read, ~1k tokens
	var messages []anthropic.MessageParam
	for i := 0; i < frozen+staging; i++ {
		if i%2 == 0 {
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock("u"+fmt.Sprint(i))))
		} else {
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(fat)))
		}
	}
	messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock("please refactor the parser")))
	for i := 0; i < round; i++ {
		messages = append(messages, anthropic.NewAssistantMessage(
			anthropic.NewToolUseBlock(fmt.Sprintf("t%d", i), map[string]string{"path": "x.go"}, "read_files"),
		))
		messages = append(messages, anthropic.NewUserMessage(
			anthropic.NewToolResultBlock(fmt.Sprintf("t%d", i), fat, false),
		))
	}
	return messages
}

// The whole point of the turn anchor: a multi-tool-call turn only appends after
// its own user message, so every call but the first should pay for the tool loop
// and nothing else. Without the anchor the breakpoint sits behind the staging
// zone and each call re-pays for everything the earlier ones added.
//
// This is an A/B on one transcript, so the two numbers differ only by where the
// breakpoints went. Run with -v for the per-call figures.
func TestTheTurnAnchorStopsEachToolCallRePayingForTheStagingZone(t *testing.T) {
	const frozen, staging, rounds = 10, 8, 6
	anchor := frozen + staging // this turn's user input

	withAnchor, withoutAnchor := 0, 0
	for r := 0; r <= rounds; r++ {
		messages := toolLoopTranscript(frozen, staging, r)
		placeHistoryCacheBreakpoints(messages, frozen, anchor)
		anchored := uncachedTailTokens(messages)
		withAnchor += anchored

		messages = toolLoopTranscript(frozen, staging, r)
		placeHistoryCacheBreakpoints(messages, frozen, -1)
		frozenOnly := uncachedTailTokens(messages)
		withoutAnchor += frozenOnly

		t.Logf("api call %d (%2d messages): %6d tokens at full price, was %6d",
			r+1, len(messages), anchored, frozenOnly)

		if anchored > frozenOnly {
			t.Fatalf("call %d: anchoring cost more, %d > %d", r+1, anchored, frozenOnly)
		}
	}
	t.Logf("turn total: %d tokens at full price, was %d", withAnchor, withoutAnchor)

	// The first call of a turn must be a pure write: the anchor is the last
	// message, so nothing sits outside the cached prefix.
	first := toolLoopTranscript(frozen, staging, 0)
	placeHistoryCacheBreakpoints(first, frozen, anchor)
	if got := uncachedTailTokens(first); got != 0 {
		t.Fatalf("first call of the turn pays %d tokens at full price, want 0", got)
	}

	if withAnchor >= withoutAnchor {
		t.Fatalf("anchoring saved nothing: %d vs %d", withAnchor, withoutAnchor)
	}
}
