package main

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestRepairDanglingToolUse_LastMessage(t *testing.T) {
	// Assistant message with tool_use is the last message (interrupted)
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("do something")),
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    "tool-abc",
					Name:  "write_file",
					Input: json.RawMessage(`{}`),
				}},
			},
		},
	}

	repaired := repairDanglingToolUse(msgs)

	// Should have 3 messages: user, assistant, synthetic user with tool_result
	if len(repaired) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(repaired))
	}

	// Last message should be user with tool_result
	last := repaired[2]
	if last.Role != anthropic.MessageParamRoleUser {
		t.Errorf("expected user role, got %s", last.Role)
	}
	if len(last.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(last.Content))
	}
	if last.Content[0].OfToolResult == nil {
		t.Fatal("expected tool_result block")
	}
	if last.Content[0].OfToolResult.ToolUseID != "tool-abc" {
		t.Errorf("expected tool_use_id tool-abc, got %s", last.Content[0].OfToolResult.ToolUseID)
	}
}

func TestRepairDanglingToolUse_MissingInNextMessage(t *testing.T) {
	// Assistant has 2 tool_use, but next user message only has 1 tool_result
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("do two things")),
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    "tool-1",
					Name:  "read_file",
					Input: json.RawMessage(`{}`),
				}},
				{OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    "tool-2",
					Name:  "write_file",
					Input: json.RawMessage(`{}`),
				}},
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool-1",
				}},
				// tool-2 result is missing (interrupted)
			},
		},
	}

	repaired := repairDanglingToolUse(msgs)

	// Should still be 3 messages, but last user message should now have 2 tool_results
	if len(repaired) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(repaired))
	}

	lastUser := repaired[2]
	resultCount := 0
	for _, block := range lastUser.Content {
		if block.OfToolResult != nil {
			resultCount++
		}
	}
	if resultCount != 2 {
		t.Errorf("expected 2 tool_results, got %d", resultCount)
	}
}

func TestRepairDanglingToolUse_NoRepairNeeded(t *testing.T) {
	// Normal conversation, no dangling tool_use
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("bye")),
	}

	repaired := repairDanglingToolUse(msgs)
	if len(repaired) != 3 {
		t.Errorf("expected 3 messages, got %d", len(repaired))
	}
}
