package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// TestAnOpenAIReplyCutOffByLengthIsAnErrorAndKeepsItsText: finish_reason
// "length" maps to max_tokens, which this loop used to return as a success.
func TestAnOpenAIReplyCutOffByLengthIsAnErrorAndKeepsItsText(t *testing.T) {
	response := textResponse("the first half of an ans")
	response.Choices[0].FinishReason = "length"
	f := newFakeOpenAI(t, response)
	e := openAIEngine(t, f)
	e.Messages = []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))}

	result, err := e.runOpenAITurn(context.Background(), nil)

	if !errors.Is(err, agent.ErrReplyCutOffAtOutputLimit) {
		t.Fatalf("err = %v, want ErrReplyCutOffAtOutputLimit", err)
	}
	if !result.Incomplete || result.Text != "the first half of an ans" {
		t.Fatalf("Incomplete %v, Text %q; want the cut-off text, marked incomplete", result.Incomplete, result.Text)
	}
	if len(e.Messages) != 2 || len(e.Messages[1].Content) != 2 {
		t.Fatalf("conversation = %+v, want the user message then the text and a cut-off notice", e.Messages)
	}
}

// TestAReplyCutOffAtTheOutputLimitDoesNotMarkTheModelUnhealthy: the limit is
// the MaxTokens inber set, not a fault in the model.
func TestAReplyCutOffAtTheOutputLimitDoesNotMarkTheModelUnhealthy(t *testing.T) {
	e, store := engineWithTempModelStore(t)
	const model = "claude-sonnet-4-5"
	healthyModel(t, e, model)

	e.recordModelHealth(context.Background(), model, 900, agent.ErrReplyCutOffAtOutputLimit)

	if after := store.GetHealth(model); !after.IsHealthy(healthWindow) || !after.LastErrorAt.IsZero() {
		t.Fatalf("a cut-off reply was recorded against %s (last error %q)", model, after.LastError)
	}
}
