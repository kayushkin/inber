package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

type beforeRequestProbeKey struct{}

// contextLengthErrorProvider fails every call the way an over-long prompt does,
// which is the one error that sends executeAPICall back through BeforeRequest.
type contextLengthErrorProvider struct{}

func (contextLengthErrorProvider) Complete(ctx context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error) {
	return nil, errPromptTooLong
}

func (contextLengthErrorProvider) CompleteStreaming(ctx context.Context, params *anthropic.MessageNewParams) (StreamingResponse, error) {
	return nil, errPromptTooLong
}

var errPromptTooLong = errors.New("prompt is too long: 300000 tokens > 200000 maximum")

// BeforeRequest is the hook the engine hangs conversation pruning on, and that
// pruning can summarize — an API call of its own, made on the way into another
// API call. It is only stoppable if it is handed the context of the call it is
// guarding, so this pins that it gets that context and not a fresh root.
func TestBeforeRequestReceivesTheAPICallsContext(t *testing.T) {
	a := &Agent{}
	a.SetContextWindow(200000)

	var seen context.Context
	a.SetBeforeRequest(func(ctx context.Context, messages []anthropic.MessageParam, contextWindow int) []anthropic.MessageParam {
		seen = ctx
		return messages
	})

	want := "the caller's"
	ctx := context.WithValue(context.Background(), beforeRequestProbeKey{}, want)
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
	}

	a.buildRequest(ctx, "claude-sonnet-4-5-20250929", &messages, a.prepareTools(), false)

	if seen == nil {
		t.Fatal("BeforeRequest was never called — this test proves nothing about the context it gets")
	}
	if got := seen.Value(beforeRequestProbeKey{}); got != want {
		t.Fatalf("BeforeRequest saw a context carrying %v, want the caller's %q — "+
			"pruning done for a cancelled call cannot be cancelled", got, want)
	}
}

// The same guarantee on the retry path. A context-length error re-enters
// BeforeRequest to prune harder, and that second attempt is the one most likely
// to summarize, so it is the one that most needs to stop with its call.
func TestBeforeRequestOnContextLengthRetryReceivesTheAPICallsContext(t *testing.T) {
	a := &Agent{}
	a.SetContextWindow(200000)

	var calls []context.Context
	a.SetBeforeRequest(func(ctx context.Context, messages []anthropic.MessageParam, contextWindow int) []anthropic.MessageParam {
		calls = append(calls, ctx)
		// Return the messages unchanged: shrinking them here would make the
		// retry fire a second provider call, which this agent has no provider
		// to serve.
		return messages
	})

	want := "the caller's"
	ctx := context.WithValue(context.Background(), beforeRequestProbeKey{}, want)
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
	}
	params := a.buildRequest(ctx, "claude-sonnet-4-5-20250929", &messages, a.prepareTools(), false)

	a.provider = contextLengthErrorProvider{}
	if _, err := a.executeAPICall(ctx, params, &messages); err == nil {
		t.Fatal("executeAPICall returned no error — the retry path was never reached")
	}

	if len(calls) != 2 {
		t.Fatalf("BeforeRequest called %d times, want 2 (the build and the context-length retry)", len(calls))
	}
	if got := calls[1].Value(beforeRequestProbeKey{}); got != want {
		t.Fatalf("the retry's BeforeRequest saw a context carrying %v, want the caller's %q", got, want)
	}
}
