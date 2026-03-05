package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
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
	onComplete func(*SpawnedAgent)
	pending    []*SpawnedAgent
	pendingMu  sync.Mutex
}

// SpawnedAgent represents a running or completed sub-agent task
type SpawnedAgent struct {
	ID           string    `json:"id"`
	Agent        string    `json:"agent"`
	Task         string    `json:"task"`
	StartedAt    time.Time `json:"started_at"`
	CompletedAt  time.Time `json:"completed_at,omitempty"`
	Status       string    `json:"status"` // running, completed, failed
	Result       string    `json:"result,omitempty"`
	Error        string    `json:"error,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	ToolCalls    int       `json:"tool_calls,omitempty"`
}

// NewSpawnManager creates a manager for tracking spawned agents
func NewSpawnManager(logsDir string) *SpawnManager {
	resultsDir := filepath.Join(logsDir, "_spawns")
	os.MkdirAll(resultsDir, 0755)
	repoRoot := filepath.Dir(logsDir)

	return &SpawnManager{
		spawns:     make(map[string]*SpawnedAgent),
		resultsDir: resultsDir,
		repoRoot:   repoRoot,
	}
}

// SpawnAsync launches a sub-agent as a child process and returns immediately.
// The child process survives parent exit.
func (sm *SpawnManager) SpawnAsync(
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

	// Fork a child process
	inberBin, err := os.Executable()
	if err != nil {
		inberBin = "inber"
	}

	lowerAgent := strings.ToLower(agentName)
	args := []string{"run", "--agent", lowerAgent, "--detach", "--raw"}
	cmd := exec.Command(inberBin, args...)
	cmd.Dir = sm.repoRoot
	cmd.Env = os.Environ()
	// Detach child from parent's process group so it survives parent exit
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

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

	go func() {
		io.WriteString(stdin, task)
		stdin.Close()
	}()

	// Monitor child process in background
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

	return taskID, nil
}

// SpawnSync runs a sub-agent in-process and blocks until completion.
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

// failSpawn marks a spawn as failed and writes the result
func (sm *SpawnManager) failSpawn(spawn *SpawnedAgent, errMsg string) {
	sm.mu.Lock()
	spawn.Status = "failed"
	spawn.Error = errMsg
	spawn.CompletedAt = time.Now()
	sm.mu.Unlock()
	sm.writeResult(spawn)
}

// stripANSI removes ANSI escape codes from a string
func stripANSI(s string) string {
	result := strings.Builder{}
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '\x1b' && s[i+1] == '[' {
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
}

// GetStatus returns the status of a spawned agent
func (sm *SpawnManager) GetStatus(taskID string) (*SpawnedAgent, error) {
	sm.mu.RLock()
	spawn, ok := sm.spawns[taskID]
	sm.mu.RUnlock()

	if !ok {
		return sm.loadResult(taskID)
	}

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
	return os.WriteFile(resultPath, data, 0644)
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

// SetOnComplete sets a callback for when spawns finish
func (sm *SpawnManager) SetOnComplete(fn func(*SpawnedAgent)) {
	sm.onComplete = fn
}

// EnablePendingQueue enables queuing spawn completions for DrainPending.
func (sm *SpawnManager) EnablePendingQueue() {
	sm.onComplete = func(spawn *SpawnedAgent) {
		sm.pendingMu.Lock()
		defer sm.pendingMu.Unlock()
		sm.pending = append(sm.pending, spawn)
	}
}

// DrainPending returns completed spawns since last drain.
func (sm *SpawnManager) DrainPending() []*SpawnedAgent {
	sm.pendingMu.Lock()
	defer sm.pendingMu.Unlock()
	result := sm.pending
	sm.pending = nil
	return result
}

// CleanupCompleted removes completed spawns from memory (keeps disk records)
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
