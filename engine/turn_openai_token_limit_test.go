package engine

import (
	"context"
	"testing"
)

// TestTheOutputTokenLimitTravelsInTheFieldEachProviderAccepts. OpenAI answers
// 400 to max_tokens on gpt-5 and the o-series, and accepts
// max_completion_tokens on every chat model it serves. The loop used to pick
// the field from the model's name, knew only "o1" and "o3", and so every gpt-5
// turn failed before it began. The compatible endpoints of the other
// providers speak max_tokens, so they must keep it.
func TestTheOutputTokenLimitTravelsInTheFieldEachProviderAccepts(t *testing.T) {
	for _, testCase := range []struct {
		provider                string
		wantMaxCompletionTokens bool
	}{
		{provider: "openai", wantMaxCompletionTokens: true},
		{provider: "openrouter", wantMaxCompletionTokens: false},
		{provider: "google", wantMaxCompletionTokens: false},
		{provider: "ollama", wantMaxCompletionTokens: false},
	} {
		t.Run(testCase.provider, func(t *testing.T) {
			f := newFakeOpenAI(t, textResponse("pong"))
			e := openAIEngineWithModel(t, f, "gpt-5.4-mini")
			e.modelClient.Provider = testCase.provider

			if _, err := e.runOpenAITurn(context.Background(), nil); err != nil {
				t.Fatalf("runOpenAITurn: %v", err)
			}
			if len(f.requests) != 1 {
				t.Fatalf("the loop made %d requests, want 1", len(f.requests))
			}
			request := f.requests[0]
			if testCase.wantMaxCompletionTokens {
				if request.MaxCompletionTokens == 0 || request.MaxTokens != 0 {
					t.Fatalf("max_completion_tokens=%d max_tokens=%d, want only max_completion_tokens set", request.MaxCompletionTokens, request.MaxTokens)
				}
			} else {
				if request.MaxTokens == 0 || request.MaxCompletionTokens != 0 {
					t.Fatalf("max_tokens=%d max_completion_tokens=%d, want only max_tokens set", request.MaxTokens, request.MaxCompletionTokens)
				}
			}
		})
	}
}
