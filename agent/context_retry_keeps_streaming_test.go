package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// The defect these tests pin: when a streamed call failed with a
// context-length error, the prune-and-retry sent the retry through the
// non-streaming Complete. The retried reply then emitted no text deltas and ran
// with no stream idle watchdog, on the request most likely to be slow.

type eventListStream struct {
	events []anthropic.MessageStreamEventUnion
	index  int
}

func (s *eventListStream) Next() bool {
	if s.index >= len(s.events) {
		return false
	}
	s.index++
	return true
}

func (s *eventListStream) Current() anthropic.MessageStreamEventUnion {
	return s.events[s.index-1]
}

func (s *eventListStream) Err() error { return nil }

// streamFailsOnceWithContextLengthProvider refuses its first streamed call
// as an over-long prompt does, then streams one text reply. It counts the
// calls that went to the non-streaming Complete, which a streaming turn must
// never make.
type streamFailsOnceWithContextLengthProvider struct {
	streamingCalls    int
	nonStreamingCalls int
	replyText         string
}

func (p *streamFailsOnceWithContextLengthProvider) Complete(context.Context, *anthropic.MessageNewParams) (*anthropic.Message, error) {
	p.nonStreamingCalls++
	return &anthropic.Message{}, nil
}

func (p *streamFailsOnceWithContextLengthProvider) CompleteStreaming(context.Context, *anthropic.MessageNewParams) (StreamingResponse, error) {
	p.streamingCalls++
	if p.streamingCalls == 1 {
		return nil, errPromptTooLong
	}
	textDelta, err := json.Marshal(p.replyText)
	if err != nil {
		return nil, err
	}
	rawEvents := []string{
		`{"type":"message_start","message":{"id":"msg_retry_1","role":"assistant"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":` + string(textDelta) + `}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
	}
	events := make([]anthropic.MessageStreamEventUnion, 0, len(rawEvents))
	for _, raw := range rawEvents {
		var event anthropic.MessageStreamEventUnion
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return &eventListStream{events: events}, nil
}

func TestTheContextLengthRetryStreamsWhenTheFirstAttemptStreamed(t *testing.T) {
	a := &Agent{}
	a.SetContextWindow(200000)
	a.SetBeforeRequest(pruneRewritingContentInPlace(78952))

	var deltas []string
	a.SetHooks(&Hooks{OnTextDelta: func(text string) { deltas = append(deltas, text) }})

	messages := conversationOfLongTextMessages(36)
	params := a.buildRequest(context.Background(), "claude-sonnet-4-5-20250929", &messages, a.prepareTools(), false)

	provider := &streamFailsOnceWithContextLengthProvider{replyText: "after the prune"}
	a.provider = provider

	resp, err := a.executeAPICall(context.Background(), params, &messages)
	if err != nil {
		t.Fatalf("the retry failed: %v", err)
	}
	if provider.nonStreamingCalls != 0 {
		t.Errorf("the retry went to the non-streaming Complete %d time(s): its reply sends no "+
			"deltas and runs with no idle watchdog", provider.nonStreamingCalls)
	}
	if provider.streamingCalls != 2 {
		t.Errorf("streamed %d call(s), want 2: the refused first attempt and the retry",
			provider.streamingCalls)
	}
	if len(deltas) != 1 || deltas[0] != provider.replyText {
		t.Errorf("deltas = %q, want exactly [%q] from the retried call", deltas, provider.replyText)
	}
	if resp == nil || len(resp.Content) != 1 || resp.Content[0].Text != provider.replyText {
		t.Errorf("the retried reply did not come back as the turn's response: %+v", resp)
	}
}
