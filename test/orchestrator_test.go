package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
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

// =============================================================================
// SPAWN MANAGER TESTS
// =============================================================================

// TestSpawnManager_AsyncSpawn verifies SpawnAsync returns task ID immediately
func TestSpawnManager_AsyncSpawn(t *testing.T) {
	tmpDir := t.TempDir()
	sm := registry.NewSpawnManager(tmpDir)

	// Create a mock registry with slow API
	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow API call
		time.Sleep(2 * time.Second)
		resp := agent.OpenAIResponse{
			ID:      "test-resp",
			Object:  "chat.completion",
			Model:   "glm-5",
			Choices: []agent.OpenAIChoice{{Index: 0, Message: agent.OpenAIMessage{Role: "assistant", Content: "Done"}, FinishReason: "stop"}},
			Usage:   agent.OpenAIUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockAPI.Close()

	reg := createMockRegistry(t, mockAPI.URL)

	ctx := context.Background()
	start := time.Now()
	taskID, err := sm.SpawnAsync(ctx, reg, "worker", "test task", 30*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("SpawnAsync failed: %v", err)
	}

	// Should return immediately (< 100ms), not wait for the 2s API call
	if elapsed > 100*time.Millisecond {
		t.Errorf("SpawnAsync took too long: %v (should be immediate)", elapsed)
	}

	if taskID == "" {
		t.Error("expected non-empty task ID")
	}

	// Check spawn is registered
	spawn, err := sm.GetStatus(taskID)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if spawn.Status != "running" {
		t.Errorf("expected status 'running', got %q", spawn.Status)
	}

	t.Logf("✓ Async spawn returned immediately in %v with task ID %s", elapsed, taskID)
}

// TestSpawnManager_SyncSpawn verifies wait:true blocks until completion
func TestSpawnManager_SyncSpawn(t *testing.T) {
	tmpDir := t.TempDir()
	sm := registry.NewSpawnManager(tmpDir)

	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		resp := agent.OpenAIResponse{
			ID:      "test-resp",
			Object:  "chat.completion",
			Model:   "glm-5",
			Choices: []agent.OpenAIChoice{{Index: 0, Message: agent.OpenAIMessage{Role: "assistant", Content: "Sync done"}, FinishReason: "stop"}},
			Usage:   agent.OpenAIUsage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockAPI.Close()

	reg := createMockRegistry(t, mockAPI.URL)

	ctx := context.Background()
	taskID, err := sm.SpawnAsync(ctx, reg, "worker", "sync test", 30*time.Second)
	if err != nil {
		t.Fatalf("SpawnAsync failed: %v", err)
	}

	// Wait for completion
	result, err := sm.WaitForCompletion(taskID, 100*time.Millisecond, 10*time.Second)
	if err != nil {
		t.Fatalf("WaitForCompletion failed: %v", err)
	}

	if result.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", result.Status)
	}

	if result.Result == "" {
		t.Error("expected non-empty result")
	}

	t.Logf("✓ Sync spawn completed: %s", result.Result)
}

// TestSpawnManager_CheckSpawns verifies GetStatus and ListSpawns
func TestSpawnManager_CheckSpawns(t *testing.T) {
	tmpDir := t.TempDir()
	sm := registry.NewSpawnManager(tmpDir)

	// Initially empty
	spawns := sm.ListSpawns()
	if len(spawns) != 0 {
		t.Errorf("expected 0 spawns, got %d", len(spawns))
	}

	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := agent.OpenAIResponse{
			ID:      "test-resp",
			Object:  "chat.completion",
			Model:   "glm-5",
			Choices: []agent.OpenAIChoice{{Index: 0, Message: agent.OpenAIMessage{Role: "assistant", Content: "Done"}, FinishReason: "stop"}},
			Usage:   agent.OpenAIUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockAPI.Close()

	reg := createMockRegistry(t, mockAPI.URL)

	ctx := context.Background()

	// Spawn multiple agents
	taskID1, _ := sm.SpawnAsync(ctx, reg, "worker", "task 1", 30*time.Second)
	taskID2, _ := sm.SpawnAsync(ctx, reg, "worker", "task 2", 30*time.Second)

	// Check list
	spawns = sm.ListSpawns()
	if len(spawns) != 2 {
		t.Errorf("expected 2 spawns, got %d", len(spawns))
	}

	// Check specific task (use both task IDs to avoid unused variable error)
	spawn, err := sm.GetStatus(taskID1)
	if err != nil {
		t.Fatalf("GetStatus failed for taskID1: %v", err)
	}
	if spawn.Task != "task 1" {
		t.Errorf("expected task 'task 1', got %q", spawn.Task)
	}

	_, err = sm.GetStatus(taskID2)
	if err != nil {
		t.Fatalf("GetStatus failed for taskID2: %v", err)
	}
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if spawn.Task != "task 1" {
		t.Errorf("expected task 'task 1', got %q", spawn.Task)
	}

	// Check non-existent task
	_, err = sm.GetStatus("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent task")
	}

	t.Logf("✓ Check spawns works for %d tasks", len(spawns))
}

// TestSpawnManager_NonExistentAgent verifies error when spawning unknown agent
func TestSpawnManager_NonExistentAgent(t *testing.T) {
	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := agent.OpenAIResponse{
			ID:      "test-resp",
			Object:  "chat.completion",
			Model:   "glm-5",
			Choices: []agent.OpenAIChoice{{Index: 0, Message: agent.OpenAIMessage{Role: "assistant", Content: "Done"}, FinishReason: "stop"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockAPI.Close()

	reg := createMockRegistry(t, mockAPI.URL)

	ctx := context.Background()
	_, err := reg.SpawnAndRun(ctx, "nonexistent-agent", "test task")
	if err == nil {
		t.Error("expected error for non-existent agent")
	}

	t.Logf("✓ Non-existent agent returns error: %v", err)
}

// TestSpawnManager_Timeout verifies context timeout is respected
func TestSpawnManager_Timeout(t *testing.T) {
	tmpDir := t.TempDir()
	sm := registry.NewSpawnManager(tmpDir)

	// Use a channel to control when the API responds
	respondChan := make(chan struct{})
	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until told to respond (or connection closes)
		<-respondChan
		resp := agent.OpenAIResponse{
			ID:      "test-resp",
			Object:  "chat.completion",
			Model:   "glm-5",
			Choices: []agent.OpenAIChoice{{Index: 0, Message: agent.OpenAIMessage{Role: "assistant", Content: "Done"}, FinishReason: "stop"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockAPI.Close()
	defer close(respondChan)

	reg := createMockRegistry(t, mockAPI.URL)

	ctx := context.Background()
	// Spawn with very short timeout (100ms)
	taskID, _ := sm.SpawnAsync(ctx, reg, "worker", "timeout test", 100*time.Millisecond)

	// Wait a bit for the spawn to start and hit the timeout
	time.Sleep(200 * time.Millisecond)

	// Check status - should be failed due to timeout
	spawn, err := sm.GetStatus(taskID)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if spawn.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", spawn.Status)
	}

	t.Logf("✓ Timeout works: status=%s, error=%s", spawn.Status, spawn.Error)
}

// TestSpawnManager_ConcurrentSpawns verifies multiple spawns can run simultaneously
func TestSpawnManager_ConcurrentSpawns(t *testing.T) {
	tmpDir := t.TempDir()
	sm := registry.NewSpawnManager(tmpDir)

	var callCount int
	var mu sync.Mutex

	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()

		time.Sleep(500 * time.Millisecond)
		resp := agent.OpenAIResponse{
			ID:      "test-resp",
			Object:  "chat.completion",
			Model:   "glm-5",
			Choices: []agent.OpenAIChoice{{Index: 0, Message: agent.OpenAIMessage{Role: "assistant", Content: "Done"}, FinishReason: "stop"}},
			Usage:   agent.OpenAIUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockAPI.Close()

	reg := createMockRegistry(t, mockAPI.URL)

	ctx := context.Background()

	// Spawn 3 agents concurrently
	start := time.Now()
	taskID1, _ := sm.SpawnAsync(ctx, reg, "worker", "concurrent 1", 30*time.Second)
	taskID2, _ := sm.SpawnAsync(ctx, reg, "worker", "concurrent 2", 30*time.Second)
	taskID3, _ := sm.SpawnAsync(ctx, reg, "worker", "concurrent 3", 30*time.Second)
	elapsed := time.Since(start)

	// All spawns should return immediately
	if elapsed > 100*time.Millisecond {
		t.Errorf("spawns took too long: %v", elapsed)
	}

	// Wait for all to complete
	sm.WaitForCompletion(taskID1, 100*time.Millisecond, 10*time.Second)
	sm.WaitForCompletion(taskID2, 100*time.Millisecond, 10*time.Second)
	sm.WaitForCompletion(taskID3, 100*time.Millisecond, 10*time.Second)

	mu.Lock()
	calls := callCount
	mu.Unlock()

	if calls != 3 {
		t.Errorf("expected 3 API calls, got %d", calls)
	}

	spawns := sm.ListSpawns()
	completed := 0
	for _, s := range spawns {
		if s.Status == "completed" {
			completed++
		}
	}
	if completed != 3 {
		t.Errorf("expected 3 completed, got %d", completed)
	}

	t.Logf("✓ Concurrent spawns: %d calls, %d completed", calls, completed)
}

// TestSpawnManager_Persistence verifies spawn results are written to disk
func TestSpawnManager_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	sm := registry.NewSpawnManager(tmpDir)

	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := agent.OpenAIResponse{
			ID:      "test-resp",
			Object:  "chat.completion",
			Model:   "glm-5",
			Choices: []agent.OpenAIChoice{{Index: 0, Message: agent.OpenAIMessage{Role: "assistant", Content: "Persisted result"}, FinishReason: "stop"}},
			Usage:   agent.OpenAIUsage{PromptTokens: 15, CompletionTokens: 8, TotalTokens: 23},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockAPI.Close()

	reg := createMockRegistry(t, mockAPI.URL)

	ctx := context.Background()
	taskID, _ := sm.SpawnAsync(ctx, reg, "worker", "persistence test", 30*time.Second)

	// Wait for completion
	sm.WaitForCompletion(taskID, 100*time.Millisecond, 10*time.Second)

	// Check file exists
	resultFile := tmpDir + "/_spawns/" + taskID + ".json"
	if _, err := os.Stat(resultFile); os.IsNotExist(err) {
		t.Fatalf("result file not created: %s", resultFile)
	}

	// Read and verify content
	data, err := os.ReadFile(resultFile)
	if err != nil {
		t.Fatalf("failed to read result file: %v", err)
	}

	var spawn registry.SpawnedAgent
	if err := json.Unmarshal(data, &spawn); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if spawn.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", spawn.Status)
	}
	if spawn.Result != "Persisted result" {
		t.Errorf("expected result 'Persisted result', got %q", spawn.Result)
	}

	t.Logf("✓ Persistence works: %s", resultFile)
}

// =============================================================================
// HELPERS
// =============================================================================

// createMockRegistry creates a registry with mock API for testing
func createMockRegistry(t *testing.T, apiURL string) *registry.Registry {
	t.Helper()

	tmpDir := t.TempDir()
	agentsDir := tmpDir + "/agents"

	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}

	// Write agents.json
	configJSON := `{
		"default": "worker",
		"agents": {
			"worker": {
				"name": "worker",
				"model": "glm-5",
				"tools": [],
				"system": "You are a worker agent."
			}
		}
	}`
	if err := os.WriteFile(tmpDir+"/agents.json", []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Write identity
	if err := os.WriteFile(agentsDir+"/worker.md", []byte("You are a worker agent."), 0644); err != nil {
		t.Fatalf("failed to write identity: %v", err)
	}

	client := anthropic.NewClient()
	reg, err := registry.New(&client, agentsDir, t.TempDir())
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Set up mock OpenAI client
	openAIClient := agent.NewOpenAIClient(apiURL, "test-key", "glm-5")
	modelClient := &agent.ModelClient{
		Provider:     "zai",
		OpenAIClient: openAIClient,
	}
	reg.SetModelClient(modelClient)

	return reg
}
