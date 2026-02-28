package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SpawnManager tracks running sub-agents and their results
type SpawnManager struct {
	mu        sync.RWMutex
	spawns    map[string]*SpawnedAgent
	resultsDir string
}

// SpawnedAgent represents a running or completed sub-agent task
type SpawnedAgent struct {
	ID          string    `json:"id"`
	Agent       string    `json:"agent"`
	Task        string    `json:"task"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Status      string    `json:"status"` // running, completed, failed
	Result      string    `json:"result,omitempty"`
	Error       string    `json:"error,omitempty"`
	InputTokens int       `json:"input_tokens,omitempty"`
	OutputTokens int      `json:"output_tokens,omitempty"`
	ToolCalls   int       `json:"tool_calls,omitempty"`
}

// NewSpawnManager creates a manager for tracking spawned agents
func NewSpawnManager(logsDir string) *SpawnManager {
	resultsDir := filepath.Join(logsDir, "_spawns")
	os.MkdirAll(resultsDir, 0755)
	
	return &SpawnManager{
		spawns:     make(map[string]*SpawnedAgent),
		resultsDir: resultsDir,
	}
}

// SpawnAsync launches a sub-agent in the background and returns immediately
func (sm *SpawnManager) SpawnAsync(
	ctx context.Context,
	registry *Registry,
	agentName string,
	task string,
	timeout time.Duration,
) (string, error) {
	// Generate unique task ID
	taskID := uuid.New().String()[:8]
	
	// Create spawn record
	spawn := &SpawnedAgent{
		ID:        taskID,
		Agent:     agentName,
		Task:      task,
		StartedAt: time.Now(),
		Status:    "running",
	}
	
	// Register spawn
	sm.mu.Lock()
	sm.spawns[taskID] = spawn
	sm.mu.Unlock()
	
	// Write initial status to disk
	sm.writeResult(spawn)
	
	// Launch goroutine to run the agent
	go sm.runAgent(ctx, registry, spawn, timeout)
	
	return taskID, nil
}

// runAgent executes the sub-agent and writes the result
func (sm *SpawnManager) runAgent(
	ctx context.Context,
	registry *Registry,
	spawn *SpawnedAgent,
	timeout time.Duration,
) {
	// Create context with timeout
	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	// Run the agent
	result, err := registry.SpawnAndRun(taskCtx, spawn.Agent, spawn.Task)
	
	// Update spawn record
	sm.mu.Lock()
	spawn.CompletedAt = time.Now()
	if err != nil {
		spawn.Status = "failed"
		spawn.Error = err.Error()
	} else {
		spawn.Status = "completed"
		spawn.Result = result.Text
		spawn.InputTokens = result.InputTokens
		spawn.OutputTokens = result.OutputTokens
		spawn.ToolCalls = result.ToolCalls
	}
	sm.mu.Unlock()
	
	// Write final result to disk
	sm.writeResult(spawn)
}

// GetStatus returns the status of a spawned agent
func (sm *SpawnManager) GetStatus(taskID string) (*SpawnedAgent, error) {
	sm.mu.RLock()
	spawn, ok := sm.spawns[taskID]
	sm.mu.RUnlock()
	
	if !ok {
		// Try loading from disk
		return sm.loadResult(taskID)
	}
	
	// Return a copy to avoid race conditions
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	spawnCopy := *spawn
	return &spawnCopy, nil
}

// ListSpawns returns all tracked spawns
func (sm *SpawnManager) ListSpawns() []*SpawnedAgent {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	spawns := make([]*SpawnedAgent, 0, len(sm.spawns))
	for _, spawn := range sm.spawns {
		spawnCopy := *spawn
		spawns = append(spawns, &spawnCopy)
	}
	return spawns
}

// writeResult writes spawn result to disk as JSON
func (sm *SpawnManager) writeResult(spawn *SpawnedAgent) error {
	resultPath := filepath.Join(sm.resultsDir, spawn.ID+".json")
	
	data, err := json.MarshalIndent(spawn, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	
	if err := os.WriteFile(resultPath, data, 0644); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	
	return nil
}

// loadResult loads spawn result from disk
func (sm *SpawnManager) loadResult(taskID string) (*SpawnedAgent, error) {
	resultPath := filepath.Join(sm.resultsDir, taskID+".json")
	
	data, err := os.ReadFile(resultPath)
	if err != nil {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	
	var spawn SpawnedAgent
	if err := json.Unmarshal(data, &spawn); err != nil {
		return nil, fmt.Errorf("parse result: %w", err)
	}
	
	return &spawn, nil
}

// WaitForCompletion blocks until a spawned agent completes or times out
func (sm *SpawnManager) WaitForCompletion(taskID string, pollInterval time.Duration, timeout time.Duration) (*SpawnedAgent, error) {
	deadline := time.Now().Add(timeout)
	
	for {
		spawn, err := sm.GetStatus(taskID)
		if err != nil {
			return nil, err
		}
		
		if spawn.Status != "running" {
			return spawn, nil
		}
		
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for task %s", taskID)
		}
		
		time.Sleep(pollInterval)
	}
}

// CleanupCompleted removes completed spawns from memory (but keeps disk records)
func (sm *SpawnManager) CleanupCompleted(olderThan time.Duration) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	cutoff := time.Now().Add(-olderThan)
	cleaned := 0
	
	for id, spawn := range sm.spawns {
		if spawn.Status != "running" && spawn.CompletedAt.Before(cutoff) {
			delete(sm.spawns, id)
			cleaned++
		}
	}
	
	return cleaned
}
