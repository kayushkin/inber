package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// recordingToolLoopProvider answers with a tool call `toolCalls` times and then
// with plain text, and keeps what each request looked like on the wire. Driving
// the real Run loop is the only way to see where the breakpoints actually land:
// the anchor is set inside Run and the loop is what appends to the conversation.
type recordingToolLoopProvider struct {
	toolCalls int
	calls     int
	requests  [][]anthropic.MessageParam
}

func (p *recordingToolLoopProvider) Complete(ctx context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error) {
	// Copy the messages: the loop keeps mutating the slice it was handed.
	snapshot := make([]anthropic.MessageParam, len(params.Messages))
	copy(snapshot, params.Messages)
	p.requests = append(p.requests, snapshot)

	p.calls++
	if p.calls <= p.toolCalls {
		return &anthropic.Message{
			StopReason: anthropic.StopReasonToolUse,
			Content: []anthropic.ContentBlockUnion{{
				Type:  "tool_use",
				ID:    "call",
				Name:  "read_files",
				Input: []byte(`{"path":"x.go"}`),
			}},
		}, nil
	}
	return &anthropic.Message{
		StopReason: anthropic.StopReasonEndTurn,
		Content:    []anthropic.ContentBlockUnion{{Type: "text", Text: "done"}},
	}, nil
}

func (p *recordingToolLoopProvider) CompleteStreaming(ctx context.Context, params *anthropic.MessageNewParams) (StreamingResponse, error) {
	return nil, context.Canceled
}

// The turn anchor has to survive a whole tool loop sitting on the same message,
// otherwise every round trip writes a fresh cache entry instead of reading the
// one the turn already paid for. This drives the real loop and reads the
// breakpoints off the requests the provider received.
func TestRunKeepsTheTurnAnchorOnTheSameMessageForEveryCallOfTheTurn(t *testing.T) {
	const toolCalls = 4
	provider := &recordingToolLoopProvider{toolCalls: toolCalls}
	a := New(provider, "system")
	a.FrozenIdx = 10
	a.tools = []Tool{{
		Name: "read_files",
		Run: func(ctx context.Context, input string) (string, error) {
			return strings.Repeat("y", 4000), nil
		},
	}}

	messages := append(transcript(16), anthropic.NewUserMessage(anthropic.NewTextBlock("go")))
	anchor := len(messages) - 1

	if _, err := a.Run(context.Background(), "claude-sonnet-4-5-20250929", &messages); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(provider.requests) != toolCalls+1 {
		t.Fatalf("provider saw %d requests, want %d", len(provider.requests), toolCalls+1)
	}

	for i, request := range provider.requests {
		got := breakpointIndices(request)
		if len(got) != 2 || got[0] != 9 || got[1] != anchor {
			t.Fatalf("request %d of %d (%d messages): breakpoints at %v, want [9 %d]",
				i+1, len(provider.requests), len(request), got, anchor)
		}
	}
}

// Four is the hard limit on cache_control blocks in one request. The tools array
// and the system prefix take one each, so history may never take more than two.
func TestRunNeverSendsMoreThanFourCacheControlBlocks(t *testing.T) {
	provider := &recordingToolLoopProvider{toolCalls: 3}
	a := NewWithSystemBlocks(provider, []anthropic.TextBlockParam{
		{Text: "identity", CacheControl: anthropic.NewCacheControlEphemeralParam()},
	})
	a.FrozenIdx = 10
	a.tools = []Tool{{
		Name: "read_files",
		Run:  func(ctx context.Context, input string) (string, error) { return "ok", nil },
	}}

	messages := append(transcript(16), anthropic.NewUserMessage(anthropic.NewTextBlock("go")))
	if _, err := a.Run(context.Background(), "claude-sonnet-4-5-20250929", &messages); err != nil {
		t.Fatalf("run: %v", err)
	}

	for i, request := range provider.requests {
		blocks := len(breakpointIndices(request)) + 1 // + the system block above
		if blocks > 4 {
			t.Fatalf("request %d carries %d cache_control blocks, the API allows 4", i+1, blocks)
		}
	}
}
