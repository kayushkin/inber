package agent

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/internal/toolid"
)

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

		// Normalize the schema into the dialect strict OpenAI-compatible validators
		// accept before forwarding it. This runs on the marshaled copy above, so the
		// canonical anthropic InputSchema is never mutated.
		normalizeSchemaForOpenAI(schemaMap)

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

// normalizeSchemaForOpenAI recursively rewrites a JSON Schema so it is accepted by
// strict OpenAI-compatible tool-schema validators (OpenAI structured outputs,
// Moonshot/Kimi, and similar). Today it rewrites every "oneOf" combinator to "anyOf",
// recursing into nested subschemas ($defs, properties, items, and the combinator
// arrays themselves).
//
// The rewrite is a provably-safe widening: any value valid under "oneOf" is also valid
// under "anyOf" (oneOf requires exactly one branch to match, anyOf requires at least
// one), so no argument that was legal before is newly rejected. Strict validators such
// as Moonshot's reject a "$ref → oneOf" as false-positive infinite recursion and 400
// the whole turn; OpenAI structured outputs likewise accepts "anyOf" and rejects
// "oneOf". A tool schema authored by a codegen that emits "oneOf" (common for
// enums-with-per-variant-docs and tagged unions) would otherwise fail the entire
// request against those backends.
//
// The node is mutated in place. Only the marshaled copy passed by the caller is
// touched; the canonical anthropic InputSchema is never modified.
func normalizeSchemaForOpenAI(node interface{}) {
	switch n := node.(type) {
	case map[string]interface{}:
		if oneOf, ok := n["oneOf"]; ok {
			delete(n, "oneOf")
			if existing, ok := n["anyOf"]; ok {
				// Pathological: both combinators at the same level. Merging both
				// branch lists into a single anyOf stays a widening of the original
				// (oneOf AND anyOf) constraint, so it still never newly rejects.
				n["anyOf"] = append(asSlice(existing), asSlice(oneOf)...)
			} else {
				n["anyOf"] = oneOf
			}
		}
		for _, child := range n {
			normalizeSchemaForOpenAI(child)
		}
	case []interface{}:
		for _, child := range n {
			normalizeSchemaForOpenAI(child)
		}
	}
}

// asSlice returns v as a []interface{} if it already is one, otherwise wraps it in a
// single-element slice so combinator merges never drop a branch.
func asSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}
	return []interface{}{v}
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
			ID:    toolid.Sanitize(tc.ID),
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
				clean := toolid.Sanitize(block.OfToolUse.ID)
				if clean != block.OfToolUse.ID {
					block.OfToolUse.ID = clean
					dirty = true
				}
			}
			if block.OfToolResult != nil {
				clean := toolid.Sanitize(block.OfToolResult.ToolUseID)
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
func FilterMessagesForAnthropic(messages []anthropic.MessageParam) ([]anthropic.MessageParam, FilterStats) {
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

	stats := FilterStats{
		ToolUseFiltered:    filteredToolUse,
		ToolResultFiltered: filteredToolResult,
	}

	return filtered, stats
}

// FilterStats tracks how many blocks were filtered
type FilterStats struct {
	ToolUseFiltered    int
	ToolResultFiltered int
}