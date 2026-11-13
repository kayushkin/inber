package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// TestAnOpenAIResponseWithNoChoicesIsAnEmptyReplyAndIsNotStored: a 200 with an
// empty choices array converts to a content-less end_turn message. It used to
// be appended and returned as a success; mid-conversation that message is a
// 400 from Anthropic after any failover.
func TestAnOpenAIResponseWithNoChoicesIsAnEmptyReplyAndIsNotStored(t *testing.T) {
	f := newFakeOpenAI(t, agent.OpenAIResponse{})
	e := openAIEngine(t, f)
	e.Messages = []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))}

	_, err := e.runOpenAITurn(context.Background(), nil)

	if !errors.Is(err, agent.ErrEmptyReply) {
		t.Fatalf("err = %v, want ErrEmptyReply", err)
	}
	if len(e.Messages) != 1 {
		t.Fatalf("conversation has %d messages, want only the user message", len(e.Messages))
	}
}

// TestAnOpenAITextReplyIsStillStored is the control for the test above.
func TestAnOpenAITextReplyIsStillStored(t *testing.T) {
	f := newFakeOpenAI(t, textResponse("hi there"))
	e := openAIEngine(t, f)
	e.Messages = []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))}

	result, err := e.runOpenAITurn(context.Background(), nil)

	if err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}
	if result.Text != "hi there" || len(e.Messages) != 2 {
		t.Fatalf("text %q, %d messages; want the reply stored", result.Text, len(e.Messages))
	}
}

// TestAnEmptyReplyDoesNotMarkTheModelUnhealthy: the provider answered, so one
// empty reply must not fail every session on the host over to another model.
func TestAnEmptyReplyDoesNotMarkTheModelUnhealthy(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "claude-sonnet-4-5"
	healthyModel(t, e, model)

	e.recordModelHealth(context.Background(), model, 900, agent.ErrEmptyReply)

	if after := store.GetHealth(model); !after.IsHealthy(healthWindow) || !after.LastErrorAt.IsZero() {
		t.Fatalf("an empty reply was recorded against %s (last error %q)", model, after.LastError)
	}
}
