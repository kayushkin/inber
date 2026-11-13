package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// scriptedStreamProvider answers the Nth streaming call with the Nth event
// list, so one turn can run a tool call and then meet the reply under test —
// the place Anthropic says an empty end_turn most often appears.
type scriptedStreamProvider struct {
	streams [][]string
	calls   int
}

// Complete answers the non-streaming call (the agent makes it when no delta
// hook is set) with the same script, accumulated the way the SDK would.
func (p *scriptedStreamProvider) Complete(ctx context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error) {
	stream, err := p.CompleteStreaming(ctx, params)
	if err != nil {
		return nil, err
	}
	var message anthropic.Message
	for stream.Next() {
		if err := message.Accumulate(stream.Current()); err != nil {
			return nil, err
		}
	}
	return &message, nil
}

func (p *scriptedStreamProvider) CompleteStreaming(ctx context.Context, params *anthropic.MessageNewParams) (agent.StreamingResponse, error) {
	if p.calls >= len(p.streams) {
		return nil, errors.New("scriptedStreamProvider has no stream left")
	}
	raw := p.streams[p.calls]
	p.calls++
	events := make([]anthropic.MessageStreamEventUnion, 0, len(raw))
	for _, frame := range raw {
		var event anthropic.MessageStreamEventUnion
		if err := json.Unmarshal([]byte(frame), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return &failingStream{events: events}, nil
}

var toolUseStream = []string{
	`{"type":"message_start","message":{"role":"assistant"}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"look","input":{}}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`,
}

// runToolThen runs one turn: a tool call, then the given second reply.
func runToolThen(t *testing.T, secondReply []string) (*agent.TurnResult, error, []anthropic.MessageParam) {
	t.Helper()
	provider := &scriptedStreamProvider{streams: [][]string{toolUseStream, secondReply}}
	a := agent.New(provider, "system")
	a.AddTool(agent.Tool{
		Name:        "look",
		Description: "look",
		InputSchema: anthropic.ToolInputSchemaParam{Properties: map[string]any{}},
		Run:         func(ctx context.Context, input string) (string, error) { return "seen", nil },
	})
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("look at it")),
	}
	result, err := a.Run(context.Background(), "claude-sonnet-4-5-20250929", &messages)
	if provider.calls != 2 {
		t.Fatalf("the provider was called %d times, want 2 — the turn did not reach the reply under test (err %v)", provider.calls, err)
	}
	return result, err, messages
}

// assertConversationIsSendable checks the property the defect broke: no
// assistant message with nothing in it, and the conversation ends on the
// tool result the model was answering.
func assertConversationIsSendable(t *testing.T, messages []anthropic.MessageParam) {
	t.Helper()
	for i, message := range messages {
		if len(message.Content) == 0 {
			t.Fatalf("message %d (%s) has no content — the next request is a 400", i, message.Role)
		}
	}
	if len(messages) != 3 {
		t.Fatalf("conversation has %d messages, want user, tool_use, tool_result", len(messages))
	}
	if last := messages[len(messages)-1]; last.Role != anthropic.MessageParamRoleUser || last.Content[0].OfToolResult == nil {
		t.Fatalf("conversation ends on %+v, want the tool result", last)
	}
}

// TestAnEmptyEndTurnAfterAToolResultIsAnErrorAndIsNotStored is the defect: the
// empty reply was appended and returned as a successful turn with no text.
func TestAnEmptyEndTurnAfterAToolResultIsAnErrorAndIsNotStored(t *testing.T) {
	result, err, messages := runToolThen(t, []string{
		`{"type":"message_start","message":{"role":"assistant"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
	})

	if !errors.Is(err, agent.ErrEmptyReply) {
		t.Fatalf("err = %v, want ErrEmptyReply", err)
	}
	if result.Text != "" {
		t.Errorf("result.Text = %q, want empty", result.Text)
	}
	if len(result.APICalls) != 2 {
		t.Errorf("recorded %d API calls, want 2 — the empty reply was still billed", len(result.APICalls))
	}
	assertConversationIsSendable(t, messages)
}

// TestABlankOrThinkingOnlyEndTurnIsEmptyToo: whitespace text answers nothing,
// and neither does thinking alone.
func TestABlankOrThinkingOnlyEndTurnIsEmptyToo(t *testing.T) {
	cases := map[string][]string{
		"blank text": {
			`{"type":"message_start","message":{"role":"assistant"}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" \n"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
		},
		"thinking only": {
			`{"type":"message_start","message":{"role":"assistant"}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"nothing to add"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`,
		},
	}
	for name, reply := range cases {
		t.Run(name, func(t *testing.T) {
			_, err, messages := runToolThen(t, reply)
			if !errors.Is(err, agent.ErrEmptyReply) {
				t.Fatalf("err = %v, want ErrEmptyReply", err)
			}
			assertConversationIsSendable(t, messages)
		})
	}
}

// TestATextEndTurnAfterAToolResultIsStillAnAnswer is the control: a guard
// that refused every reply would pass both tests above.
func TestATextEndTurnAfterAToolResultIsStillAnAnswer(t *testing.T) {
	result, err, messages := runToolThen(t, []string{
		`{"type":"message_start","message":{"role":"assistant"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"It says seen."}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`,
	})
	if err != nil {
		t.Fatalf("a text reply was refused: %v", err)
	}
	if result.Text != "It says seen." {
		t.Errorf("result.Text = %q", result.Text)
	}
	if len(messages) != 4 {
		t.Fatalf("conversation has %d messages, want the answer appended as the fourth", len(messages))
	}
}
