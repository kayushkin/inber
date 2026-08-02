package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// neverFinishingProvider answers every request with the same tool call, so the
// Run loop has no way to end except its own cap.
type neverFinishingProvider struct{ calls int }

func (p *neverFinishingProvider) Complete(ctx context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error) {
	p.calls++
	return &anthropic.Message{
		StopReason: anthropic.StopReasonToolUse,
		Content: []anthropic.ContentBlockUnion{{
			Type: "tool_use", ID: "call", Name: "spin", Input: []byte(`{}`),
		}},
	}, nil
}

func (p *neverFinishingProvider) CompleteStreaming(ctx context.Context, params *anthropic.MessageNewParams) (StreamingResponse, error) {
	return nil, context.Canceled
}

// TestRunSignalsItsOwnAPICallCapAsASentinel drives the real loop into the cap and
// checks the error it produces is matchable, not just readable.
//
// engine.recordModelHealth uses errors.Is to keep this out of model-store's
// host-shared health table — hitting the cap means the provider answered every
// call, so recording it against the model fails every session on the box over to
// a fallback. A test that builds the sentinel itself cannot see a regression
// here: rewriting this line as a plain fmt.Errorf leaves the classifier correct
// and the live path broken.
func TestRunSignalsItsOwnAPICallCapAsASentinel(t *testing.T) {
	provider := &neverFinishingProvider{}
	a := New(provider, "system")
	a.tools = []Tool{{
		Name: "spin",
		Run:  func(ctx context.Context, input string) (string, error) { return "ok", nil },
	}}

	messages := []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("go"))}
	_, err := a.Run(context.Background(), "claude-sonnet-4-5-20250929", &messages)

	if err == nil {
		t.Fatalf("the loop ended on its own after %d calls; it should have hit the cap", provider.calls)
	}
	if !errors.Is(err, ErrMaxAPICallsExceeded) {
		t.Fatalf("the cap returned %q, which errors.Is cannot match against ErrMaxAPICallsExceeded — "+
			"engine.recordModelHealth will read it as a provider fault and mark the model unhealthy", err)
	}
	if got, want := err.Error(), "exceeded max API calls (50)"; got != want {
		t.Fatalf("operator-visible text changed: got %q, want %q", got, want)
	}
}
