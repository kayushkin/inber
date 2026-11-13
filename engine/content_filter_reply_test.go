package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// TestAnOpenAIReplyStoppedByTheContentFilterIsAnErrorAndKeepsItsText:
// finish_reason "content_filter" used to map to end_turn, so this loop
// returned a reply the provider stopped part way as a finished success.
func TestAnOpenAIReplyStoppedByTheContentFilterIsAnErrorAndKeepsItsText(t *testing.T) {
	response := textResponse("the first half of an ans")
	response.Choices[0].FinishReason = "content_filter"
	f := newFakeOpenAI(t, response)
	e := openAIEngine(t, f)
	e.Messages = []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))}

	result, err := e.runOpenAITurn(context.Background(), nil)

	if !errors.Is(err, agent.ErrReplyStoppedByContentFilter) {
		t.Fatalf("err = %v, want ErrReplyStoppedByContentFilter", err)
	}
	if !result.Incomplete || result.Text != "the first half of an ans" {
		t.Fatalf("Incomplete %v, Text %q; want the filtered text, marked incomplete", result.Incomplete, result.Text)
	}
	if len(e.Messages) != 2 || len(e.Messages[1].Content) != 2 {
		t.Fatalf("conversation = %+v, want the user message then the text and a notice", e.Messages)
	}
}

// TestAnOpenAIReplyStoppedByTheContentFilterWithNoTextStoresNothing: the
// conversation ends on the message being answered, so it stays sendable.
func TestAnOpenAIReplyStoppedByTheContentFilterWithNoTextStoresNothing(t *testing.T) {
	response := textResponse("")
	response.Choices[0].FinishReason = "content_filter"
	f := newFakeOpenAI(t, response)
	e := openAIEngine(t, f)
	e.Messages = []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))}

	result, err := e.runOpenAITurn(context.Background(), nil)

	if !errors.Is(err, agent.ErrReplyStoppedByContentFilter) {
		t.Fatalf("err = %v, want ErrReplyStoppedByContentFilter", err)
	}
	if result.Incomplete || result.Text != "" || len(e.Messages) != 1 {
		t.Fatalf("Incomplete %v, Text %q, %d messages; want nothing stored", result.Incomplete, result.Text, len(e.Messages))
	}
}

// TestAReplyStoppedByTheContentFilterDoesNotMarkTheModelUnhealthy: a filter
// is not an outage.
func TestAReplyStoppedByTheContentFilterDoesNotMarkTheModelUnhealthy(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "gpt-5"
	healthyModel(t, e, model)

	e.recordModelHealth(context.Background(), model, 900, agent.ErrReplyStoppedByContentFilter)

	if after := store.GetHealth(model); !after.IsHealthy(healthWindow) || !after.LastErrorAt.IsZero() {
		t.Fatalf("a filtered reply was recorded against %s (last error %q)", model, after.LastError)
	}
}
