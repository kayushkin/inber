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
	client  *http.Client
}

// NewOpenAIClient creates an OpenAI-compatible client.
func NewOpenAIClient(baseURL, apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		client: &http.Client{
			Timeout: 120 * time.Second, // Prevent infinite hangs
		},
	}
}

// ChatCompletion sends a chat completion request to the OpenAI-compatible API.
func (c *OpenAIClient) ChatCompletion(ctx context.Context, req OpenAIRequest) (*OpenAIResponse, error) {
	// Default model if not specified
	if req.Model == "" {
		req.Model = c.Model
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