package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/conversation"
)

// What these tests pin: pruneIfNeeded's emergency branch asks
// conversation.EstimateTokens how big THIS conversation is
// (engine/lifecycle.go, `emergency := conversation.EstimateTokens(e.Messages) > cfg.TokenBudget*2`).
//
// The branch exists so an oversized conversation is flushed straight away rather
// than waiting out the ManageInterval. Before these tests nothing in the repo
// reached it: conversation's own suite has a thorough table for EstimateTokens,
// and no engine test asked whether the engine ever calls it. Replacing the
// argument with an empty conversation left the whole suite green while the
// emergency flush silently stopped firing — measured, not assumed
// (~/.nightly-todoworker-0829-esttok/score.py).
//
// The observable is TurnsSinceFlush. Tick() raises it on every call; Flush()
// resets it to zero. So 0 after the call means a flush happened and 1 means it
// did not, whatever pruning did to the message slice.

// conversationOfApproximateTokens builds a conversation whose EstimateTokens
// answer is at least `tokens`, in messages wide enough that the count comes from
// the text rather than from per-message overhead.
//
// EstimateTokens is roughly 4 chars to the token, so each block is sized in
// chars and the total is checked against the real estimator below rather than
// trusted from the arithmetic.
func conversationOfApproximateTokens(tokens int) []anthropic.MessageParam {
	const perMessageChars = 40000
	messages := []anthropic.MessageParam{}
	for chars := 0; chars < tokens*4; chars += perMessageChars {
		filler := strings.Repeat("word ", perMessageChars/5)
		if len(messages)%2 == 0 {
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(filler)))
		} else {
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(filler)))
		}
	}
	return messages
}

// engineAwaitingItsInterval returns an engine holding `messages`, with a staged
// conversation that has just flushed. The interval is therefore NOT elapsed, so
// ShouldFlush() is false and the emergency branch is the only thing that can
// flush. Without this the tests below would pass on the interval alone.
func engineAwaitingItsInterval(messages []anthropic.MessageParam) *Engine {
	e := &Engine{Messages: messages}
	e.staged = conversation.NewStagedConversation(conversation.DefaultPruneConfig().ManageInterval)
	e.MemStore = emptyMemoryStore{}
	return e
}

func TestAnOversizedConversationFlushesBeforeItsIntervalIsUp(t *testing.T) {
	cfg := conversation.DefaultPruneConfig()
	messages := conversationOfApproximateTokens(cfg.TokenBudget*2 + 20000)

	// Establish the fixture against the real estimator rather than against the
	// arithmetic above: if this ever stops holding, the test below is vacuous
	// and would pass for the wrong reason.
	if got := conversation.EstimateTokens(messages); got <= cfg.TokenBudget*2 {
		t.Fatalf("fixture is not oversized: EstimateTokens = %d, need > %d", got, cfg.TokenBudget*2)
	}

	e := engineAwaitingItsInterval(messages)
	if e.staged.ShouldFlush() {
		t.Fatalf("fixture is wrong: the interval is already up, so this would not test the emergency branch")
	}

	e.pruneIfNeeded(context.Background())

	if e.staged.TurnsSinceFlush != 0 {
		t.Fatalf("no flush on a conversation of %d tokens against a budget of %d: TurnsSinceFlush = %d, want 0",
			conversation.EstimateTokens(messages), cfg.TokenBudget, e.staged.TurnsSinceFlush)
	}
}

// The complement, and the reason the test above is not satisfied by a
// pruneIfNeeded that flushes unconditionally. Same interval state, same code
// path, a conversation comfortably under the threshold: nothing should flush.
func TestASmallConversationWaitsForItsInterval(t *testing.T) {
	cfg := conversation.DefaultPruneConfig()
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")),
	}

	if got := conversation.EstimateTokens(messages); got > cfg.TokenBudget*2 {
		t.Fatalf("fixture is not small: EstimateTokens = %d", got)
	}

	e := engineAwaitingItsInterval(messages)
	e.pruneIfNeeded(context.Background())

	if e.staged.TurnsSinceFlush == 0 {
		t.Fatalf("a %d-token conversation flushed with its interval not up",
			conversation.EstimateTokens(messages))
	}
}

// The discriminating pair. The two tests above bracket the threshold widely
// enough that a call site asking about the wrong thing — a fixed string, a nil
// slice, a constant — is caught. This one pins the threshold itself: the
// emergency fires on 2x budget and not on a conversation that is merely large.
//
// It is what stops a future repair from reading the budget as, say, 10x and
// still passing everything above.
func TestTheEmergencyThresholdIsTwiceTheTokenBudget(t *testing.T) {
	cfg := conversation.DefaultPruneConfig()

	cases := []struct {
		name        string
		tokens      int
		shouldFlush bool
	}{
		{"well under 2x budget", cfg.TokenBudget, false},
		{"well over 2x budget", cfg.TokenBudget*2 + 20000, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			messages := conversationOfApproximateTokens(tc.tokens)
			estimate := conversation.EstimateTokens(messages)
			if over := estimate > cfg.TokenBudget*2; over != tc.shouldFlush {
				t.Fatalf("fixture does not sit where the case says: EstimateTokens = %d, over 2x budget = %v, want %v",
					estimate, over, tc.shouldFlush)
			}

			e := engineAwaitingItsInterval(messages)
			e.pruneIfNeeded(context.Background())

			flushed := e.staged.TurnsSinceFlush == 0
			if flushed != tc.shouldFlush {
				t.Fatalf("%s (%d tokens, budget %d): flushed = %v, want %v",
					tc.name, estimate, cfg.TokenBudget, flushed, tc.shouldFlush)
			}
		})
	}
}
