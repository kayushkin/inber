package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/kayushkin/inber/agent"
	modelstore "github.com/kayushkin/model-store"
)

// OpenAI's newest models (gpt-5.6-*, gpt-6-*, the *-codex models) answer 400
// to a tool request on chat completions, so an API-key OpenAI client sends to
// the Responses endpoint instead (todo 9d41f3a8). These tests run a whole tool
// turn through that client against a fake endpoint that speaks only the
// Responses wire format, and check that the tokens it reports reach the turn
// result.

// fakeResponsesAPI answers each POST with the next canned stream of
// Responses events and keeps every request body and header it was sent.
type fakeResponsesAPI struct {
	mu       sync.Mutex
	streams  [][]string
	bodies   []map[string]any
	headers  []http.Header
	paths    []string
	server   *httptest.Server
	endpoint string
}

func newFakeResponsesAPI(t *testing.T, streams ...[]string) *fakeResponsesAPI {
	t.Helper()
	f := &fakeResponsesAPI{streams: streams}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// The Responses API has no messages field; a chat-completions body
		// sent here is the defect this file guards against, so refuse it as
		// OpenAI does.
		if _, ok := body["messages"]; ok {
			http.Error(w, `{"error":{"message":"Unsupported parameter: 'messages'"}}`, http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.bodies = append(f.bodies, body)
		f.headers = append(f.headers, r.Header.Clone())
		f.paths = append(f.paths, r.URL.Path)
		if len(f.streams) == 0 {
			f.mu.Unlock()
			http.Error(w, "no canned stream left", http.StatusInternalServerError)
			return
		}
		stream := f.streams[0]
		f.streams = f.streams[1:]
		f.mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range stream {
			fmt.Fprintf(w, "event: x\ndata: %s\n\n", event)
		}
	}))
	f.endpoint = f.server.URL + "/v1/responses"
	t.Cleanup(f.server.Close)
	return f
}

func responsesCompletedEvent(input, cached, output int) string {
	return fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp","model":"gpt-5.6-terra","status":"completed","usage":{"input_tokens":%d,"input_tokens_details":{"cached_tokens":%d},"output_tokens":%d,"total_tokens":%d}}}`,
		input, cached, output, input+output)
}

// A tool turn on a Responses-only model shape: the first reply carries a
// reasoning item and a function call, the second the answer. The tool runs
// once, the second request carries the call and its output as Responses
// items, and each call's input, cached-input and output tokens reach the
// public turn result.
func TestAToolTurnThroughTheAPIKeyResponsesClientRunsAndCountsItsTokens(t *testing.T) {
	f := newFakeResponsesAPI(t,
		[]string{
			`{"type":"response.output_item.done","item":{"type":"reasoning","summary":[]}}`,
			`{"type":"response.output_item.done","item":{"type":"function_call","call_id":"call_1","name":"read_it","arguments":"{\"path\":\"a.txt\"}"}}`,
			responsesCompletedEvent(120, 100, 7),
		},
		[]string{
			`{"type":"response.output_item.done","item":{"type":"message","content":[{"type":"output_text","text":"a.txt says contents"}]}}`,
			responsesCompletedEvent(300, 250, 20),
		},
	)
	var seen []string
	e := &Engine{
		Model:      "gpt-5.6-terra",
		agentTools: []agent.Tool{recordingTool("read_it", "contents", &seen)},
		modelClient: &agent.ModelClient{
			Provider:     "openai",
			Model:        &modelstore.Model{ID: "gpt-5.6-terra", Provider: "openai"},
			OpenAIClient: agent.NewOpenAIResponsesAPIClient(f.endpoint, "sk-test", "gpt-5.6-terra"),
		},
	}

	result, err := e.RunTurn(context.Background(), "what does a.txt say?")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	if len(seen) != 1 || seen[0] != `{"path":"a.txt"}` {
		t.Fatalf("tool inputs %q, want the one call the model made", seen)
	}
	if result.Text != "a.txt says contents" {
		t.Errorf("turn text %q, want the second reply's answer", result.Text)
	}
	if len(f.bodies) != 2 {
		t.Fatalf("the turn made %d requests, want 2", len(f.bodies))
	}
	for index := range f.bodies {
		if f.paths[index] != "/v1/responses" {
			t.Errorf("request %d went to %q, want /v1/responses", index, f.paths[index])
		}
		if got := f.headers[index].Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("request %d Authorization %q, want the API key as a bearer", index, got)
		}
		if got := f.bodies[index]["model"]; got != "gpt-5.6-terra" {
			t.Errorf("request %d model %v, want gpt-5.6-terra", index, got)
		}
		if tools, _ := f.bodies[index]["tools"].([]any); len(tools) != 1 {
			t.Errorf("request %d sent %d tools, want 1", index, len(tools))
		}
	}

	var sawCall, sawOutput bool
	input, _ := f.bodies[1]["input"].([]any)
	for _, raw := range input {
		item, _ := raw.(map[string]any)
		switch item["type"] {
		case "function_call":
			sawCall = item["call_id"] == "call_1" && item["name"] == "read_it"
		case "function_call_output":
			sawOutput = item["call_id"] == "call_1" && item["output"] == "contents"
		}
	}
	if !sawCall || !sawOutput {
		t.Fatalf("second request input %v, want the function_call and its function_call_output for call_1", input)
	}

	if result.InputTokens != 420 || result.OutputTokens != 27 {
		t.Errorf("turn tokens %d in / %d out, want 420 / 27 summed over both calls", result.InputTokens, result.OutputTokens)
	}
	if len(result.APICalls) != 2 {
		t.Fatalf("%d per-call records, want 2", len(result.APICalls))
	}
	for index, want := range []agent.APICallUsage{
		{InputTokens: 120, OutputTokens: 7, CachedTokensIncludedInInputTokens: 100},
		{InputTokens: 300, OutputTokens: 20, CachedTokensIncludedInInputTokens: 250},
	} {
		got := result.APICalls[index]
		if got.InputTokens != want.InputTokens || got.OutputTokens != want.OutputTokens ||
			got.CachedTokensIncludedInInputTokens != want.CachedTokensIncludedInInputTokens {
			t.Errorf("call %d usage %+v, want %+v", index, got, want)
		}
	}
	if result.CacheReadTokens != 0 || result.CacheCreationTokens != 0 {
		t.Errorf("Anthropic cache counters set (read=%d write=%d); OpenAI's cached count is a subset of input and must not be priced as disjoint",
			result.CacheReadTokens, result.CacheCreationTokens)
	}
}
