package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// The defect these tests pin. Both places that call BeforeRequest used to adopt
// its answer only when the slice came back shorter, and the prune path cannot
// make the slice shorter except by dropping messages off the head — which needs
// more than twice KeepRecentTurns messages. Inside that band the pruner freed
// tens of thousands of tokens per call, the request was built from the
// unpruned conversation anyway, and on the retry path the turn simply died.

// pruneRewritingContentInPlace stands in for the real pruner in the band this
// change is about: it truncates every text block and returns exactly as many
// messages as it was given. Only the freed count distinguishes it from a
// pruner that did nothing, which is the entire point.
func pruneRewritingContentInPlace(tokensFreed int) func(context.Context, []anthropic.MessageParam, int) ([]anthropic.MessageParam, int) {
	return func(_ context.Context, messages []anthropic.MessageParam, _ int) ([]anthropic.MessageParam, int) {
		rewritten := make([]anthropic.MessageParam, len(messages))
		for i, msg := range messages {
			blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Content))
			for _, block := range msg.Content {
				if block.OfText != nil {
					blocks = append(blocks, anthropic.ContentBlockParamUnion{
						OfText: &anthropic.TextBlockParam{Text: prunedMarker},
					})
					continue
				}
				blocks = append(blocks, block)
			}
			msg.Content = blocks
			rewritten[i] = msg
		}
		return rewritten, tokensFreed
	}
}

const prunedMarker = "[pruned]"

func conversationOfLongTextMessages(count int) []anthropic.MessageParam {
	messages := make([]anthropic.MessageParam, 0, count)
	for i := 0; i < count; i++ {
		messages = append(messages, anthropic.NewUserMessage(
			anthropic.NewTextBlock(strings.Repeat("a long line of tool output\n", 500)),
		))
	}
	return messages
}

func everyMessageWasPruned(messages []anthropic.MessageParam) bool {
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfText != nil && block.OfText.Text != prunedMarker {
				return false
			}
		}
	}
	return true
}

// TestBuildRequestAdoptsAPruneThatFreedTokensWithoutDroppingMessages is the
// defect, reproduced. Before the fix the gate read the message count, the count
// was identical, and the whole prune was discarded.
func TestBuildRequestAdoptsAPruneThatFreedTokensWithoutDroppingMessages(t *testing.T) {
	a := &Agent{}
	a.SetContextWindow(200000)
	a.SetBeforeRequest(pruneRewritingContentInPlace(78952))

	messages := conversationOfLongTextMessages(36)
	params := a.buildRequest(context.Background(), "claude-sonnet-4-5-20250929", &messages, a.prepareTools(), false)

	if !everyMessageWasPruned(params.Messages) {
		t.Error("the request went out carrying unpruned messages — the prune was discarded " +
			"because the message count did not change")
	}
	// The caller's own slice has to move too, or the next turn re-sends the
	// weight this one just freed and the freeing is paid for every turn.
	if !everyMessageWasPruned(messages) {
		t.Error("the conversation the caller holds was left unpruned, so the freed tokens " +
			"come straight back on the next request")
	}
}

// TestBuildRequestIgnoresAPruneThatFreedNothing is the control. Without it the
// fix could be "always adopt whatever BeforeRequest returns", which would let a
// pruner that achieved nothing still rewrite the live conversation.
func TestBuildRequestIgnoresAPruneThatFreedNothing(t *testing.T) {
	a := &Agent{}
	a.SetContextWindow(200000)
	a.SetBeforeRequest(pruneRewritingContentInPlace(0))

	messages := conversationOfLongTextMessages(36)
	params := a.buildRequest(context.Background(), "claude-sonnet-4-5-20250929", &messages, a.prepareTools(), false)

	if everyMessageWasPruned(params.Messages) {
		t.Error("a prune reporting 0 tokens freed was adopted anyway")
	}
	if everyMessageWasPruned(messages) {
		t.Error("a prune reporting 0 tokens freed rewrote the caller's conversation")
	}
}

// failsOnceWithContextLengthProvider fails its first call the way an over-long
// prompt does and succeeds on every call after, so a test can tell whether the
// context-length retry fired at all.
type failsOnceWithContextLengthProvider struct {
	calls    int
	lastSent []anthropic.MessageParam
}

func (p *failsOnceWithContextLengthProvider) Complete(_ context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error) {
	p.calls++
	p.lastSent = params.Messages
	if p.calls == 1 {
		return nil, errPromptTooLong
	}
	return &anthropic.Message{}, nil
}

func (p *failsOnceWithContextLengthProvider) CompleteStreaming(context.Context, *anthropic.MessageNewParams) (StreamingResponse, error) {
	return nil, errPromptTooLong
}

// TestTheContextLengthRetryFiresWhenThePruneFreedTokensWithoutDroppingMessages
// is the sharper half of the defect. On the build path a discarded prune costs
// tokens; here it costs the turn. The recovery this repo already has —
// prune harder, send again — was unreachable for any conversation the pruner
// could not shorten, which is every conversation short of the head-drop line.
func TestTheContextLengthRetryFiresWhenThePruneFreedTokensWithoutDroppingMessages(t *testing.T) {
	a := &Agent{}
	a.SetContextWindow(200000)
	a.SetBeforeRequest(pruneRewritingContentInPlace(78952))

	messages := conversationOfLongTextMessages(36)
	params := a.buildRequest(context.Background(), "claude-sonnet-4-5-20250929", &messages, a.prepareTools(), false)

	provider := &failsOnceWithContextLengthProvider{}
	a.provider = provider

	if _, err := a.executeAPICall(context.Background(), params, &messages); err != nil {
		t.Fatalf("the turn died on a context-length error the pruner had already recovered from: %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("provider called %d times, want 2 — the context-length retry never fired, so "+
			"the freed tokens bought nothing and the turn returns the raw provider string",
			provider.calls)
	}
	if !everyMessageWasPruned(provider.lastSent) {
		t.Error("the retry re-sent the same over-long conversation that had just been refused")
	}
}
