package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/agent/registry"
)

// TestInberCLI_BasicInvocation tests that inber can be invoked from command line
func TestInberCLI_BasicInvocation(t *testing.T) {
	// Build inber first
	cmd := exec.Command("go", "build", "-o", os.ExpandEnv("$HOME/bin/inber"), "./cmd/inber")
	cmd.Dir = "/home/slava/life/repos/inber"
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build inber: %v\n%s", err, output)
	}

	// Test basic invocation with GLM-5
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd = exec.CommandContext(ctx, os.ExpandEnv("$HOME/bin/inber"), "run", "--model", "glm-5", "--raw")
	cmd.Dir = "/home/slava/life/repos/inber"
	cmd.Stdin = strings.NewReader("Say 'test passed' and nothing else")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inber run failed: %v\n%s", err, output)
	}

	response := string(output)
	if !strings.Contains(response, "test passed") && !strings.Contains(response, "Test passed") {
		t.Errorf("unexpected response: %s", output)
	}

	t.Logf("✓ CLI invocation works: %s", strings.TrimSpace(response))
}

// TestInberCLI_SubAgentSpawn tests that inber can spawn sub-agents via CLI
func TestInberCLI_SubAgentSpawn(t *testing.T) {
	// Skip if no agents.json configured
	agentsPath := "/home/slava/life/repos/inber/agents.json"
	if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
		t.Skip("no agents.json configured")
	}

	// Build inber
	cmd := exec.Command("go", "build", "-o", os.ExpandEnv("$HOME/bin/inber"), "./cmd/inber")
	cmd.Dir = "/home/slava/life/repos/inber"
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build inber: %v\n%s", err, output)
	}

	// Run with an agent that has spawn capability
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// This test requires an orchestrator agent configured in agents.json
	cmd = exec.CommandContext(ctx, os.ExpandEnv("$HOME/bin/inber"), "run", "--model", "glm-5", "--agent", "orchestrator", "--raw")
	cmd.Dir = "/home/slava/life/repos/inber"
	cmd.Stdin = strings.NewReader("Use the spawn_agent tool to spawn a 'worker' agent with task 'Say hello'")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Might fail if no orchestrator configured - that's OK for now
		if strings.Contains(string(output), "agent not found") {
			t.Skip("orchestrator agent not configured")
		}
		t.Fatalf("inber spawn test failed: %v\n%s", err, output)
	}

	t.Logf("✓ Sub-agent spawn works: %s", strings.TrimSpace(string(output)))
}

// TestSubAgentSpawn_OpenAI tests spawning sub-agents with OpenAI-compatible API (GLM-5)
func TestSubAgentSpawn_OpenAI(t *testing.T) {
	// Create mock OpenAI server
	var apiCalls []string
	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agent.OpenAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		apiCalls = append(apiCalls, req.Model)

		// Return response based on whether this is a tool call or not
		resp := agent.OpenAIResponse{
			ID:      "test-resp",
			Object:  "chat.completion",
			Model:   req.Model,
			Choices: []agent.OpenAIChoice{
				{
					Index: 0,
					Message: agent.OpenAIMessage{
						Role:    "assistant",
						Content: "Task completed successfully.",
					},
					FinishReason: "stop",
				},
			},
			Usage: agent.OpenAIUsage{
				PromptTokens:     50,
				CompletionTokens: 20,
				TotalTokens:      70,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockAPI.Close()

	// Create temp config with proper directory structure
	// Registry.LoadConfigDir expects: agents.json at parent level, .md files in agents/ subdir
	tmpDir := t.TempDir()
	agentsDir := tmpDir + "/agents"

	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}

	// Write agents.json at parent level
	configJSON := `{
		"default": "worker",
		"agents": {
			"worker": {
				"name": "worker",
				"model": "glm-5",
				"tools": []
			}
		}
	}`
	if err := os.WriteFile(tmpDir+"/agents.json", []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Write worker.md identity file (required by registry)
	workerIdentity := "You are a worker agent. Complete tasks efficiently."
	if err := os.WriteFile(agentsDir+"/worker.md", []byte(workerIdentity), 0644); err != nil {
		t.Fatalf("failed to write worker identity: %v", err)
	}

	// Create registry
	client := anthropic.NewClient()
	reg, err := registry.New(&client, agentsDir, t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Set up OpenAI client pointing to mock
	openAIClient := agent.NewOpenAIClient(mockAPI.URL, "test-key", "glm-5")
	modelClient := &agent.ModelClient{
		Provider:     "zai",
		OpenAIClient: openAIClient,
	}
	reg.SetModelClient(modelClient)

	// Spawn agent
	ctx := context.Background()
	result, err := reg.SpawnAndRun(ctx, "worker", "Do something")
	if err != nil {
		t.Fatalf("SpawnAndRun failed: %v", err)
	}

	if result.Text == "" {
		t.Error("expected non-empty response")
	}
	if result.InputTokens == 0 {
		t.Error("expected input tokens to be tracked")
	}

	t.Logf("✓ OpenAI sub-agent spawn: in=%d out=%d", result.InputTokens, result.OutputTokens)
}

// TestSubAgentSpawn_ToolCalls tests spawning with tool calls
// Note: Tool registration requires internal access. This is tested in
// agent/registry/spawn_openai_test.go with proper tool registration.
func TestSubAgentSpawn_ToolCalls(t *testing.T) {
	t.Skip("Tool registration requires internal registry access - tested in spawn_openai_test.go")
}

// TestOrchestratorIntegration tests full orchestrator workflow
func TestOrchestratorIntegration(t *testing.T) {
	t.Skip("Integration test - run manually with real API")

	// This test requires:
	// 1. Real GLM-5 API credentials
	// 2. Configured agents.json with orchestrator and workers
	// 3. Working spawn_agent tool

	// Manual test command:
	// echo "Spawn a worker to say hello" | ~/bin/inber run --model glm-5 --agent orchestrator
}
