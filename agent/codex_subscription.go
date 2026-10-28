package agent

// This file sends OpenAI-provider requests through the ChatGPT subscription
// that the Codex CLI signs in with, instead of a pay-per-token API key.
//
// The endpoint is the one the Codex CLI itself posts to. It is not a
// documented API: it can change, or start refusing callers that are not the
// Codex CLI, without notice. The credential is the CLI's own, read from its
// auth file. So a working call is a fact about today, and the API key path
// stays one setting away.
//
// The endpoint speaks the Responses API, not chat completions, and only
// streams. The turn loop builds chat-completion requests and reads
// chat-completion replies, so this file converts in both directions and
// leaves the turn loop as it is. Nothing is dropped in either direction except
// the model's reasoning items, which a chat-completion reply has no field for;
// the next call reasons afresh, as it does over chat completions.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// CodexSubscriptionResponsesEndpoint is where the Codex CLI posts when it is
// signed in with a ChatGPT subscription.
const CodexSubscriptionResponsesEndpoint = "https://chatgpt.com/backend-api/codex/responses"

// ErrCodexSubscriptionTokenExpired means the token in the Codex CLI's auth
// file is past its expiry. The CLI owns refresh: any codex command rewrites the
// file with a fresh token.
var ErrCodexSubscriptionTokenExpired = errors.New("codex subscription access token expired")

// readCodexSubscriptionAccessToken reads the access token from the Codex CLI's
// auth file. It reads the file on every call rather than once, because the CLI
// refreshes the token on its own schedule and rewrites the file; a copy held
// in memory goes stale and starts failing.
func readCodexSubscriptionAccessToken(authFilePath string, now time.Time) (string, error) {
	raw, err := os.ReadFile(authFilePath)
	if err != nil {
		return "", fmt.Errorf("read codex auth file: %w", err)
	}
	var file struct {
		AuthMode string `json:"auth_mode"`
		Tokens   struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return "", fmt.Errorf("decode codex auth file %s: %w", authFilePath, err)
	}
	if file.AuthMode != "chatgpt" {
		return "", fmt.Errorf("codex auth file %s is in auth_mode %q, not a ChatGPT subscription", authFilePath, file.AuthMode)
	}
	if file.Tokens.AccessToken == "" {
		return "", fmt.Errorf("codex auth file %s has no access_token", authFilePath)
	}

	segments := strings.Split(file.Tokens.AccessToken, ".")
	if len(segments) != 3 {
		return "", fmt.Errorf("codex access token is not a JWT: %d segments, want 3", len(segments))
	}
	payload, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return "", fmt.Errorf("decode codex access token claims: %w", err)
	}
	var claims struct {
		Expiry int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("decode codex access token claims: %w", err)
	}
	if claims.Expiry == 0 {
		return "", errors.New("codex access token has no exp claim")
	}
	if expiresAt := time.Unix(claims.Expiry, 0); !now.Before(expiresAt) {
		return "", fmt.Errorf("%w at %s; run any codex command to refresh it", ErrCodexSubscriptionTokenExpired, expiresAt.UTC().Format(time.RFC3339))
	}
	return file.Tokens.AccessToken, nil
}

// chatCompletionThroughCodexSubscription sends one chat-completion request as
// a Responses request on the subscription endpoint and returns the reply as a
// chat completion.
func (c *OpenAIClient) chatCompletionThroughCodexSubscription(ctx context.Context, request OpenAIRequest) (*OpenAIResponse, error) {
	token, err := readCodexSubscriptionAccessToken(c.CodexSubscriptionAuthFilePath, time.Now())
	if err != nil {
		return nil, err
	}
	registerProviderCredentialWithEgressRedactor("openai-codex-subscription", token)

	body, err := responsesRequestFromChatCompletion(request)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode responses request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "text/event-stream")
	httpRequest.Header.Set("Authorization", "Bearer "+token)

	httpResponse, err := c.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		failure, _ := io.ReadAll(io.LimitReader(httpResponse.Body, 8192))
		return nil, fmt.Errorf("API error %d: %s", httpResponse.StatusCode, strings.TrimSpace(string(failure)))
	}
	return chatCompletionFromResponsesStream(httpResponse.Body)
}

// responsesRequestFromChatCompletion converts the request. System messages
// become the instructions; every other message becomes input items in order.
func responsesRequestFromChatCompletion(request OpenAIRequest) (map[string]any, error) {
	var instructions []string
	input := []any{}

	for index, message := range request.Messages {
		text, err := openAIMessageText(message)
		if err != nil {
			return nil, fmt.Errorf("message %d (%s): %w", index, message.Role, err)
		}
		switch message.Role {
		case "system":
			instructions = append(instructions, text)
		case "user":
			input = append(input, map[string]any{
				"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_text", "text": text}},
			})
		case "assistant":
			if text != "" {
				input = append(input, map[string]any{
					"type": "message", "role": "assistant",
					"content": []any{map[string]any{"type": "output_text", "text": text}},
				})
			}
			for _, call := range message.ToolCalls {
				input = append(input, map[string]any{
					"type": "function_call", "call_id": call.ID,
					"name": call.Function.Name, "arguments": call.Function.Arguments,
				})
			}
		case "tool":
			input = append(input, map[string]any{
				"type": "function_call_output", "call_id": message.ToolCallID, "output": text,
			})
		default:
			return nil, fmt.Errorf("message %d has role %q, which the Responses API has no item for", index, message.Role)
		}
	}

	body := map[string]any{
		"model":        request.Model,
		"instructions": strings.Join(instructions, "\n\n"),
		"input":        input,
		"stream":       true,
		"store":        false,
	}
	if len(request.Tools) > 0 {
		tools := make([]any, 0, len(request.Tools))
		for _, tool := range request.Tools {
			tools = append(tools, map[string]any{
				"type":        "function",
				"name":        tool.Function.Name,
				"description": tool.Function.Description,
				"parameters":  tool.Function.Parameters,
			})
		}
		body["tools"] = tools
	}
	if limit := request.MaxCompletionTokens; limit > 0 {
		body["max_output_tokens"] = limit
	} else if request.MaxTokens > 0 {
		body["max_output_tokens"] = request.MaxTokens
	}
	if request.PromptCacheKey != "" {
		body["prompt_cache_key"] = request.PromptCacheKey
	}
	if request.Temperature != 0 {
		body["temperature"] = request.Temperature
	}
	return body, nil
}

// openAIMessageText returns a message's content as text. The converters in
// this package only ever produce a string or nothing; anything else is an
// error rather than content silently left out.
func openAIMessageText(message OpenAIMessage) (string, error) {
	switch content := message.Content.(type) {
	case nil:
		return "", nil
	case string:
		return content, nil
	default:
		return "", fmt.Errorf("content of type %T cannot be sent to the Responses API", message.Content)
	}
}

// responsesStreamEvent is the part of each server-sent event this file reads.
type responsesStreamEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Item    struct {
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		Content   []struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Refusal string `json:"refusal"`
		} `json:"content"`
	} `json:"item"`
	Response struct {
		ID                string `json:"id"`
		Model             string `json:"model"`
		Status            string `json:"status"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Usage *struct {
			InputTokens        int `json:"input_tokens"`
			InputTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	} `json:"response"`
}

// chatCompletionFromResponsesStream reads the event stream to its end and
// builds the chat-completion reply from the finished output items. The
// endpoint's completed event carries no output of its own, so the items have
// to be collected as each one finishes.
func chatCompletionFromResponsesStream(stream io.Reader) (*OpenAIResponse, error) {
	var text strings.Builder
	var toolCalls []OpenAIToolCall
	var finished *responsesStreamEvent

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		payload, found := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "data:")
		if !found {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil, fmt.Errorf("decode responses stream event: %w", err)
		}

		switch event.Type {
		case "response.output_item.done":
			switch event.Item.Type {
			case "message":
				for _, part := range event.Item.Content {
					switch part.Type {
					case "output_text":
						text.WriteString(part.Text)
					case "refusal":
						text.WriteString(part.Refusal)
					}
				}
			case "function_call":
				toolCalls = append(toolCalls, OpenAIToolCall{
					ID:   event.Item.CallID,
					Type: "function",
					Function: OpenAIFunctionCall{
						Name:      event.Item.Name,
						Arguments: event.Item.Arguments,
					},
				})
			}
		case "response.completed", "response.incomplete":
			finished = &event
		case "response.failed":
			if event.Response.Error != nil {
				return nil, fmt.Errorf("response failed: %s: %s", event.Response.Error.Code, event.Response.Error.Message)
			}
			return nil, errors.New("response failed with no error given")
		case "error":
			return nil, fmt.Errorf("responses stream error: %s", payload)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read responses stream: %w", err)
	}
	if finished == nil {
		return nil, errors.New("responses stream ended before the response completed")
	}

	finishReason := "stop"
	switch {
	case finished.Type == "response.incomplete":
		finishReason = "length"
		if details := finished.Response.IncompleteDetails; details != nil && details.Reason == "content_filter" {
			finishReason = "content_filter"
		}
	case len(toolCalls) > 0:
		finishReason = "tool_calls"
	}

	reply := &OpenAIResponse{
		ID:     finished.Response.ID,
		Object: "chat.completion",
		Model:  finished.Response.Model,
		Choices: []OpenAIChoice{{
			Message: OpenAIMessage{
				Role:      "assistant",
				Content:   text.String(),
				ToolCalls: toolCalls,
			},
			FinishReason: finishReason,
		}},
	}
	if usage := finished.Response.Usage; usage != nil {
		reply.Usage = OpenAIUsage{
			PromptTokens:     usage.InputTokens,
			CompletionTokens: usage.OutputTokens,
			TotalTokens:      usage.TotalTokens,
			PromptTokensDetails: OpenAIPromptTokensDetails{
				CachedTokens: usage.InputTokensDetails.CachedTokens,
			},
		}
	}
	return reply, nil
}
