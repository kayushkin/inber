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
	"syscall"
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
<<<<<<< Updated upstream

	// Find repo root (where inber binary runs from)
	repoRoot, _ := os.Getwd()
=======
	
	repoRoot := filepath.Dir(logsDir)
>>>>>>> Stashed changes

	return &SpawnManager{
		spawns:     make(map[string]*SpawnedAgent),
		resultsDir: resultsDir,
		repoRoot:   repoRoot,
	}
}

// SpawnAsync launches a sub-agent as a fully detached shell process.
// The shell script captures output and writes the result JSON itself,
// so it works even if the parent Go process exits immediately.
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
=======
	// Fork a child process that survives parent exit
>>>>>>> Stashed changes
	inberBin, err := os.Executable()
	if err != nil {
		inberBin = "inber"
	}

<<<<<<< Updated upstream
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
=======
	// Build command: inber run --agent <name> --detach --raw
	lowerAgent := strings.ToLower(agentName)
	args := []string{"run", "--agent", lowerAgent, "--detach", "--raw"}
	cmd := exec.Command(inberBin, args...)
	cmd.Dir = sm.repoRoot
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		sm.failSpawn(spawn, fmt.Sprintf("stdin pipe: %v", err))
		return taskID, nil
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		sm.failSpawn(spawn, fmt.Sprintf("stdout pipe: %v", err))
		return taskID, nil
	}

	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		sm.failSpawn(spawn, fmt.Sprintf("start: %v", err))
		return taskID, nil
	}

	// Write task and close stdin
	go func() {
		io.WriteString(stdin, task)
		stdin.Close()
	}()

	// Monitor in background — reads output then updates result file
	// This goroutine blocks on I/O (cmd.Wait), so it won't be GC'd
	// The child process runs independently; even if this goroutine dies,
	// the child completes but result file won't update.
	// For true fire-and-forget, the child could write its own result.
	go func() {
		timer := time.AfterFunc(timeout, func() {
			cmd.Process.Kill()
		})
		defer timer.Stop()

		outData, _ := io.ReadAll(stdout)
		errData, _ := io.ReadAll(stderr)
		waitErr := cmd.Wait()

		sm.mu.Lock()
		spawn.CompletedAt = time.Now()
		responseText := strings.TrimSpace(string(outData))
		if waitErr != nil && responseText == "" {
			spawn.Status = "failed"
			errMsg := strings.TrimSpace(string(errData))
			if errMsg == "" {
				errMsg = waitErr.Error()
			}
			// Strip ANSI codes from error
			spawn.Error = stripANSI(errMsg)
		} else {
			spawn.Status = "completed"
			spawn.Result = responseText
		}
		sm.mu.Unlock()
		sm.writeResult(spawn)

		if sm.onComplete != nil {
			sm.onComplete(spawn)
		}
	}()
>>>>>>> Stashed changes

	return taskID, nil
}

<<<<<<< Updated upstream
// SpawnSync launches a sub-agent and blocks until completion (for wait:true mode)
func (sm *SpawnManager) SpawnSync(
	ctx context.Context,
	registry *Registry,
	agentName string,
	task string,
	timeout time.Duration,
) (*SpawnedAgent, error) {
	taskID, err := sm.SpawnAsync(ctx, registry, agentName, task, timeout)
	if err != nil {
		return nil, err
	}

	return sm.WaitForCompletion(taskID, 2*time.Second, timeout)
}

=======
>>>>>>> Stashed changes
// failSpawn marks a spawn as failed and writes the result
func (sm *SpawnManager) failSpawn(spawn *SpawnedAgent, errMsg string) {
	sm.mu.Lock()
	spawn.Status = "failed"
	spawn.Error = errMsg
	spawn.CompletedAt = time.Now()
	sm.mu.Unlock()
	sm.writeResult(spawn)
<<<<<<< Updated upstream
=======
}

// stripANSI removes ANSI escape codes from a string
func stripANSI(s string) string {
	// Simple approach: remove \x1b[...m sequences
	result := strings.Builder{}
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '\x1b' && s[i+1] == '[' {
			// Skip until 'm'
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
			continue
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
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
		return nil, fmt.Errorf("task %s not found", taskID)
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
