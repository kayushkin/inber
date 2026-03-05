package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SpawnManager tracks running sub-agents and their results
type SpawnManager struct {
	mu         sync.RWMutex
	spawns     map[string]*SpawnedAgent
	resultsDir string
	repoRoot   string
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

	// Derive repoRoot from logsDir (logs/ is inside the repo)
	repoRoot := filepath.Dir(logsDir)

	return &SpawnManager{
		spawns:     make(map[string]*SpawnedAgent),
		resultsDir: resultsDir,
		repoRoot:   repoRoot,
	}
}

// SpawnAsync launches a sub-agent as a separate process and returns immediately.
// The child process survives parent exit and writes results to disk.
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

	// Find inber binary
	inberBin, err := os.Executable()
	if err != nil {
		inberBin = "inber" // fallback to PATH
	}

	// Launch child process: inber run --agent <name> --detach --raw < task
	args := []string{"run", "--agent", agentName, "--detach", "--raw"}
	cmd := exec.Command(inberBin, args...)
	cmd.Dir = sm.repoRoot
	cmd.Env = os.Environ()

	// Pipe task as stdin
	stdin, err := cmd.StdinPipe()
	if err != nil {
		spawn.Status = "failed"
		spawn.Error = fmt.Sprintf("stdin pipe: %v", err)
		spawn.CompletedAt = time.Now()
		sm.writeResult(spawn)
		return taskID, nil // return taskID so caller can check status
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		spawn.Status = "failed"
		spawn.Error = fmt.Sprintf("stdout pipe: %v", err)
		spawn.CompletedAt = time.Now()
		sm.writeResult(spawn)
		return taskID, nil
	}

	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		spawn.Status = "failed"
		spawn.Error = fmt.Sprintf("start: %v", err)
		spawn.CompletedAt = time.Now()
		sm.writeResult(spawn)
		return taskID, nil
	}

	// Write task and close stdin
	go func() {
		io.WriteString(stdin, task)
		stdin.Close()
	}()

	// Monitor in background goroutine (reads output, waits for exit, updates JSON)
	go sm.monitorProcess(cmd, stdout, stderr, spawn, timeout)

	return taskID, nil
}

// monitorProcess waits for a spawned process to complete and updates the result file.
func (sm *SpawnManager) monitorProcess(
	cmd *exec.Cmd,
	stdout, stderr io.ReadCloser,
	spawn *SpawnedAgent,
	timeout time.Duration,
) {
	// Set up kill timer
	timer := time.AfterFunc(timeout, func() {
		cmd.Process.Kill()
	})
	defer timer.Stop()

	// Read stdout
	outData, _ := io.ReadAll(stdout)
	errData, _ := io.ReadAll(stderr)
	waitErr := cmd.Wait()

	// Update spawn record
	sm.mu.Lock()
	spawn.CompletedAt = time.Now()

	responseText := strings.TrimSpace(string(outData))

	if waitErr != nil && responseText == "" {
		spawn.Status = "failed"
		errMsg := waitErr.Error()
		if len(errData) > 0 {
			errMsg = strings.TrimSpace(string(errData))
		}
		spawn.Error = errMsg
	} else {
		spawn.Status = "completed"
		spawn.Result = responseText
	}
	sm.mu.Unlock()

	// Write final result to disk
	sm.writeResult(spawn)

	// Notify completion
	if sm.onComplete != nil {
		sm.onComplete(spawn)
	}
}

// SpawnSync launches a sub-agent in-process and blocks until completion.
func (sm *SpawnManager) SpawnSync(
	ctx context.Context,
	registry *Registry,
	agentName string,
	task string,
	timeout time.Duration,
) (string, error) {
	taskID := uuid.New().String()[:8]

	spawn := &SpawnedAgent{
		ID:        taskID,
		Agent:     agentName,
		Task:      task,
		StartedAt: time.Now(),
		Status:    "running",
	}

	sm.mu.Lock()
	sm.spawns[taskID] = spawn
	sm.mu.Unlock()

	sm.writeResult(spawn)

	// Run in-process with timeout
	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := registry.SpawnAndRun(taskCtx, spawn.Agent, spawn.Task)

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

	sm.writeResult(spawn)

	if sm.onComplete != nil {
		sm.onComplete(spawn)
	}

	return taskID, nil
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
