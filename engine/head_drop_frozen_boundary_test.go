package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
)

// The defect these tests pin: configureContextPruning hard-drops messages off the
// head of the conversation, and nothing moved the frozen/staging boundary the drop
// invalidates. From that point on e.staged.FrozenIdx pointed `dropped` positions too
// far into the conversation, so the "staging zone" ManageStaging is free to mutate
// began inside the frozen zone — the one thing the frozen zone exists to prevent.

// conversationOfAlternatingTurns builds `turns` user/assistant pairs whose text
// identifies each message, so a test can say which message an index points at.
func conversationOfAlternatingTurns(turns int) []anthropic.MessageParam {
	messages := make([]anthropic.MessageParam, 0, turns*2)
	for turn := 0; turn < turns; turn++ {
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(fmt.Sprintf("user %d", turn))))
		messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(fmt.Sprintf("assistant %d", turn))))
	}
	return messages
}

// firstTextOf reports the leading text block of a message, for identifying it.
func firstTextOf(t *testing.T, msg anthropic.MessageParam) string {
	t.Helper()
	for _, block := range msg.Content {
		if block.OfText != nil {
			return block.OfText.Text
		}
	}
	t.Fatalf("message carries no text block: %#v", msg)
	return ""
}

func TestAHeadDropMovesTheFrozenBoundaryWithTheMessagesItPointsAt(t *testing.T) {
	pruneConfig := conversation.DefaultPruneConfig()

	// Comfortably past the hard-drop threshold of KeepRecentTurns*2 messages.
	messages := conversationOfAlternatingTurns(pruneConfig.KeepRecentTurns + 20)

	// A boundary that survives the drop: the hard drop keeps the last
	// KeepRecentTurns*2 messages, so anything inside that tail is still there
	// afterwards and the test can name the message it points at.
	const frozenBoundaryBefore = 60

	engine := &Engine{staged: conversation.NewStagedConversation(pruneConfig.ManageInterval)}
	engine.staged.FrozenIdx = frozenBoundaryBefore
	frozenBoundaryMessageBefore := firstTextOf(t, messages[frozenBoundaryBefore])

	testAgent := &agent.Agent{}
	engine.configureContextPruning(testAgent)

	// A huge context window keeps ShouldPrune's token branch out of it: this test is
	// about the message-count hard drop, which fires on count alone.
	shortened := testAgent.BeforeRequest(context.Background(), messages, 1<<24)

	dropped := len(messages) - len(shortened)
	if dropped == 0 {
		t.Fatalf("expected a head drop from %d messages, got none", len(messages))
	}

	if dropped >= frozenBoundaryBefore {
		t.Fatalf("this test needs a boundary that survives the drop: dropped %d, boundary %d",
			dropped, frozenBoundaryBefore)
	}
	if engine.staged.FrozenIdx != frozenBoundaryBefore-dropped {
		t.Errorf("frozen boundary = %d after dropping %d, want %d",
			engine.staged.FrozenIdx, dropped, frozenBoundaryBefore-dropped)
	}
	if engine.staged.FrozenIdx < 0 || engine.staged.FrozenIdx >= len(shortened) {
		t.Fatalf("frozen boundary %d is outside the shortened conversation (%d messages)",
			engine.staged.FrozenIdx, len(shortened))
	}

	// The assertion that actually matters: the boundary names the same message it did
	// before, not the same position.
	if got := firstTextOf(t, shortened[engine.staged.FrozenIdx]); got != frozenBoundaryMessageBefore {
		t.Errorf("frozen boundary now points at %q, it pointed at %q before the drop",
			got, frozenBoundaryMessageBefore)
	}
}

func TestAHeadDropPastTheFrozenBoundaryCollapsesItToZero(t *testing.T) {
	pruneConfig := conversation.DefaultPruneConfig()
	messages := conversationOfAlternatingTurns(pruneConfig.KeepRecentTurns + 20)

	engine := &Engine{staged: conversation.NewStagedConversation(pruneConfig.ManageInterval)}
	// A boundary inside the region about to be dropped.
	engine.staged.FrozenIdx = 2

	testAgent := &agent.Agent{}
	engine.configureContextPruning(testAgent)
	shortened := testAgent.BeforeRequest(context.Background(), messages, 1<<24)

	if len(shortened) >= len(messages) {
		t.Fatalf("expected a head drop from %d messages, got %d", len(messages), len(shortened))
	}
	if engine.staged.FrozenIdx != 0 {
		t.Errorf("frozen boundary = %d, want 0 — every frozen message was dropped",
			engine.staged.FrozenIdx)
	}
}

func TestAConversationShortEnoughToKeepIsNotShiftedAtAll(t *testing.T) {
	pruneConfig := conversation.DefaultPruneConfig()
	// Under the KeepRecentTurns*2 message threshold, so nothing is dropped.
	messages := conversationOfAlternatingTurns(pruneConfig.KeepRecentTurns / 2)

	engine := &Engine{staged: conversation.NewStagedConversation(pruneConfig.ManageInterval)}
	engine.staged.FrozenIdx = 4

	testAgent := &agent.Agent{}
	engine.configureContextPruning(testAgent)
	unchanged := testAgent.BeforeRequest(context.Background(), messages, 1<<24)

	if len(unchanged) != len(messages) {
		t.Fatalf("expected no drop from %d messages, got %d", len(messages), len(unchanged))
	}
	if engine.staged.FrozenIdx != 4 {
		t.Errorf("frozen boundary = %d with nothing dropped, want 4", engine.staged.FrozenIdx)
	}
}

// The agent keeps a second copy of this boundary and shifts it in its own package
// (Agent.shiftBreakpointIndicesAfterHeadDrop). The two cannot share one helper —
// conversation imports agent, so agent cannot import conversation — so the rule is
// written twice and both must subtract the same count and floor at zero. The first
// two tests here assert the engine's half against `boundary-dropped` and against 0 rather
// than against whatever the code does, so a divergence shows up as a failure here
// rather than as a cache breakpoint landing in the wrong place.
