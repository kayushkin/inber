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
	// cacheControlTotals[i] is every cache_control block request i carried, on
	// all three surfaces the API counts together. Recorded here rather than
	// derived later because only Complete sees the tools and system arrays; the
	// messages snapshot below cannot show them.
	cacheControlTotals []int
}

// countCacheControlBlocks totals the cache_control markers on a request, across
// the tools array, the system prefix and the message history. The API's limit of
// four is a limit on this sum — no surface has its own budget — so a test that
// counts one surface and assumes the others, or that hardcodes their
// contribution, is not checking the limit it names.
func countCacheControlBlocks(params *anthropic.MessageNewParams) int {
	total := 0
	for _, tool := range params.Tools {
		if tool.OfTool != nil && tool.OfTool.CacheControl.Type != "" {
			total++
		}
	}
	for _, block := range params.System {
		if block.CacheControl.Type != "" {
			total++
		}
	}
	for _, message := range params.Messages {
		for _, b := range message.Content {
			if (b.OfText != nil && b.OfText.CacheControl.Type != "") ||
				(b.OfToolUse != nil && b.OfToolUse.CacheControl.Type != "") ||
				(b.OfToolResult != nil && b.OfToolResult.CacheControl.Type != "") {
				total++
			}
		}
	}
	return total
}

func (p *recordingToolLoopProvider) Complete(ctx context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error) {
	// Copy the messages: the loop keeps mutating the slice it was handed.
	snapshot := make([]anthropic.MessageParam, len(params.Messages))
	copy(snapshot, params.Messages)
	p.requests = append(p.requests, snapshot)
	p.cacheControlTotals = append(p.cacheControlTotals, countCacheControlBlocks(params))

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

// Four is the hard limit on cache_control blocks in one request, and it is a
// limit on the SUM: the tools array, the system prefix and the history share it.
// Anthropic rejects a fifth with a 400, and inber sends exactly four today, so
// the whole ceiling is spent and any new marker anywhere breaks every request.
//
// This used to compute `len(breakpointIndices(request)) + 1`, which hardcoded the
// system contribution as one and never counted the tools array at all — so it
// would have passed with three system markers and two on the tools, and the only
// thing it really bounded was history. It now counts what goes on the wire.
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

	if len(provider.cacheControlTotals) == 0 {
		t.Fatal("no requests recorded; the test asserts nothing")
	}
	for i, blocks := range provider.cacheControlTotals {
		if blocks > 4 {
			t.Fatalf("request %d carries %d cache_control blocks, the API allows 4", i+1, blocks)
		}
	}
}

// TestRunSpendsTheWholeCacheControlBudgetAndNoMore is the other half, and it is
// the one that would catch a marker going missing rather than a fifth arriving.
// Inber places one on the last tool definition, one on the last system block and
// two in history (the frozen boundary and this turn's anchor). A request carrying
// three is not a safe request that came in under the limit — it is a breakpoint
// that stopped being placed, which costs money silently.
func TestRunSpendsTheWholeCacheControlBudgetAndNoMore(t *testing.T) {
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

	if len(provider.cacheControlTotals) == 0 {
		t.Fatal("no requests recorded; the test asserts nothing")
	}
	for i, blocks := range provider.cacheControlTotals {
		if blocks != 4 {
			t.Errorf("request %d carries %d cache_control blocks, want all 4 spent (1 tools + 1 system + 2 history)", i+1, blocks)
		}
	}
}
