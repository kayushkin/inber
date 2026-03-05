package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
<<<<<<< Updated upstream
=======
	"strings"
>>>>>>> Stashed changes
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// SpawnManager tracks running sub-agents and their results
type SpawnManager struct {
	mu         sync.RWMutex
	spawns     map[string]*SpawnedAgent
	resultsDir string
	onComplete func(*SpawnedAgent)    // called when a spawn finishes
	pending    []*SpawnedAgent         // completed spawns not yet seen by orchestrator
	pendingMu  sync.Mutex
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

<<<<<<< Updated upstream
// SpawnAsync launches a sub-agent in the background and returns immediately
=======
// SpawnAsync launches a sub-agent as a fully detached shell process.
// The shell script captures output and writes the result JSON itself,
// so it works even if the parent Go process exits immediately.
>>>>>>> Stashed changes
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
<<<<<<< Updated upstream
	
	// Launch goroutine with background context so spawn survives parent exit
	go sm.runAgent(context.Background(), registry, spawn, timeout)
	
=======

	inberBin, err := os.Executable()
	if err != nil {
		inberBin = "inber"
	}

	lowerAgent := strings.ToLower(agentName)
	resultPath := filepath.Join(sm.resultsDir, taskID+".json")
	taskFile := resultPath + ".task"

	// Write task to file to avoid shell escaping issues
	if err := os.WriteFile(taskFile, []byte(task), 0644); err != nil {
		sm.failSpawn(spawn, fmt.Sprintf("write task file: %v", err))
		return taskID, nil
	}

	// Build a self-contained shell script that:
	// 1. Runs inber with the task as stdin
	// 2. Captures stdout and stderr
	// 3. Writes the result JSON file
	// 4. Cleans up temp files
	timeoutSec := int(timeout.Seconds())
	startedAt := spawn.StartedAt.Format(time.RFC3339Nano)

	script := fmt.Sprintf(
		`cd %q && timeout %d %q run --agent %q --detach --raw < %q > /tmp/spawn-%s.out 2>/tmp/spawn-%s.err; `+
			`EXIT=$?; `+
			`OUTPUT=$(cat /tmp/spawn-%s.out 2>/dev/null); `+
			`ERR=$(cat /tmp/spawn-%s.err 2>/dev/null); `+
			`if [ $EXIT -eq 0 ] && [ -n "$OUTPUT" ]; then STATUS=completed; else STATUS=failed; fi; `+
			`python3 -c "
import json, sys, datetime
result = {
    'id': '%s',
    'agent': '%s',
    'task': open('%s').read(),
    'started_at': '%s',
    'completed_at': datetime.datetime.utcnow().isoformat() + 'Z',
    'status': '$STATUS',
    'result': open('/tmp/spawn-%s.out').read().strip() if '$STATUS' == 'completed' else '',
    'error': open('/tmp/spawn-%s.err').read().strip() if '$STATUS' == 'failed' else ''
}
with open('%s', 'w') as f:
    json.dump(result, f, indent=2)
" 2>/dev/null; `+
			`rm -f %q /tmp/spawn-%s.out /tmp/spawn-%s.err`,
		sm.repoRoot, timeoutSec, inberBin, lowerAgent, taskFile, taskID, taskID,
		taskID, taskID,
		taskID, agentName, taskFile, startedAt,
		taskID, taskID, resultPath,
		taskFile, taskID, taskID,
	)

	cmd := exec.Command("nohup", "sh", "-c", script)
	cmd.Dir = sm.repoRoot
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		sm.failSpawn(spawn, fmt.Sprintf("start: %v", err))
		os.Remove(taskFile)
		return taskID, nil
	}

	// Release — don't wait
	go cmd.Wait()

>>>>>>> Stashed changes
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

	// Notify completion
	if sm.onComplete != nil {
		sm.onComplete(spawn)
	}
<<<<<<< Updated upstream
=======

	return taskID, nil
}

// failSpawn marks a spawn as failed and writes the result
func (sm *SpawnManager) failSpawn(spawn *SpawnedAgent, errMsg string) {
	sm.mu.Lock()
	spawn.Status = "failed"
	spawn.Error = errMsg
	spawn.CompletedAt = time.Now()
	sm.mu.Unlock()
	sm.writeResult(spawn)
>>>>>>> Stashed changes
}

// GetStatus returns the status of a spawned agent
func (sm *SpawnManager) GetStatus(taskID string) (*SpawnedAgent, error) {
	// Always check disk first (child process may have updated it)
	diskSpawn, diskErr := sm.loadResult(taskID)
	if diskErr == nil {
		return diskSpawn, nil
	}

	sm.mu.RLock()
	spawn, ok := sm.spawns[taskID]
	sm.mu.RUnlock()
	
	if !ok {
<<<<<<< Updated upstream
		// Try loading from disk
		return sm.loadResult(taskID)
=======
		return nil, fmt.Errorf("task %s not found", taskID)
>>>>>>> Stashed changes
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

// SetOnComplete sets a callback that fires when any spawn finishes.
// The callback runs in the spawn's goroutine — keep it fast.
func (sm *SpawnManager) SetOnComplete(fn func(*SpawnedAgent)) {
	sm.onComplete = fn
}

// EnablePendingQueue enables queuing completed spawns for DrainPending.
// Call this to have the orchestrator auto-inject results on the next turn.
func (sm *SpawnManager) EnablePendingQueue() {
	sm.onComplete = func(spawn *SpawnedAgent) {
		sm.pendingMu.Lock()
		sm.pending = append(sm.pending, spawn)
		sm.pendingMu.Unlock()
	}
}

// DrainPending returns and clears all completed spawns since last drain.
// The orchestrator calls this at the start of each turn to inject results.
func (sm *SpawnManager) DrainPending() []*SpawnedAgent {
	sm.pendingMu.Lock()
	defer sm.pendingMu.Unlock()
	if len(sm.pending) == 0 {
		return nil
	}
	result := sm.pending
	sm.pending = nil
	return result
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
