package context

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestPruneMessages_Oldest_UnderBudget(t *testing.T) {
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")),
	}
	result := PruneMessages(&msgs, 100000, "oldest")
	if result.Removed != 0 {
		t.Errorf("expected 0 removed, got %d", result.Removed)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
}

func TestPruneMessages_Oldest_OverBudget(t *testing.T) {
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("first message that is fairly long to use tokens")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("first response that is also fairly long")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("second message")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("second response")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("third message")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("third response")),
	}

	// Set a very low budget to force pruning
	result := PruneMessages(&msgs, 30, "oldest")
	if result.Removed == 0 {
		t.Error("expected some messages to be removed")
	}
	if result.TokensFreed == 0 {
		t.Error("expected tokens freed > 0")
	}
	if result.Strategy != "oldest" {
		t.Errorf("expected strategy 'oldest', got %q", result.Strategy)
	}
	// Should keep at least 4 messages (last 2 turns)
	if len(msgs) > 6 {
		t.Error("messages should not have grown")
	}
}

func TestPruneMessages_Summary(t *testing.T) {
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("a long message with lots of content here")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("a long response with lots of content")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("another message")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("another response")),
	}

	result := PruneMessages(&msgs, 10, "summary")
	if result.Strategy != "summary" {
		t.Errorf("expected strategy 'summary', got %q", result.Strategy)
	}
}

func TestPruneMessages_NilMessages(t *testing.T) {
	result := PruneMessages(nil, 100, "oldest")
	if result.Removed != 0 {
		t.Errorf("expected 0 removed for nil, got %d", result.Removed)
	}
}

func TestPruneMessages_EmptyMessages(t *testing.T) {
	msgs := []anthropic.MessageParam{}
	result := PruneMessages(&msgs, 100, "oldest")
	if result.Removed != 0 {
		t.Errorf("expected 0 removed for empty, got %d", result.Removed)
	}
}

func TestEstimateMessageTokens(t *testing.T) {
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello world")),
	}
	tokens := estimateMessageTokens(msgs)
	if tokens <= 0 {
		t.Errorf("expected positive token count, got %d", tokens)
	}
}
