package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// replayStream replays a recorded event list and ends cleanly, which is what a
// response that arrived in full looks like from the agent's side.
type replayStream struct {
	events []anthropic.MessageStreamEventUnion
	index  int
}

func (s *replayStream) Next() bool {
	if s.index >= len(s.events) {
		return false
	}
	s.index++
	return true
}

func (s *replayStream) Current() anthropic.MessageStreamEventUnion { return s.events[s.index-1] }

func (s *replayStream) Err() error { return nil }

// replayProvider streams the given raw events, or answers in one piece when
// the caller has no delta hook and the agent therefore does not stream.
type replayProvider struct {
	rawEvents []string
	whole     *anthropic.Message
}

func (p *replayProvider) Complete(ctx context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error) {
	return p.whole, nil
}

func (p *replayProvider) CompleteStreaming(ctx context.Context, params *anthropic.MessageNewParams) (agent.StreamingResponse, error) {
	events := make([]anthropic.MessageStreamEventUnion, 0, len(p.rawEvents))
	for _, raw := range p.rawEvents {
		var event anthropic.MessageStreamEventUnion
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return &replayStream{events: events}, nil
}

// TestStreamedMessageIDArrivesBeforeItsDeltas is the curative test for the
// finding that nothing downstream could key an assistant delta on the message
// it belongs to. Reporting the id is only useful if it lands before the first
// delta of that message, so the order is what this asserts.
func TestStreamedMessageIDArrivesBeforeItsDeltas(t *testing.T) {
	provider := &replayProvider{rawEvents: []string{
		`{"type":"message_start","message":{"id":"msg_stream_1","role":"assistant"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"first "}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"second"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
	}}

	a := agent.New(provider, "system")

	var order []string
	var reportedIDs []string
	a.SetHooks(&agent.Hooks{
		OnMessageID: func(messageID string) {
			order = append(order, "id:"+messageID)
			reportedIDs = append(reportedIDs, messageID)
		},
		OnTextDelta: func(text string) { order = append(order, "delta:"+text) },
	})

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("say something")),
	}
	if _, err := a.Run(context.Background(), "claude-sonnet-4-5-20250929", &messages); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(order) == 0 {
		t.Fatal("no events reached the hooks — the test's premise is wrong")
	}
	if order[0] != "id:msg_stream_1" {
		t.Fatalf("first hook call was %q, want the message id before any delta (full order: %v)", order[0], order)
	}
	for _, id := range reportedIDs {
		if id != "msg_stream_1" {
			t.Errorf("reported message id %q, want the id the provider sent", id)
		}
	}
}

// TestUnstreamedResponseReportsItsMessageID covers the other half: a caller
// with no delta hook gets no stream, and the message is still named.
func TestUnstreamedResponseReportsItsMessageID(t *testing.T) {
	provider := &replayProvider{whole: &anthropic.Message{
		ID:         "msg_whole_1",
		Role:       "assistant",
		Content:    []anthropic.ContentBlockUnion{{Type: "text", Text: "done"}},
		StopReason: anthropic.StopReasonEndTurn,
	}}

	a := agent.New(provider, "system")

	var reported []string
	a.SetHooks(&agent.Hooks{
		OnMessageID: func(messageID string) { reported = append(reported, messageID) },
	})

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("say something")),
	}
	if _, err := a.Run(context.Background(), "claude-sonnet-4-5-20250929", &messages); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(reported) != 1 || reported[0] != "msg_whole_1" {
		t.Fatalf("reported %v, want exactly the one id the provider sent", reported)
	}
}

// TestUnnamedMessageIsNotReported keeps the hook honest: a provider that names
// nothing must produce no id at all rather than an empty one, so a consumer can
// tell "this harness surfaces no id" from "this message has the id \"\"".
func TestUnnamedMessageIsNotReported(t *testing.T) {
	provider := &replayProvider{rawEvents: []string{
		`{"type":"message_start","message":{"role":"assistant"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"anonymous"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
	}}

	a := agent.New(provider, "system")

	var reported []string
	a.SetHooks(&agent.Hooks{
		OnMessageID: func(messageID string) { reported = append(reported, messageID) },
		OnTextDelta: func(text string) {},
	})

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("say something")),
	}
	if _, err := a.Run(context.Background(), "claude-sonnet-4-5-20250929", &messages); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(reported) != 0 {
		t.Fatalf("reported %v, want nothing: the provider named no message", reported)
	}
}
