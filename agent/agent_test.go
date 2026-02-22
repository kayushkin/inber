package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/kayushkin/inber/agent"
)

func skipIfNoKey(t *testing.T) *anthropic.Client {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	c := anthropic.NewClient(option.WithAPIKey(key))
	return &c
}

func TestSimpleResponse(t *testing.T) {
	client := skipIfNoKey(t)
	a := agent.New(client, "claude-sonnet-4-5-20250929", "You are a helpful assistant. Be very brief.")

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("What is 2+2? Reply with just the number.")),
	}

	result, err := a.Run(context.Background(), &messages)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.Text == "" {
		t.Fatal("expected non-empty text response")
	}
	if result.ToolCalls != 0 {
		t.Errorf("expected 0 tool calls, got %d", result.ToolCalls)
	}
	if result.InputTokens == 0 {
		t.Error("expected non-zero input tokens")
	}
	if result.OutputTokens == 0 {
		t.Error("expected non-zero output tokens")
	}
	t.Logf("Response: %s", result.Text)
	t.Logf("Tokens: in=%d out=%d", result.InputTokens, result.OutputTokens)
}

func TestToolCall(t *testing.T) {
	client := skipIfNoKey(t)
	a := agent.New(client, "claude-sonnet-4-5-20250929", "You are a helpful assistant. Use the add tool to add numbers. Be very brief.")

	a.AddTool(agent.Tool{
		Name:        "add",
		Description: "Add two numbers together",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"a": map[string]interface{}{"type": "number", "description": "first number"},
				"b": map[string]interface{}{"type": "number", "description": "second number"},
			},
			Required: []string{"a", "b"},
		},
		Run: func(ctx context.Context, input string) (string, error) {
			var args struct {
				A float64 `json:"a"`
				B float64 `json:"b"`
			}
			if err := json.Unmarshal([]byte(input), &args); err != nil {
				return "", err
			}
			return jsonMarshal(args.A + args.B)
		},
	})

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("What is 37 + 58? Use the add tool.")),
	}

	result, err := a.Run(context.Background(), &messages)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.ToolCalls == 0 {
		t.Error("expected at least one tool call")
	}
	if result.Text == "" {
		t.Fatal("expected non-empty text response")
	}
	t.Logf("Response: %s", result.Text)
	t.Logf("Tool calls: %d, Tokens: in=%d out=%d", result.ToolCalls, result.InputTokens, result.OutputTokens)
}

func TestMultipleToolCalls(t *testing.T) {
	client := skipIfNoKey(t)
	a := agent.New(client, "claude-sonnet-4-5-20250929", "You are a calculator. Use tools for all math. Be very brief.")

	a.AddTool(agent.Tool{
		Name:        "multiply",
		Description: "Multiply two numbers",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"a": map[string]interface{}{"type": "number", "description": "first number"},
				"b": map[string]interface{}{"type": "number", "description": "second number"},
			},
			Required: []string{"a", "b"},
		},
		Run: func(ctx context.Context, input string) (string, error) {
			var args struct {
				A float64 `json:"a"`
				B float64 `json:"b"`
			}
			if err := json.Unmarshal([]byte(input), &args); err != nil {
				return "", err
			}
			return jsonMarshal(args.A * args.B)
		},
	})

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("What is 6 * 7? Then what is 8 * 9? Use the multiply tool for each.")),
	}

	result, err := a.Run(context.Background(), &messages)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.ToolCalls < 2 {
		t.Errorf("expected at least 2 tool calls, got %d", result.ToolCalls)
	}
	t.Logf("Response: %s", result.Text)
	t.Logf("Tool calls: %d, Tokens: in=%d out=%d", result.ToolCalls, result.InputTokens, result.OutputTokens)
}

func TestConversationContinuity(t *testing.T) {
	client := skipIfNoKey(t)
	a := agent.New(client, "claude-sonnet-4-5-20250929", "You are a helpful assistant. Be very brief.")

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("My name is Inber. Just say hi.")),
	}

	result1, err := a.Run(context.Background(), &messages)
	if err != nil {
		t.Fatalf("Run 1 failed: %v", err)
	}
	t.Logf("Turn 1: %s", result1.Text)

	// Continue the conversation
	messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock("What's my name?")))

	result2, err := a.Run(context.Background(), &messages)
	if err != nil {
		t.Fatalf("Run 2 failed: %v", err)
	}
	t.Logf("Turn 2: %s", result2.Text)

	if result2.Text == "" {
		t.Fatal("expected non-empty response")
	}
}

// json.Marshal helper that returns string
func jsonMarshal(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
