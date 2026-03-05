package conversation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

func TestShouldSummarize(t *testing.T) {
	cfg := DefaultSummarizeConfig(RoleDefault)

	// Below threshold
	msgs := makeMessages(cfg.TriggerMessages - 1)
	if ShouldSummarize(msgs, cfg) {
		t.Error("should not summarize below threshold")
	}

	// At threshold
	msgs = makeMessages(cfg.TriggerMessages)
	if ShouldSummarize(msgs, cfg) {
		t.Error("should not summarize at exact threshold")
	}

	// Above threshold
	msgs = makeMessages(cfg.TriggerMessages + 1)
	if !ShouldSummarize(msgs, cfg) {
		t.Error("should summarize above threshold")
	}
}

func TestFindTurnBoundary(t *testing.T) {
	// 10 user messages = 10 turns, 20 messages total (alternating)
	msgs := makeMessages(20)

	idx := findTurnBoundary(msgs, 5)
	// Should keep last 5 turns, so split at message 10 (index 10)
	if idx < 1 {
		t.Errorf("expected non-zero split index, got %d", idx)
	}

	// Count remaining user messages after split
	remaining := 0
	for _, msg := range msgs[idx:] {
		if msg.Role == anthropic.MessageParamRoleUser {
			remaining++
		}
	}
	if remaining < 5 {
		t.Errorf("expected at least 5 remaining turns, got %d", remaining)
	}
}

func TestCountTurns(t *testing.T) {
	msgs := makeMessages(20) // 10 user + 10 assistant
	turns := countTurns(msgs)
	if turns != 10 {
		t.Errorf("expected 10 turns, got %d", turns)
	}
}

func TestMessagesToText(t *testing.T) {
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi there")),
	}
	text := messagesToText(msgs)
	if text == "" {
		t.Error("expected non-empty text")
	}
	if !contains(text, "hello") || !contains(text, "hi there") {
		t.Error("expected text to contain message contents")
	}
}

func TestMechanicalSummary(t *testing.T) {
	msgs := makeMessages(20)
	summary := mechanicalSummary(msgs)
	if summary == "" {
		t.Error("expected non-empty mechanical summary")
	}
	if !contains(summary, "10 conversation turns") {
		t.Errorf("expected turn count in summary, got: %s", summary)
	}
}

func TestFixAlternation(t *testing.T) {
	// Two user messages in a row
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("a")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("b")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("c")),
	}
	fixed := fixAlternation(msgs)
	// Should merge the two user messages
	if len(fixed) != 2 {
		t.Errorf("expected 2 messages after fix, got %d", len(fixed))
	}

	// Two assistant messages in a row
	msgs = []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("a")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("b")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("c")),
	}
	fixed = fixAlternation(msgs)
	// Should insert a placeholder user message
	if len(fixed) != 4 {
		t.Errorf("expected 4 messages after fix, got %d", len(fixed))
	}
	if fixed[2].Role != anthropic.MessageParamRoleUser {
		t.Error("expected inserted placeholder to be user role")
	}
}

func TestDefaultSummarizeConfigs(t *testing.T) {
	orch := DefaultSummarizeConfig(RoleOrchestrator)
	coder := DefaultSummarizeConfig(RoleCoder)
	def := DefaultSummarizeConfig(RoleDefault)

	// Orchestrator should have higher trigger (longer conversations)
	if orch.TriggerMessages <= coder.TriggerMessages {
		t.Error("orchestrator should have higher trigger than coder")
	}

	// All should save to memory
	if !orch.SaveToMemory || !coder.SaveToMemory || !def.SaveToMemory {
		t.Error("all configs should save to memory by default")
	}
}

// makeMessages creates N alternating user/assistant messages
func makeMessages(n int) []anthropic.MessageParam {
	msgs := make([]anthropic.MessageParam, n)
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			msgs[i] = anthropic.NewUserMessage(anthropic.NewTextBlock("user message " + string(rune('a'+i%26))))
		} else {
			msgs[i] = anthropic.NewAssistantMessage(anthropic.NewTextBlock("assistant response " + string(rune('a'+i%26))))
		}
	}
	return msgs
}

func TestStripOrphanedToolResults(t *testing.T) {
	// Simulate: assistant used tool in message 1, user has tool_result in message 2
	msgs := []anthropic.MessageParam{
		// No tool_use in these messages (it was in the summarized part)
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "orphaned-tool-id",
				}},
			},
		},
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("ok got it")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("next question")),
	}

	stripped := stripOrphanedToolResults(msgs)
	// The orphaned tool_result message should be removed (no content left)
	// Remaining: assistant + user = 2 messages
	if len(stripped) != 2 {
		t.Errorf("expected 2 messages after stripping, got %d", len(stripped))
	}
}

func TestFindTurnBoundaryWithToolUse(t *testing.T) {
	// Build messages where tool_use and tool_result span the potential split point
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("old message 1")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("old response 1")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("old message 2")),
		// Assistant uses a tool
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    "tool-123",
					Name:  "read_file",
					Input: json.RawMessage(`{}`),
				}},
			},
		},
		// User has tool result
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool-123",
				}},
			},
		},
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("got the file")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("recent 1")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("recent response 1")),
	}

	// Keep 2 turns — naive split would be at index 4 (tool_result),
	// orphaning it from tool_use at index 3
	idx := findTurnBoundary(msgs, 2)

	// The split should not orphan tool results
	keptMessages := msgs[idx:]
	toolUseIDs := collectToolUseIDs(keptMessages)
	orphaned := findOrphanedToolResults(keptMessages, collectToolUseIDs(msgs[:idx]))
	if len(orphaned) > 0 {
		t.Errorf("found %d orphaned tool results at split index %d, tool_use IDs in kept: %v",
			len(orphaned), idx, toolUseIDs)
	}
}

// contains is declared in postwrite_test.go
