package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIClient is a lightweight client for OpenAI-compatible chat completion APIs.
type OpenAIClient struct {
	BaseURL string
	APIKey  string
	Model   string
	// CodexSubscriptionAuthFilePath, when set, sends every request through the
	// ChatGPT subscription the Codex CLI signs in with, using the token in this
	// file, and BaseURL is the subscription's Responses endpoint. APIKey is
	// then unused. See codex_subscription.go.
	CodexSubscriptionAuthFilePath string
	client                        *http.Client
}

// NewOpenAIClient creates an OpenAI-compatible client.
//
// The transport carries the same egress redaction as the Anthropic client.
// This path is easy to forget — it is a hand-rolled client rather than an SDK,
// and it serves openai, google, openrouter, ollama and the catch-all for every
// provider inber does not name — so the gate is installed on the transport,
// where every request through this client must pass whatever the call site.
func NewOpenAIClient(baseURL, apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		client: &http.Client{
			Timeout:   120 * time.Second, // Prevent infinite hangs
			Transport: EgressRedactionTransport(nil),
		},
	}
}

// NewOpenAIClientThroughCodexSubscription creates a client that sends OpenAI
// requests through the ChatGPT subscription whose token the Codex CLI keeps in
// authFilePath.
func NewOpenAIClientThroughCodexSubscription(endpoint, authFilePath, model string) *OpenAIClient {
	client := NewOpenAIClient(endpoint, "", model)
	client.CodexSubscriptionAuthFilePath = authFilePath
	return client
}

// ChatCompletion sends a chat completion request to the OpenAI-compatible API,
// or, on a client made with NewOpenAIClientThroughCodexSubscription, sends the
// same request to the subscription's Responses endpoint and returns the reply
// in chat-completion form.
func (c *OpenAIClient) ChatCompletion(ctx context.Context, req OpenAIRequest) (*OpenAIResponse, error) {
	// Default model if not specified
	if req.Model == "" {
		req.Model = c.Model
	}
	if c.CodexSubscriptionAuthFilePath != "" {
		return c.chatCompletionThroughCodexSubscription(ctx, req)
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", httpResp.StatusCode, string(respBytes))
	}

	var resp OpenAIResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}
