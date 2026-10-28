package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeCodexAuthFile writes an auth file shaped like the Codex CLI's, holding
// a token that expires at expiresAt, and returns its path and the token.
func writeCodexAuthFile(t *testing.T, authMode string, expiresAt time.Time) (string, string) {
	t.Helper()
	claims, _ := json.Marshal(map[string]any{"exp": expiresAt.Unix()})
	token := "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(claims) + ".signature"
	file, _ := json.Marshal(map[string]any{
		"auth_mode": authMode,
		"tokens":    map[string]any{"access_token": token, "account_id": "account-1"},
	})
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, file, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, token
}

// fakeResponsesEndpoint records each request body and answers with the given
// server-sent events.
type fakeResponsesEndpoint struct {
	server        *httptest.Server
	bodies        []map[string]any
	authorization []string
}

func newFakeResponsesEndpoint(t *testing.T, events ...string) *fakeResponsesEndpoint {
	t.Helper()
	f := &fakeResponsesEndpoint{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.bodies = append(f.bodies, body)
		f.authorization = append(f.authorization, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range events {
			fmt.Fprintf(w, "event: x\ndata: %s\n\n", event)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

const completedEvent = `{"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed","usage":{"input_tokens":120,"input_tokens_details":{"cached_tokens":100},"output_tokens":7,"total_tokens":127}}}`

func TestARequestThroughTheCodexSubscriptionCarriesEveryMessageAsAResponsesItem(t *testing.T) {
	authFile, token := writeCodexAuthFile(t, "chatgpt", time.Now().Add(time.Hour))
	f := newFakeResponsesEndpoint(t,
		`{"type":"response.output_item.done","item":{"type":"message","content":[{"type":"output_text","text":"done"}]}}`,
		completedEvent)
	client := NewOpenAIClientThroughCodexSubscription(f.server.URL, authFile, "gpt-test")

	_, err := client.ChatCompletion(context.Background(), OpenAIRequest{
		Messages: []OpenAIMessage{
			{Role: "system", Content: "be brief"},
			{Role: "user", Content: "read a.txt"},
			{Role: "assistant", Content: "Reading it.", ToolCalls: []OpenAIToolCall{{ID: "call_1", Type: "function", Function: OpenAIFunctionCall{Name: "read_it", Arguments: `{"path":"a.txt"}`}}}},
			{Role: "tool", ToolCallID: "call_1", Content: "contents"},
		},
		Tools:               []OpenAITool{{Type: "function", Function: OpenAIFunctionSchema{Name: "read_it", Description: "reads", Parameters: map[string]interface{}{"type": "object"}}}},
		MaxCompletionTokens: 16384,
		PromptCacheKey:      "session-1",
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	if f.authorization[0] != "Bearer "+token {
		t.Errorf("the request was not sent with the token from the auth file")
	}
	got, _ := json.Marshal(f.bodies[0])
	want := `{"input":[` +
		`{"content":[{"text":"read a.txt","type":"input_text"}],"role":"user","type":"message"},` +
		`{"content":[{"text":"Reading it.","type":"output_text"}],"role":"assistant","type":"message"},` +
		`{"arguments":"{\"path\":\"a.txt\"}","call_id":"call_1","name":"read_it","type":"function_call"},` +
		`{"call_id":"call_1","output":"contents","type":"function_call_output"}],` +
		`"instructions":"be brief","max_output_tokens":16384,"model":"gpt-test","prompt_cache_key":"session-1","store":false,"stream":true,` +
		`"tools":[{"description":"reads","name":"read_it","parameters":{"type":"object"},"type":"function"}]}`
	if string(got) != want {
		t.Errorf("request body\n got %s\nwant %s", got, want)
	}
}

func TestAReplyFromTheCodexSubscriptionComesBackAsAChatCompletion(t *testing.T) {
	authFile, _ := writeCodexAuthFile(t, "chatgpt", time.Now().Add(time.Hour))
	f := newFakeResponsesEndpoint(t,
		`{"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		`{"type":"response.output_item.done","item":{"type":"reasoning","summary":[]}}`,
		`{"type":"response.output_item.done","item":{"type":"message","content":[{"type":"output_text","text":"Reading it."}]}}`,
		`{"type":"response.output_item.done","item":{"type":"function_call","call_id":"call_9","name":"read_it","arguments":"{\"path\":\"b.txt\"}"}}`,
		completedEvent)
	client := NewOpenAIClientThroughCodexSubscription(f.server.URL, authFile, "gpt-test")

	reply, err := client.ChatCompletion(context.Background(), OpenAIRequest{Messages: []OpenAIMessage{{Role: "user", Content: "go"}}})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	choice := reply.Choices[0]
	if choice.Message.Content != "Reading it." {
		t.Errorf("text = %q", choice.Message.Content)
	}
	if len(choice.Message.ToolCalls) != 1 || choice.Message.ToolCalls[0].ID != "call_9" ||
		choice.Message.ToolCalls[0].Function.Name != "read_it" || choice.Message.ToolCalls[0].Function.Arguments != `{"path":"b.txt"}` {
		t.Errorf("tool calls = %+v", choice.Message.ToolCalls)
	}
	if choice.FinishReason != "tool_calls" {
		t.Errorf("finish reason = %q, want tool_calls", choice.FinishReason)
	}
	if reply.ID != "resp_1" || reply.Usage.PromptTokens != 120 || reply.Usage.CompletionTokens != 7 || reply.Usage.PromptTokensDetails.CachedTokens != 100 {
		t.Errorf("id/usage = %q %+v", reply.ID, reply.Usage)
	}
}

func TestAnIncompleteResponseFromTheCodexSubscriptionEndsWithFinishReasonLength(t *testing.T) {
	authFile, _ := writeCodexAuthFile(t, "chatgpt", time.Now().Add(time.Hour))
	f := newFakeResponsesEndpoint(t,
		`{"type":"response.output_item.done","item":{"type":"message","content":[{"type":"output_text","text":"partial"}]}}`,
		`{"type":"response.incomplete","response":{"id":"resp_2","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`)
	client := NewOpenAIClientThroughCodexSubscription(f.server.URL, authFile, "gpt-test")

	reply, err := client.ChatCompletion(context.Background(), OpenAIRequest{Messages: []OpenAIMessage{{Role: "user", Content: "go"}}})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if reply.Choices[0].FinishReason != "length" {
		t.Errorf("finish reason = %q, want length", reply.Choices[0].FinishReason)
	}
}

func TestTheCodexSubscriptionClientFailsLoudWhereItCannotSendOrRead(t *testing.T) {
	request := OpenAIRequest{Messages: []OpenAIMessage{{Role: "user", Content: "go"}}}

	t.Run("expired token sends nothing", func(t *testing.T) {
		authFile, _ := writeCodexAuthFile(t, "chatgpt", time.Now().Add(-time.Minute))
		f := newFakeResponsesEndpoint(t, completedEvent)
		_, err := NewOpenAIClientThroughCodexSubscription(f.server.URL, authFile, "m").ChatCompletion(context.Background(), request)
		if !errors.Is(err, ErrCodexSubscriptionTokenExpired) {
			t.Errorf("err = %v, want ErrCodexSubscriptionTokenExpired", err)
		}
		if len(f.bodies) != 0 {
			t.Errorf("a request went out on an expired token")
		}
	})
	t.Run("auth file signed in with an API key", func(t *testing.T) {
		authFile, _ := writeCodexAuthFile(t, "apikey", time.Now().Add(time.Hour))
		f := newFakeResponsesEndpoint(t, completedEvent)
		_, err := NewOpenAIClientThroughCodexSubscription(f.server.URL, authFile, "m").ChatCompletion(context.Background(), request)
		if err == nil || !strings.Contains(err.Error(), "not a ChatGPT subscription") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("stream ends before the response completes", func(t *testing.T) {
		authFile, _ := writeCodexAuthFile(t, "chatgpt", time.Now().Add(time.Hour))
		f := newFakeResponsesEndpoint(t, `{"type":"response.output_item.done","item":{"type":"message","content":[{"type":"output_text","text":"cut"}]}}`)
		_, err := NewOpenAIClientThroughCodexSubscription(f.server.URL, authFile, "m").ChatCompletion(context.Background(), request)
		if err == nil || !strings.Contains(err.Error(), "before the response completed") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("response failed", func(t *testing.T) {
		authFile, _ := writeCodexAuthFile(t, "chatgpt", time.Now().Add(time.Hour))
		f := newFakeResponsesEndpoint(t, `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"slow down"}}}`)
		_, err := NewOpenAIClientThroughCodexSubscription(f.server.URL, authFile, "m").ChatCompletion(context.Background(), request)
		if err == nil || !strings.Contains(err.Error(), "slow down") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("content that is not a string", func(t *testing.T) {
		authFile, _ := writeCodexAuthFile(t, "chatgpt", time.Now().Add(time.Hour))
		f := newFakeResponsesEndpoint(t, completedEvent)
		_, err := NewOpenAIClientThroughCodexSubscription(f.server.URL, authFile, "m").ChatCompletion(context.Background(),
			OpenAIRequest{Messages: []OpenAIMessage{{Role: "user", Content: []contentPart{{Type: "image"}}}}})
		if err == nil || len(f.bodies) != 0 {
			t.Errorf("err = %v, requests sent = %d; want an error and nothing sent", err, len(f.bodies))
		}
	})
}

func TestAnOpenAIModelIsServedThroughTheCodexSubscriptionWhenItsAuthFileIsConfigured(t *testing.T) {
	client, err := NewModelClient("gpt-5.6-terra", nil, nil, ProviderCredentials{
		OpenAI:                          "sk-should-not-be-used",
		OpenAICodexSubscriptionAuthFile: "/configured/auth.json",
	})
	if err != nil {
		t.Fatalf("NewModelClient: %v", err)
	}
	if client.Provider != "openai" || client.OpenAIClient == nil {
		t.Fatalf("provider %q, OpenAI client %v", client.Provider, client.OpenAIClient)
	}
	if client.OpenAIClient.CodexSubscriptionAuthFilePath != "/configured/auth.json" ||
		client.OpenAIClient.BaseURL != CodexSubscriptionResponsesEndpoint || client.OpenAIClient.APIKey != "" {
		t.Errorf("client = %+v; want the subscription endpoint and auth file, and no API key", client.OpenAIClient)
	}
}
