package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// isAnthropicToolID returns true if the tool ID appears to be from Anthropic.
// Anthropic IDs typically start with "toolu_".
func isAnthropicToolID(id string) bool {
	// Anthropic tool_use IDs typically start with "toolu_"
	return strings.HasPrefix(id, "toolu_")
}

// ConvertAnthropicMessagesToOpenAI converts Anthropic message format to OpenAI format.
// It filters out tool_use/tool_result pairs that originated from Anthropic (non-OpenAI)
// to avoid ID confusion when sending to OpenAI-compatible APIs.
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
					// Skip Anthropic-originated tool calls - only include OpenAI-compatible ones
					if isAnthropicToolID(block.OfToolUse.ID) {
						continue
					}
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
					// Skip Anthropic-originated tool results - only include OpenAI-compatible ones
					if isAnthropicToolID(block.OfToolResult.ToolUseID) {
						continue
					}
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
			ID:    sanitizeToolID(tc.ID),
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

// sanitizeToolID ensures a tool ID matches Anthropic's pattern ^[a-zA-Z0-9_-]+$
// OpenAI/GLM may generate IDs with dots, colons, or other characters.
func sanitizeToolID(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	result := b.String()
	if result == "" {
		return "tool_" + fmt.Sprintf("%d", len(id))
	}
	return result
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

// SanitizeMessageToolIDs cleans up tool_use and tool_result IDs in a message
// history to match Anthropic's pattern ^[a-zA-Z0-9_-]+$.
// This is needed when resuming sessions that contain responses from
// OpenAI-compatible providers (GLM, etc.) which may use different ID formats.
func SanitizeMessageToolIDs(messages []anthropic.MessageParam) []anthropic.MessageParam {
	dirty := false
	for i := range messages {
		for j := range messages[i].Content {
			block := &messages[i].Content[j]
			if block.OfToolUse != nil {
				clean := sanitizeToolID(block.OfToolUse.ID)
				if clean != block.OfToolUse.ID {
					block.OfToolUse.ID = clean
					dirty = true
				}
			}
			if block.OfToolResult != nil {
				clean := sanitizeToolID(block.OfToolResult.ToolUseID)
				if clean != block.OfToolResult.ToolUseID {
					block.OfToolResult.ToolUseID = clean
					dirty = true
				}
			}
		}
	}
	_ = dirty
	return messages
}

// isOpenAIToolID returns true if the tool ID appears to be from OpenAI-compatible APIs.
// OpenAI/GLM IDs often have prefixes like "call_", "chatcmpl_", or "tool_" (after sanitization).
func isOpenAIToolID(id string) bool {
	// Common OpenAI tool call ID prefixes
	if strings.HasPrefix(id, "call_") || strings.HasPrefix(id, "chatcmpl_") {
		return true
	}
	// Sanitized IDs that start with "tool_" (from e.g., "call.abc" → "tool_abc")
	// These are typically short (<20 chars) and come from OpenAI-compatible APIs
	if strings.HasPrefix(id, "tool_") && len(id) < 30 {
		return true
	}
	return false
}

// FilterMessagesForAnthropic removes tool_use/tool_result pairs that originated
// from OpenAI-compatible APIs. This prevents ID confusion when sending messages
// to Anthropic after a provider switch.
func FilterMessagesForAnthropic(messages []anthropic.MessageParam) []anthropic.MessageParam {
	filtered := make([]anthropic.MessageParam, 0, len(messages))
	filteredToolUse := 0
	filteredToolResult := 0

	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleAssistant {
			// Filter out OpenAI-sourced tool_use blocks
			var newBlocks []anthropic.ContentBlockParamUnion
			for _, block := range msg.Content {
				if block.OfToolUse != nil && isOpenAIToolID(block.OfToolUse.ID) {
					filteredToolUse++
					continue // Skip OpenAI-sourced tool calls
				}
				newBlocks = append(newBlocks, block)
			}
			if len(newBlocks) != len(msg.Content) {
				filtered = append(filtered, anthropic.MessageParam{
					Role:    msg.Role,
					Content: newBlocks,
				})
			} else {
				filtered = append(filtered, msg)
			}
		} else if msg.Role == anthropic.MessageParamRoleUser {
			// Filter out OpenAI-sourced tool_result blocks
			var newBlocks []anthropic.ContentBlockParamUnion
			for _, block := range msg.Content {
				if block.OfToolResult != nil && isOpenAIToolID(block.OfToolResult.ToolUseID) {
					filteredToolResult++
					continue // Skip OpenAI-sourced tool results
				}
				newBlocks = append(newBlocks, block)
			}
			if len(newBlocks) != len(msg.Content) {
				// Only add if there's content left
				if len(newBlocks) > 0 {
					filtered = append(filtered, anthropic.MessageParam{
						Role:    msg.Role,
						Content: newBlocks,
					})
				}
			} else {
				filtered = append(filtered, msg)
			}
		} else {
			filtered = append(filtered, msg)
		}
	}

	// Log if any filtering occurred (caller can check)
	if filteredToolUse > 0 || filteredToolResult > 0 {
		// Return stats via a package-level variable for logging
		lastFilterStats = FilterStats{
			ToolUseFiltered:   filteredToolUse,
			ToolResultFiltered: filteredToolResult,
		}
	}

	return filtered
}

// FilterStats tracks how many blocks were filtered
type FilterStats struct {
	ToolUseFiltered    int
	ToolResultFiltered int
}

// lastFilterStats stores the most recent filter operation stats
var lastFilterStats FilterStats

// LastFilterStats returns stats from the most recent filter operation
func LastFilterStats() FilterStats {
	return lastFilterStats
}
