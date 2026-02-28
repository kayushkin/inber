package registry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func TestSpawnManager_AsyncSpawning(t *testing.T) {
	// Skip if no API key
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	// Create temp directory for logs
	tmpDir := t.TempDir()
	logsDir := filepath.Join(tmpDir, "logs")
	configDir := filepath.Join(tmpDir, "agents")

	// Create minimal agent config
	os.MkdirAll(configDir, 0755)
	
	agentsJSON := `{
		"default": "test-agent",
		"agents": {
			"test-agent": {
				"name": "test-agent",
				"role": "test agent",
				"model": "claude-sonnet-4-5",
				"tools": ["shell"],
				"context": {
					"budget": 10000
				}
			}
		}
	}`
	
	identityMD := `# Test Agent
You are a test agent. When asked to echo something, use the shell tool to run 'echo <message>'.`
	
	if err := os.WriteFile(filepath.Join(tmpDir, "agents.json"), []byte(agentsJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "test-agent.md"), []byte(identityMD), 0644); err != nil {
		t.Fatal(err)
	}

	// Create registry
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	registry, err := New(&client, configDir, logsDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	ctx := context.Background()

	// Test 1: Async spawn returns immediately
	t.Run("async_returns_immediately", func(t *testing.T) {
		start := time.Now()
		taskID, err := registry.spawnManager.SpawnAsync(ctx, registry, "test-agent", "echo 'async test'", 30*time.Second)
		elapsed := time.Since(start)
		
		if err != nil {
			t.Fatalf("SpawnAsync failed: %v", err)
		}
		if taskID == "" {
			t.Fatal("Expected task ID, got empty string")
		}
		
		// Should return in <1 second (not wait for agent to complete)
		if elapsed > 1*time.Second {
			t.Errorf("SpawnAsync took too long: %v (expected <1s)", elapsed)
		}
		
		t.Logf("✅ Async spawn returned in %v with task ID: %s", elapsed, taskID)
		
		// Check initial status
		status, err := registry.spawnManager.GetStatus(taskID)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if status.Status != "running" && status.Status != "completed" {
			t.Errorf("Expected status 'running' or 'completed', got: %s", status.Status)
		}
		
		// Wait for completion
		result, err := registry.spawnManager.WaitForCompletion(taskID, 100*time.Millisecond, 60*time.Second)
		if err != nil {
			t.Fatalf("WaitForCompletion failed: %v", err)
		}
		
		if result.Status != "completed" {
			t.Errorf("Expected status 'completed', got: %s (error: %s)", result.Status, result.Error)
		}
		
		t.Logf("✅ Task completed: %s", result.Result)
	})

	// Test 2: Multiple async spawns run in parallel
	t.Run("parallel_spawning", func(t *testing.T) {
		start := time.Now()
		
		// Spawn 3 tasks
		taskIDs := make([]string, 3)
		for i := 0; i < 3; i++ {
			taskID, err := registry.spawnManager.SpawnAsync(ctx, registry, "test-agent", "echo 'parallel test'", 30*time.Second)
			if err != nil {
				t.Fatalf("Spawn %d failed: %v", i, err)
			}
			taskIDs[i] = taskID
		}
		
		spawnElapsed := time.Since(start)
		
		// Spawning all 3 should be fast (<2s)
		if spawnElapsed > 2*time.Second {
			t.Errorf("Spawning 3 tasks took too long: %v (expected <2s)", spawnElapsed)
		}
		
		t.Logf("✅ Spawned 3 tasks in %v: %v", spawnElapsed, taskIDs)
		
		// Wait for all to complete
		for i, taskID := range taskIDs {
			result, err := registry.spawnManager.WaitForCompletion(taskID, 100*time.Millisecond, 60*time.Second)
			if err != nil {
				t.Errorf("Task %d (%s) failed to complete: %v", i, taskID, err)
				continue
			}
			if result.Status != "completed" {
				t.Errorf("Task %d (%s) status: %s (error: %s)", i, taskID, result.Status, result.Error)
			}
		}
		
		totalElapsed := time.Since(start)
		t.Logf("✅ All 3 tasks completed in %v total", totalElapsed)
	})

	// Test 3: Result persistence to disk
	t.Run("result_persistence", func(t *testing.T) {
		taskID, err := registry.spawnManager.SpawnAsync(ctx, registry, "test-agent", "echo 'persistence test'", 30*time.Second)
		if err != nil {
			t.Fatalf("SpawnAsync failed: %v", err)
		}
		
		// Wait for completion
		_, err = registry.spawnManager.WaitForCompletion(taskID, 100*time.Millisecond, 60*time.Second)
		if err != nil {
			t.Fatalf("WaitForCompletion failed: %v", err)
		}
		
		// Check file exists
		resultPath := filepath.Join(registry.spawnManager.resultsDir, taskID+".json")
		if _, err := os.Stat(resultPath); os.IsNotExist(err) {
			t.Errorf("Result file not created: %s", resultPath)
		}
		
		// Create new manager (simulates restart)
		newManager := NewSpawnManager(logsDir)
		
		// Should be able to load result from disk
		result, err := newManager.GetStatus(taskID)
		if err != nil {
			t.Errorf("Failed to load result from disk: %v", err)
		}
		if result.Status != "completed" {
			t.Errorf("Loaded result has wrong status: %s", result.Status)
		}
		
		t.Logf("✅ Result persisted to disk and reloaded: %s", resultPath)
	})
}

func TestSpawnManager_BasicOperations(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewSpawnManager(filepath.Join(tmpDir, "logs"))

	// Test result directory creation
	if _, err := os.Stat(manager.resultsDir); os.IsNotExist(err) {
		t.Errorf("Results directory not created: %s", manager.resultsDir)
	}

	// Test empty list
	spawns := manager.ListSpawns()
	if len(spawns) != 0 {
		t.Errorf("Expected empty list, got %d spawns", len(spawns))
	}

	// Test non-existent task
	_, err := manager.GetStatus("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent task, got nil")
	}
}
