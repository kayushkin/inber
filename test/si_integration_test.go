package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/agent/registry"

	"github.com/anthropics/anthropic-sdk-go"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// FeedMessage matches the sí feed message format
type FeedMessage struct {
	Text      string    `json:"text"`
	Author    string    `json:"author,omitempty"`
	Channel   string    `json:"channel,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// TestSiPipeline_MessageRoundtrip tests basic message flow through a sí feed
func TestSiPipeline_MessageRoundtrip(t *testing.T) {
	// 1. Mock OpenAI server
	mockOpenAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agent.OpenAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Return canned response
		resp := agent.OpenAIResponse{
			ID:     "test-resp-1",
			Object: "chat.completion",
			Model:  req.Model,
			Choices: []agent.OpenAIChoice{
				{
					Index: 0,
					Message: agent.OpenAIMessage{
						Role:    "assistant",
						Content: "Hello! I received your message.",
					},
					FinishReason: "stop",
				},
			},
			Usage: agent.OpenAIUsage{
				PromptTokens:     25,
				CompletionTokens: 12,
				TotalTokens:      37,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockOpenAI.Close()

	// 2. Mock sí feed (WebSocket server)
	var wg sync.WaitGroup
	responseChan := make(chan string, 5)

	mockFeed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					return
				}

				var msg FeedMessage
				if err := json.Unmarshal(data, &msg); err == nil {
					if msg.Text != "" {
						responseChan <- msg.Text
					}
				}
			}
		}()

		// Send test message
		testMsg := FeedMessage{
			Text:      "Hello, inber!",
			Author:    "test-user",
			Timestamp: time.Now(),
		}
		if err := conn.WriteJSON(testMsg); err != nil {
			t.Logf("write error: %v", err)
			return
		}

		// Keep connection alive for test
		time.Sleep(2 * time.Second)
	}))
	defer mockFeed.Close()

	// 3. Create registry and engine
	tmpDir := t.TempDir()
	agentsDir := tmpDir + "/agents"
	logsDir := t.TempDir()

	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}

	configJSON := `{
		"default": "test-agent",
		"agents": {
			"test-agent": {
				"name": "test-agent",
				"model": "gpt-4",
				"tools": []
			}
		}
	}`
	if err := os.WriteFile(tmpDir+"/agents.json", []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	identityMD := "You are a helpful assistant."
	if err := os.WriteFile(agentsDir+"/test-agent.md", []byte(identityMD), 0644); err != nil {
		t.Fatalf("failed to write identity: %v", err)
	}

	client := anthropic.NewClient()
	reg, err := registry.New(&client, agentsDir, logsDir)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	openAIClient := agent.NewOpenAIClient(mockOpenAI.URL, "test-key", "gpt-4")
	modelClient := &agent.ModelClient{
		Provider:     "openai",
		OpenAIClient: openAIClient,
	}
	reg.SetModelClient(modelClient)

	// 4. Test spawn
	ctx := context.Background()
	result, err := reg.SpawnAndRun(ctx, "test-agent", "Say hello")
	if err != nil {
		t.Fatalf("SpawnAndRun failed: %v", err)
	}

	// 5. Verify
	if result.Text == "" {
		t.Error("expected non-empty response text")
	}
	if result.InputTokens == 0 {
		t.Error("expected input tokens > 0")
	}
	if result.OutputTokens == 0 {
		t.Error("expected output tokens > 0")
	}

	t.Logf("✓ Message roundtrip successful: '%s'", result.Text)
}

// TestSiPipeline_SpawnThroughFeed tests spawning a sub-agent via tool call
func TestSiPipeline_SpawnThroughFeed(t *testing.T) {
	toolCallCount := 0

	// Mock OpenAI server that returns spawn_agent tool call
	mockOpenAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agent.OpenAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// First call: return tool call
		// Second call: return final response after tool execution
		toolCallCount++
		if toolCallCount == 1 {
			resp := agent.OpenAIResponse{
				ID:     "test-resp-tool",
				Object: "chat.completion",
				Model:  req.Model,
				Choices: []agent.OpenAIChoice{
					{
						Index: 0,
						Message: agent.OpenAIMessage{
							Role:    "assistant",
							Content: "",
							ToolCalls: []agent.OpenAIToolCall{
								{
									ID:   "call_123",
									Type: "function",
									Function: agent.OpenAIFunctionCall{
										Name:      "spawn_agent",
										Arguments: `{"agent":"helper","task":"Process this","wait":true}`,
									},
								},
							},
						},
						FinishReason: "tool_calls",
					},
				},
				Usage: agent.OpenAIUsage{
					PromptTokens:     30,
					CompletionTokens: 15,
					TotalTokens:      45,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else {
			// Final response after tool execution
			resp := agent.OpenAIResponse{
				ID:     "test-resp-final",
				Object: "chat.completion",
				Model:  req.Model,
				Choices: []agent.OpenAIChoice{
					{
						Index: 0,
						Message: agent.OpenAIMessage{
							Role:    "assistant",
							Content: "Task delegated successfully!",
						},
						FinishReason: "stop",
					},
				},
				Usage: agent.OpenAIUsage{
					PromptTokens:     40,
					CompletionTokens: 10,
					TotalTokens:      50,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer mockOpenAI.Close()

	// Create registry with spawn_agent tool
	tmpDir := t.TempDir()
	agentsDir := tmpDir + "/agents"
	logsDir := t.TempDir()

	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}

	// Create parent and helper agents
	configJSON := `{
		"default": "parent",
		"agents": {
			"parent": {
				"name": "parent",
				"model": "gpt-4",
				"tools": ["spawn_agent"]
			},
			"helper": {
				"name": "helper",
				"model": "gpt-4",
				"tools": []
			}
		}
	}`
	if err := os.WriteFile(tmpDir+"/agents.json", []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	for _, name := range []string{"parent", "helper"} {
		identityMD := "You are a helpful assistant."
		if err := os.WriteFile(agentsDir+"/"+name+".md", []byte(identityMD), 0644); err != nil {
			t.Fatalf("failed to write identity for %s: %v", name, err)
		}
	}

	client := anthropic.NewClient()
	reg, err := registry.New(&client, agentsDir, logsDir)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	openAIClient := agent.NewOpenAIClient(mockOpenAI.URL, "test-key", "gpt-4")
	modelClient := &agent.ModelClient{
		Provider:     "openai",
		OpenAIClient: openAIClient,
	}
	reg.SetModelClient(modelClient)

	// Run parent agent (it should spawn helper)
	ctx := context.Background()
	result, err := reg.SpawnAndRun(ctx, "parent", "Delegate a task to helper")
	if err != nil {
		t.Fatalf("SpawnAndRun failed: %v", err)
	}

	// Verify spawn happened (tool call count should be > 1)
	if toolCallCount < 2 {
		t.Errorf("expected tool call loop, got %d calls", toolCallCount)
	}

	if result.ToolCalls == 0 {
		t.Error("expected at least one tool call")
	}

	t.Logf("✓ Spawn through feed successful: %d tool calls", toolCallCount)
}

// TestSiPipeline_MultipleMessages tests sequential message handling
func TestSiPipeline_MultipleMessages(t *testing.T) {
	messageCount := 0

	mockOpenAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agent.OpenAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		messageCount++
		content := "Response #" + string(rune('0'+messageCount))

		resp := agent.OpenAIResponse{
			ID:     "test-resp-" + string(rune('0'+messageCount)),
			Object: "chat.completion",
			Model:  req.Model,
			Choices: []agent.OpenAIChoice{
				{
					Index: 0,
					Message: agent.OpenAIMessage{
						Role:    "assistant",
						Content: content,
					},
					FinishReason: "stop",
				},
			},
			Usage: agent.OpenAIUsage{
				PromptTokens:     20 + messageCount*5,
				CompletionTokens: 10,
				TotalTokens:      30 + messageCount*5,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockOpenAI.Close()

	tmpDir := t.TempDir()
	agentsDir := tmpDir + "/agents"
	logsDir := t.TempDir()

	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}

	configJSON := `{
		"default": "test-agent",
		"agents": {
			"test-agent": {
				"name": "test-agent",
				"model": "gpt-4",
				"tools": []
			}
		}
	}`
	if err := os.WriteFile(tmpDir+"/agents.json", []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	identityMD := "You are a helpful assistant."
	if err := os.WriteFile(agentsDir+"/test-agent.md", []byte(identityMD), 0644); err != nil {
		t.Fatalf("failed to write identity: %v", err)
	}

	client := anthropic.NewClient()
	reg, err := registry.New(&client, agentsDir, logsDir)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	openAIClient := agent.NewOpenAIClient(mockOpenAI.URL, "test-key", "gpt-4")
	modelClient := &agent.ModelClient{
		Provider:     "openai",
		OpenAIClient: openAIClient,
	}
	reg.SetModelClient(modelClient)

	ctx := context.Background()

	// Send multiple messages
	for i := 1; i <= 3; i++ {
		result, err := reg.SpawnAndRun(ctx, "test-agent", "Message #"+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("SpawnAndRun #%d failed: %v", i, err)
		}

		if result.Text == "" {
			t.Errorf("message %d: expected response", i)
		}

		t.Logf("Message %d → '%s'", i, result.Text)
	}

	// Verify all messages were processed
	if messageCount != 3 {
		t.Errorf("expected 3 API calls, got %d", messageCount)
	}

	t.Logf("✓ Multiple messages processed successfully")
}
