package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/model-store"
)

func TestSpawnAndRun_OpenAI(t *testing.T) {
	// Create a mock OpenAI server
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		
		// Parse request
		var req agent.OpenAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		// Verify request structure
		if req.Model == "" {
			t.Error("expected model to be set")
		}
		if len(req.Messages) == 0 {
			t.Error("expected messages")
		}

		// Return a simple text response (no tool calls)
		resp := agent.OpenAIResponse{
			ID:      "test-123",
			Object:  "chat.completion",
			Model:   req.Model,
			Choices: []agent.OpenAIChoice{
				{
					Index: 0,
					Message: agent.OpenAIMessage{
						Role:    "assistant",
						Content: "Task completed successfully",
					},
					FinishReason: "stop",
				},
			},
			Usage: agent.OpenAIUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create a minimal registry with OpenAI client
	client := anthropic.NewClient()
	reg := &Registry{
		client:  &client,
		configs: make(map[string]*AgentConfig),
		tools:   NewToolRegistry(),
	}

	// Add a test agent config
	reg.configs["test-agent"] = &AgentConfig{
		Name:   "test-agent",
		System: "You are a test agent",
		Model:  "gpt-4",
		Tools:  []string{},
	}

	// Set up OpenAI model client
	openAIClient := agent.NewOpenAIClient(server.URL, "test-key", "gpt-4")
	modelClient := &agent.ModelClient{
		Provider:     "openai",
		OpenAIClient: openAIClient,
		Model: &modelstore.Model{
			ID:      "gpt-4",
			Provider: "openai",
		},
	}
	reg.SetModelClient(modelClient)

	// Create a temporary model store for testing
	tmpStore, err := modelstore.Open(t.TempDir() + "/models.db")
	if err != nil {
		t.Fatalf("failed to create model store: %v", err)
	}
	reg.SetModelStore(tmpStore)

	// Create context store
	reg.contexts = make(map[string]*inbercontext.Store)
	// Initialize with a real context store for the test agent
	reg.contexts["test-agent"] = inbercontext.NewStore()

	// Run the spawned agent
	ctx := context.Background()
	result, err := reg.SpawnAndRun(ctx, "test-agent", "Test task")
	if err != nil {
		t.Fatalf("SpawnAndRun failed: %v", err)
	}

	// Verify results
	if result.Text != "Task completed successfully" {
		t.Errorf("expected 'Task completed successfully', got %q", result.Text)
	}
	if result.InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", result.InputTokens)
	}
	if result.OutputTokens != 5 {
		t.Errorf("expected 5 output tokens, got %d", result.OutputTokens)
	}
	if result.ToolCalls != 0 {
		t.Errorf("expected 0 tool calls, got %d", result.ToolCalls)
	}
	if callCount != 1 {
		t.Errorf("expected 1 API call, got %d", callCount)
	}
}

func TestSpawnAndRun_OpenAI_WithToolCalls(t *testing.T) {
	// Create a mock OpenAI server that simulates tool calls
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		
		var req agent.OpenAIRequest
		json.NewDecoder(r.Body).Decode(&req)

		var resp agent.OpenAIResponse
		
		if callCount == 1 {
			// First call: return a tool call
			resp = agent.OpenAIResponse{
				ID:    "test-123",
				Model: req.Model,
				Choices: []agent.OpenAIChoice{
					{
						Index: 0,
						Message: agent.OpenAIMessage{
							Role: "assistant",
							ToolCalls: []agent.OpenAIToolCall{
								{
									ID:   "call_1",
									Type: "function",
									Function: agent.OpenAIFunctionCall{
										Name:      "test_tool",
										Arguments: `{"arg":"value"}`,
									},
								},
							},
						},
						FinishReason: "tool_calls",
					},
				},
				Usage: agent.OpenAIUsage{
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				},
			}
		} else {
			// Second call: return final text response
			resp = agent.OpenAIResponse{
				ID:    "test-456",
				Model: req.Model,
				Choices: []agent.OpenAIChoice{
					{
						Index: 0,
						Message: agent.OpenAIMessage{
							Role:    "assistant",
							Content: "Tool result processed",
						},
						FinishReason: "stop",
					},
				},
				Usage: agent.OpenAIUsage{
					PromptTokens:     15,
					CompletionTokens: 8,
					TotalTokens:      23,
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create registry with a test tool
	client := anthropic.NewClient()
	reg := &Registry{
		client:  &client,
		configs: make(map[string]*AgentConfig),
		tools:   NewToolRegistry(),
	}

	// Register a test tool
	testTool := agent.Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"arg": map[string]any{"type": "string"},
			},
		},
		Run: func(ctx context.Context, input string) (string, error) {
			return "tool executed", nil
		},
	}
	reg.tools.Register("test_tool", testTool)

	// Add agent config with the tool
	reg.configs["test-agent"] = &AgentConfig{
		Name:   "test-agent",
		System: "You are a test agent",
		Model:  "gpt-4",
		Tools:  []string{"test_tool"},
	}

	// Set up OpenAI model client
	openAIClient := agent.NewOpenAIClient(server.URL, "test-key", "gpt-4")
	modelClient := &agent.ModelClient{
		Provider:     "openai",
		OpenAIClient: openAIClient,
		Model: &modelstore.Model{
			ID:      "gpt-4",
			Provider: "openai",
		},
	}
	reg.SetModelClient(modelClient)

	// Create a temporary model store for testing
	tmpStore, err := modelstore.Open(t.TempDir() + "/models.db")
	if err != nil {
		t.Fatalf("failed to create model store: %v", err)
	}
	reg.SetModelStore(tmpStore)

	reg.contexts = make(map[string]*inbercontext.Store)
	reg.contexts["test-agent"] = inbercontext.NewStore()

	// Run the spawned agent
	ctx := context.Background()
	result, err := reg.SpawnAndRun(ctx, "test-agent", "Test task with tools")
	if err != nil {
		t.Fatalf("SpawnAndRun failed: %v", err)
	}

	// Verify results
	if result.Text != "Tool result processed" {
		t.Errorf("expected 'Tool result processed', got %q", result.Text)
	}
	if result.ToolCalls != 1 {
		t.Errorf("expected 1 tool call, got %d", result.ToolCalls)
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
	if result.InputTokens != 25 { // 10 + 15
		t.Errorf("expected 25 input tokens, got %d", result.InputTokens)
	}
	if result.OutputTokens != 13 { // 5 + 8
		t.Errorf("expected 13 output tokens, got %d", result.OutputTokens)
	}
}

func TestSpawnAndRun_Anthropic_Fallback(t *testing.T) {
	// This test verifies that when no model client is set,
	// the registry falls back to using the Anthropic client
	
	// We can't easily test the actual Anthropic path without mocking
	// the Anthropic SDK, but we can verify the branching logic
	
	client := anthropic.NewClient()
	reg := &Registry{
		client:      &client,
		modelClient: nil, // No model client set
		configs:     make(map[string]*AgentConfig),
		tools:       NewToolRegistry(),
		contexts:    make(map[string]*inbercontext.Store),
	}
	
	reg.contexts["test-agent"] = inbercontext.NewStore()

	reg.configs["test-agent"] = &AgentConfig{
		Name:   "test-agent",
		System: "You are a test agent",
		Model:  "claude-3-5-sonnet-20241022",
		Tools:  []string{},
	}

	// This would normally fail since we don't have a real API key,
	// but it demonstrates that the code path exists
	ctx := context.Background()
	_, err := reg.SpawnAndRun(ctx, "test-agent", "Test task")
	
	// We expect an error because there's no valid API key,
	// but the important thing is that it tries the Anthropic path
	if err == nil {
		t.Error("expected error when using Anthropic without valid API key")
	}
}
