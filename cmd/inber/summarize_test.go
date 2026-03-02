package main

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

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

// contains is declared in postwrite_test.go
