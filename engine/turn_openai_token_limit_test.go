package engine

import (
	"context"
	"testing"

	"github.com/kayushkin/inber/session"
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

// TestEveryCallOfASessionCarriesTheSessionIDAsItsPromptCacheKey. Without a
// key OpenAI spread an inber session's calls across caches, and on the Codex
// subscription served 0-48% of their input from cache against Codex's ~87%.
func TestEveryCallOfASessionCarriesTheSessionIDAsItsPromptCacheKey(t *testing.T) {
	var seen []string
	f := newFakeOpenAI(t, toolCallResponse("call_1", "read_it", `{"path":"a.txt"}`), textResponse("done"))
	e := openAIEngine(t, f, recordingTool("read_it", "contents", &seen))
	sess, err := session.New(t.TempDir(), "test-model", "tester", "", nil, "")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	t.Cleanup(sess.Close)
	e.Session = sess

	if _, err := e.runOpenAITurn(context.Background(), nil); err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}
	if len(f.requests) != 2 {
		t.Fatalf("the loop made %d requests, want 2", len(f.requests))
	}
	for i, request := range f.requests {
		if request.PromptCacheKey == "" || request.PromptCacheKey != sess.SessionID() {
			t.Errorf("request %d prompt_cache_key = %q, want the session id %q", i, request.PromptCacheKey, sess.SessionID())
		}
	}
}

// TestTheInjectedSystemPromptLeadsTheAgentsOwn. It goes first because it is the
// longest part and the same for every session the caller starts, which is what
// lets a provider serve it from cache.
func TestTheInjectedSystemPromptLeadsTheAgentsOwn(t *testing.T) {
	e := &Engine{IdentityOverride: "You are the tester."}
	e.SetInjectedSystemPrompt("# Host prompt")
	blocks, err := e.BuildSystemPrompt("hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || blocks[0].Text != "# Host prompt" || blocks[1].Text != "You are the tester." {
		t.Fatalf("blocks = %+v, want the injected prompt then the identity", blocks)
	}

	bare := &Engine{}
	bare.SetInjectedSystemPrompt("# Host prompt")
	blocks, err = bare.BuildSystemPrompt("hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].Text != "# Host prompt" {
		t.Fatalf("blocks = %+v, want only the injected prompt", blocks)
	}
}
