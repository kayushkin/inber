package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// failingStream replays a recorded event list and then reports an error, which
// is what a connection dropped mid-response looks like from the agent's side.
type failingStream struct {
	events []anthropic.MessageStreamEventUnion
	index  int
	err    error
}

func (s *failingStream) Next() bool {
	if s.index >= len(s.events) {
		return false
	}
	s.index++
	return true
}

func (s *failingStream) Current() anthropic.MessageStreamEventUnion { return s.events[s.index-1] }

func (s *failingStream) Err() error { return s.err }

// failingStreamProvider streams the given raw events and then fails.
type failingStreamProvider struct {
	rawEvents []string
	err       error
}

func (p *failingStreamProvider) Complete(ctx context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error) {
	return nil, p.err
}

func (p *failingStreamProvider) CompleteStreaming(ctx context.Context, params *anthropic.MessageNewParams) (agent.StreamingResponse, error) {
	events := make([]anthropic.MessageStreamEventUnion, 0, len(p.rawEvents))
	for _, raw := range p.rawEvents {
		var event anthropic.MessageStreamEventUnion
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return &failingStream{events: events, err: p.err}, nil
}

// runWithFailingStream drives one turn against a stream that dies after
// replaying rawEvents, and reports what the user saw through the delta hook.
func runWithFailingStream(t *testing.T, rawEvents []string, streamErr error) (*agent.TurnResult, error, []anthropic.MessageParam, string) {
	t.Helper()

	provider := &failingStreamProvider{rawEvents: rawEvents, err: streamErr}
	a := agent.New(provider, "system")

	var seenByUser strings.Builder
	a.SetHooks(&agent.Hooks{
		OnTextDelta: func(text string) { seenByUser.WriteString(text) },
	})

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("explain something long")),
	}
	result, err := a.Run(context.Background(), "claude-sonnet-4-5-20250929", &messages)
	return result, err, messages, seenByUser.String()
}

// TestStreamErrorKeepsTextTheUserSaw is the curative test for the finding that a
// stream error threw away the accumulated message: the user watched several
// paragraphs arrive and none of them reached the conversation or the result.
func TestStreamErrorKeepsTextTheUserSaw(t *testing.T) {
	streamErr := errors.New("connection reset by peer")
	result, err, messages, seenByUser := runWithFailingStream(t, []string{
		`{"type":"message_start","message":{"role":"assistant"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"The first paragraph. "}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"The second paragraph."}}`,
	}, streamErr)

	if err == nil {
		t.Fatal("Run must still report the stream error")
	}
	if !errors.Is(err, streamErr) {
		t.Fatalf("Run must wrap the stream error, got %v", err)
	}

	const want = "The first paragraph. The second paragraph."
	if seenByUser != want {
		t.Fatalf("the delta hook saw %q, want %q — the test's premise is wrong", seenByUser, want)
	}
	if result.Text != want {
		t.Errorf("result.Text = %q, want the text the user saw (%q)", result.Text, want)
	}
	if !result.Incomplete {
		t.Error("result.Incomplete must be set: the text is a cut-off answer, not a finished one")
	}

	if len(messages) != 2 {
		t.Fatalf("conversation has %d messages, want the user message plus the partial answer", len(messages))
	}
	assistant := messages[1]
	if assistant.Role != anthropic.MessageParamRoleAssistant {
		t.Fatalf("appended message has role %q, want assistant", assistant.Role)
	}
	if len(assistant.Content) != 2 {
		t.Fatalf("assistant message has %d blocks, want the text plus the cut-off notice", len(assistant.Content))
	}
	if got := assistant.Content[0].OfText; got == nil || got.Text != want {
		t.Errorf("first block = %+v, want the text the user saw unchanged", got)
	}
	notice := assistant.Content[1].OfText
	if notice == nil || !strings.Contains(notice.Text, "cut off") {
		t.Errorf("second block = %+v, want a notice marking the answer as cut off", notice)
	}
	if notice != nil && !strings.Contains(notice.Text, streamErr.Error()) {
		t.Errorf("the cut-off notice %q does not name the error that caused it", notice.Text)
	}
}

// TestStreamErrorDropsUnfinishedToolCall guards the one thing that must not be
// carried over: a tool_use block that arrived without a tool_result makes the
// next request invalid, and a call cut off mid-arguments was never dispatched.
func TestStreamErrorDropsUnfinishedToolCall(t *testing.T) {
	result, err, messages, _ := runWithFailingStream(t, []string{
		`{"type":"message_start","message":{"role":"assistant"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me look that up."}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`,
	}, errors.New("stream closed"))

	if err == nil {
		t.Fatal("Run must still report the stream error")
	}
	if result.Text != "Let me look that up." {
		t.Errorf("result.Text = %q, want the text that arrived before the tool call", result.Text)
	}

	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfToolUse != nil {
				t.Fatalf("an unfinished tool_use block reached the conversation: %+v", block.OfToolUse)
			}
		}
	}
}
