package engine

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/conversation"
)

// restoredTranscript builds a persisted-looking history: `turns` user/assistant
// pairs, every user message carrying a tool result long enough and old enough
// to be summarized or dropped once it is treated as staging. It ends on an
// assistant message, as a cleanly persisted session does.
func restoredTranscript(turns int) []anthropic.MessageParam {
	var messages []anthropic.MessageParam
	for i := 0; i < turns; i++ {
		body := "tool result " + string(rune('a'+i)) + ": " +
			strings.Repeat("the quick brown fox jumps over the lazy dog. ", 10)
		messages = append(messages,
			anthropic.NewUserMessage(anthropic.NewToolResultBlock(string(rune('a'+i)), body, false)),
			anthropic.NewAssistantMessage(anthropic.NewTextBlock("reply")),
		)
	}
	return messages
}

func toolResultTexts(messages []anthropic.MessageParam) []string {
	var out []string
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolResult == nil {
				continue
			}
			var text string
			for _, c := range block.OfToolResult.Content {
				if c.OfText != nil {
					text += c.OfText.Text
				}
			}
			out = append(out, text)
		}
	}
	return out
}

func TestRestoreMessages_FreezesRestoredHistory(t *testing.T) {
	messages := restoredTranscript(12)
	e := &Engine{}
	e.RestoreMessages(messages)

	if e.staged == nil {
		t.Fatal("RestoreMessages left e.staged nil")
	}
	want := conversation.FreezePoint(messages)
	if e.staged.FrozenIdx != want {
		t.Fatalf("FrozenIdx = %d, want %d (the whole restored transcript)", e.staged.FrozenIdx, want)
	}
	if e.staged.TurnsSinceFlush != 0 {
		t.Fatalf("TurnsSinceFlush = %d, want 0 — a restore is a flush point", e.staged.TurnsSinceFlush)
	}
	if len(e.Messages) != len(messages) {
		t.Fatalf("Messages len = %d, want %d", len(e.Messages), len(messages))
	}
}

// The defect this method exists to prevent: the first turn after a resume ran
// dedup and age-based tool-result pruning over the ENTIRE restored transcript,
// dropping results without the memory write a real prune performs.
func TestRestoreMessages_FirstResumedTurnDoesNotPruneHistory(t *testing.T) {
	messages := restoredTranscript(12)
	before := toolResultTexts(messages)

	e := &Engine{}
	e.RestoreMessages(messages)
	e.pruneIfNeeded()

	after := toolResultTexts(e.Messages)
	if len(before) != len(after) {
		t.Fatalf("tool result count changed: %d → %d", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("restored tool result %d was rewritten by the first resumed turn:\n"+
				" before %.60q\n after  %.60q", i, before[i], after[i])
		}
	}
}

// The complement, so the test above cannot pass merely because pruneIfNeeded
// is inert on this fixture: assigning to e.Messages (what the resume path used
// to do) leaves FrozenIdx at 0 and the same call rewrites the history.
func TestPruneIfNeeded_AtFrozenIdxZeroRewritesRestoredHistory(t *testing.T) {
	messages := restoredTranscript(12)
	before := toolResultTexts(messages)

	e := &Engine{}
	e.Messages = messages // the old restore path
	e.pruneIfNeeded()

	after := toolResultTexts(e.Messages)
	same := len(before) == len(after)
	if same {
		for i := range before {
			if before[i] != after[i] {
				same = false
				break
			}
		}
	}
	if same {
		t.Fatal("a bare assignment left the transcript intact; the fixture no longer " +
			"reaches the prune thresholds and the test above proves nothing")
	}
}

// A resumed session interrupted mid-turn ends on a user message. That message
// is not final — the next turn merges its input into it — so it must stay in
// staging or the frozen zone gets mutated underneath the cache breakpoint.
func TestRestoreMessages_HoldsBackTrailingUserMessage(t *testing.T) {
	messages := append(restoredTranscript(4),
		anthropic.NewUserMessage(anthropic.NewTextBlock("interrupted before the reply")))

	e := &Engine{}
	e.RestoreMessages(messages)

	if e.staged == nil {
		t.Fatal("RestoreMessages left e.staged nil")
	}
	if e.staged.FrozenIdx != len(messages)-1 {
		t.Fatalf("FrozenIdx = %d, want %d — the trailing user message must stay staged",
			e.staged.FrozenIdx, len(messages)-1)
	}
}

func TestRestoreMessages_EmptyTranscript(t *testing.T) {
	e := &Engine{}
	e.RestoreMessages(nil)
	if e.staged == nil || e.staged.FrozenIdx != 0 {
		t.Fatalf("empty restore should leave FrozenIdx 0, got %+v", e.staged)
	}
}
