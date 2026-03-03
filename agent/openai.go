package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
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

// OpenAI API types (exported for use in engine)

type OpenAIMessage struct {
	Role       string                `json:"role"`
	Content    interface{}           `json:"content,omitempty"` // string or []contentPart
	ToolCallID string                `json:"tool_call_id,omitempty"`
	ToolCalls  []OpenAIToolCall      `json:"tool_calls,omitempty"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type OpenAIToolCall struct {
	ID       string                `json:"id"`
	Type     string                `json:"type"` // always "function"
	Function OpenAIFunctionCall    `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

type OpenAITool struct {
	Type     string               `json:"type"` // always "function"
	Function OpenAIFunctionSchema `json:"function"`
}

type OpenAIFunctionSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

type OpenAIResponse struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []OpenAIChoice  `json:"choices"`
	Usage   OpenAIUsage     `json:"usage"`
}

type OpenAIChoice struct {
	Index        int            `json:"index"`
	Message      OpenAIMessage  `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletion sends a chat completion request to the OpenAI-compatible API.
func (c *OpenAIClient) ChatCompletion(ctx context.Context, req OpenAIRequest) (*OpenAIResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var apiResp OpenAIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &apiResp, nil
}

// ConvertAnthropicToolsToOpenAI converts Anthropic tool definitions to OpenAI function format.
func ConvertAnthropicToolsToOpenAI(tools []Tool) []OpenAITool {
	result := make([]OpenAITool, len(tools))
	for i, t := range tools {
		// Convert anthropic.ToolInputSchemaParam to a plain map
		schemaMap := make(map[string]interface{})
		
		// The InputSchema is already a map-like structure, we need to marshal/unmarshal it
		// to get a clean map[string]interface{}
		schemaBytes, _ := json.Marshal(t.InputSchema)
		json.Unmarshal(schemaBytes, &schemaMap)
		
		result[i] = OpenAITool{
			Type: "function",
			Function: OpenAIFunctionSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  schemaMap,
			},
		}
	}
	return result
}

// ConvertAnthropicMessagesToOpenAI converts Anthropic message format to OpenAI format.
func ConvertAnthropicMessagesToOpenAI(messages []anthropic.MessageParam) []OpenAIMessage {
	result := make([]OpenAIMessage, 0, len(messages))
	
	for _, msg := range messages {
		role := string(msg.Role)
		if role == "assistant" {
			// Check if this message has tool calls
			var toolCalls []OpenAIToolCall
			var textContent string
			
			for _, block := range msg.Content {
				if block.OfText != nil {
					textContent += block.OfText.Text
				} else if block.OfToolUse != nil {
					// Convert Input (json.RawMessage) to string
					inputBytes, _ := json.Marshal(block.OfToolUse.Input)
					toolCalls = append(toolCalls, OpenAIToolCall{
						ID:   block.OfToolUse.ID,
						Type: "function",
						Function: OpenAIFunctionCall{
							Name:      block.OfToolUse.Name,
							Arguments: string(inputBytes),
						},
					})
				}
			}
			
			oaiMsg := OpenAIMessage{
				Role: "assistant",
			}
			if textContent != "" {
				oaiMsg.Content = textContent
			}
			if len(toolCalls) > 0 {
				oaiMsg.ToolCalls = toolCalls
			}
			result = append(result, oaiMsg)
			
		} else if role == "user" {
			// User messages can have text or tool results
			var textParts []string
			var toolResults []OpenAIMessage
			
			for _, block := range msg.Content {
				if block.OfText != nil {
					textParts = append(textParts, block.OfText.Text)
				} else if block.OfToolResult != nil {
					// OpenAI expects tool results as separate messages with role "tool"
					content := ""
					if block.OfToolResult.Content != nil {
						for _, c := range block.OfToolResult.Content {
							if c.OfText != nil {
								content += c.OfText.Text
							}
						}
					}
					
					toolResults = append(toolResults, OpenAIMessage{
						Role:       "tool",
						Content:    content,
						ToolCallID: block.OfToolResult.ToolUseID,
					})
				}
			}
			
			// Add user message if there's text content
			if len(textParts) > 0 {
				result = append(result, OpenAIMessage{
					Role:    "user",
					Content: joinStrings(textParts, "\n"),
				})
			}
			
			// Add tool result messages
			result = append(result, toolResults...)
		}
	}
	
	return result
}

// ConvertOpenAIResponseToAnthropic converts an OpenAI response to Anthropic format.
func ConvertOpenAIResponseToAnthropic(resp *OpenAIResponse) *anthropic.Message {
	if len(resp.Choices) == 0 {
		return &anthropic.Message{
			ID:    resp.ID,
			Model: anthropic.Model(resp.Model),
			Role:  "assistant",
			Usage: anthropic.Usage{
				InputTokens:  int64(resp.Usage.PromptTokens),
				OutputTokens: int64(resp.Usage.CompletionTokens),
			},
			StopReason: anthropic.StopReasonEndTurn,
		}
	}
	
	choice := resp.Choices[0]
	msg := choice.Message
	
	// Build content blocks
	var content []anthropic.ContentBlockUnion
	
	// Add text content
	if msgText, ok := msg.Content.(string); ok && msgText != "" {
		content = append(content, anthropic.ContentBlockUnion{
			Type: "text",
			Text: msgText,
		})
	}
	
	// Add tool calls
	for _, tc := range msg.ToolCalls {
		content = append(content, anthropic.ContentBlockUnion{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(tc.Function.Arguments),
		})
	}
	
	// Map finish reason to Anthropic stop reason
	stopReason := mapOpenAIFinishReason(choice.FinishReason)
	
	return &anthropic.Message{
		ID:      resp.ID,
		Model:   anthropic.Model(resp.Model),
		Role:    "assistant",
		Content: content,
		Usage: anthropic.Usage{
			InputTokens:  int64(resp.Usage.PromptTokens),
			OutputTokens: int64(resp.Usage.CompletionTokens),
		},
		StopReason: stopReason,
	}
}

func mapOpenAIFinishReason(reason string) anthropic.StopReason {
	switch reason {
	case "stop":
		return anthropic.StopReasonEndTurn
	case "length":
		return anthropic.StopReasonMaxTokens
	case "tool_calls":
		return anthropic.StopReasonToolUse
	default:
		return anthropic.StopReasonEndTurn
	}
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}
