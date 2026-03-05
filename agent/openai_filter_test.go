package agent

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestIsAnthropicToolID(t *testing.T) {
	tests := []struct {
		id       string
		expected bool
	}{
		// Anthropic IDs
		{"toolu_01234567890abcdef", true},
		{"toolu_abc123xyz789", true},
		
		// OpenAI/GLM IDs (not Anthropic)
		{"call_abc123", false},
		{"chatcmpl_abc123", false},
		{"tool_abc123", false},
		{"call_12345678901234567890", false},
		
		// Edge cases - unknown format, conservatively not Anthropic
		{"short", false},
		{"verylongidalphanumericonly12345678901234567890", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			result := isAnthropicToolID(tt.id)
			if result != tt.expected {
				t.Errorf("isAnthropicToolID(%q) = %v, want %v", tt.id, result, tt.expected)
			}
		})
	}
}

func TestIsOpenAIToolID(t *testing.T) {
	tests := []struct {
		id       string
		expected bool
	}{
		// OpenAI/GLM IDs
		{"call_abc123", true},
		{"chatcmpl_abc123", true},
		{"tool_abc123", true}, // short sanitized ID (common from GLM after sanitization)
		{"call_12345678901234567890", true},
		
		// Anthropic IDs (not OpenAI)
		{"toolu_01234567890abcdef", false},
		{"toolu_abc123xyz789", false},
		
		// Edge cases - unknown format, conservatively not OpenAI
		{"short", false},
		{"verylongidalphanumericonly12345678901234567890", false},
		{"tool_verylongidthatmightbeambiguous12345678", false}, // too long, might be Anthropic-style
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			result := isOpenAIToolID(tt.id)
			if result != tt.expected {
				t.Errorf("isOpenAIToolID(%q) = %v, want %v", tt.id, result, tt.expected)
			}
		})
	}
}

func TestFilterMessagesForAnthropic(t *testing.T) {
	// Create test messages with mixed tool IDs
	messages := []anthropic.MessageParam{
		// User message with text
		anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		// Assistant with mixed tool calls
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("Let me help"),
				{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    "toolu_anthropic123", // Anthropic ID
						Name:  "read_file",
						Input: struct{}{},
					},
				},
				{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    "call_openai456", // OpenAI ID
						Name:  "shell",
						Input: struct{}{},
					},
				},
			},
		},
		// User with mixed tool results
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: "toolu_anthropic123", // Anthropic ID
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{Text: "file contents"}},
						},
					},
				},
				{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: "call_openai456", // OpenAI ID - should be filtered
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{Text: "shell output"}},
						},
					},
				},
			},
		},
	}

	filtered := FilterMessagesForAnthropic(messages)
	stats := LastFilterStats()

	// Should have filtered 1 tool_use and 1 tool_result
	if stats.ToolUseFiltered != 1 {
		t.Errorf("expected 1 tool_use filtered, got %d", stats.ToolUseFiltered)
	}
	if stats.ToolResultFiltered != 1 {
		t.Errorf("expected 1 tool_result filtered, got %d", stats.ToolResultFiltered)
	}

	// Check assistant message has only Anthropic tool_use
	assistantMsg := filtered[1]
	var toolUseCount int
	for _, block := range assistantMsg.Content {
		if block.OfToolUse != nil {
			toolUseCount++
			if block.OfToolUse.ID != "toolu_anthropic123" {
				t.Errorf("unexpected tool_use ID: %s", block.OfToolUse.ID)
			}
		}
	}
	if toolUseCount != 1 {
		t.Errorf("expected 1 tool_use in filtered message, got %d", toolUseCount)
	}

	// Check user message has only Anthropic tool_result
	userMsg := filtered[2]
	var toolResultCount int
	for _, block := range userMsg.Content {
		if block.OfToolResult != nil {
			toolResultCount++
			if block.OfToolResult.ToolUseID != "toolu_anthropic123" {
				t.Errorf("unexpected tool_result ID: %s", block.OfToolResult.ToolUseID)
			}
		}
	}
	if toolResultCount != 1 {
		t.Errorf("expected 1 tool_result in filtered message, got %d", toolResultCount)
	}
}

func TestConvertAnthropicMessagesToOpenAIFilters(t *testing.T) {
	// Create test messages with mixed tool IDs
	messages := []anthropic.MessageParam{
		// User message with text
		anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		// Assistant with mixed tool calls
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("Let me help"),
				{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    "toolu_anthropic123", // Anthropic ID - should be filtered
						Name:  "read_file",
						Input: struct{}{},
					},
				},
				{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    "call_openai456", // OpenAI ID - kept
						Name:  "shell",
						Input: struct{}{},
					},
				},
			},
		},
		// User with mixed tool results
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: "toolu_anthropic123", // Anthropic ID - should be filtered
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{Text: "file contents"}},
						},
					},
				},
				{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: "call_openai456", // OpenAI ID - kept
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{Text: "shell output"}},
						},
					},
				},
			},
		},
	}

	oaiMessages := ConvertAnthropicMessagesToOpenAI(messages)

	// Find assistant message
	var assistantMsg *OpenAIMessage
	for i := range oaiMessages {
		if oaiMessages[i].Role == "assistant" {
			assistantMsg = &oaiMessages[i]
			break
		}
	}

	if assistantMsg == nil {
		t.Fatal("no assistant message found")
	}

	// Should have only 1 tool call (the OpenAI one)
	if len(assistantMsg.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(assistantMsg.ToolCalls))
	} else if assistantMsg.ToolCalls[0].ID != "call_openai456" {
		t.Errorf("unexpected tool call ID: %s", assistantMsg.ToolCalls[0].ID)
	}

	// Find tool result message
	var toolResultMsg *OpenAIMessage
	for i := range oaiMessages {
		if oaiMessages[i].Role == "tool" {
			toolResultMsg = &oaiMessages[i]
			break
		}
	}

	if toolResultMsg == nil {
		t.Fatal("no tool result message found")
	}

	// Should have the OpenAI tool result ID
	if toolResultMsg.ToolCallID != "call_openai456" {
		t.Errorf("unexpected tool result ID: %s", toolResultMsg.ToolCallID)
	}
}
