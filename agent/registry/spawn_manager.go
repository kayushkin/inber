package registry

import (
	"context"
	"encoding/json"
	"fmt"
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
	Status       string    `json:"status"`
	Result       string    `json:"result,omitempty"`
	Error        string    `json:"error,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	ToolCalls    int       `json:"tool_calls,omitempty"`
}

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
	taskID := uuid.New().String()[:8]
	spawn := &SpawnedAgent{
		ID: taskID, Agent: agentName, Task: task,
		StartedAt: time.Now(), Status: "running",
	}
	sm.mu.Lock()
	sm.spawns[taskID] = spawn
	sm.mu.Unlock()
	sm.writeResult(spawn)

	inberBin, err := os.Executable()
	if err != nil {
		inberBin = "inber"
	}

	lowerAgent := strings.ToLower(agentName)
	resultPath := filepath.Join(sm.resultsDir, taskID+".json")
	taskFile := resultPath + ".task"

	if err := os.WriteFile(taskFile, []byte(task), 0644); err != nil {
		sm.failSpawn(spawn, fmt.Sprintf("write task file: %v", err))
		return taskID, nil
	}

	timeoutSec := int(timeout.Seconds())
	startedAt := spawn.StartedAt.Format(time.RFC3339Nano)

	script := fmt.Sprintf(
		`cd %q && timeout %d %q run --agent %q --detach --raw < %q > /tmp/spawn-%s.out 2>/tmp/spawn-%s.err; `+
			`EXIT=$?; `+
			`OUTPUT=$(cat /tmp/spawn-%s.out 2>/dev/null); `+
			`ERR=$(cat /tmp/spawn-%s.err 2>/dev/null); `+
			`if [ $EXIT -eq 0 ] && [ -n "$OUTPUT" ]; then STATUS=completed; else STATUS=failed; fi; `+
			`python3 -c "`+
			`import json, sys, datetime; `+
			`result = {`+
			`'id': '%s', 'agent': '%s', `+
			`'task': open('%s').read(), `+
			`'started_at': '%s', `+
			`'completed_at': datetime.datetime.utcnow().isoformat() + 'Z', `+
			`'status': '$STATUS', `+
			`'result': open('/tmp/spawn-%s.out').read().strip() if '$STATUS' == 'completed' else '', `+
			`'error': open('/tmp/spawn-%s.err').read().strip() if '$STATUS' == 'failed' else '' `+
			`}; `+
			`json.dump(result, open('%s', 'w'), indent=2)`+
			`" 2>/dev/null; `+
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

	go cmd.Wait()
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
		ID: taskID, Agent: agentName, Task: task,
		StartedAt: time.Now(), Status: "running",
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

func (sm *SpawnManager) failSpawn(spawn *SpawnedAgent, errMsg string) {
	sm.mu.Lock()
	spawn.Status = "failed"
	spawn.Error = errMsg
	spawn.CompletedAt = time.Now()
	sm.mu.Unlock()
	sm.writeResult(spawn)
}

func (sm *SpawnManager) GetStatus(taskID string) (*SpawnedAgent, error) {
	// Check disk first (child process may have updated)
	if s, err := sm.loadResult(taskID); err == nil {
		return s, nil
	}
	sm.mu.RLock()
	spawn, ok := sm.spawns[taskID]
	sm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	c := *spawn
	return &c, nil
}

func (sm *SpawnManager) ListSpawns() []*SpawnedAgent {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	out := make([]*SpawnedAgent, 0, len(sm.spawns))
	for _, s := range sm.spawns {
		c := *s
		out = append(out, &c)
	}
	return out
}

func (sm *SpawnManager) writeResult(spawn *SpawnedAgent) error {
	p := filepath.Join(sm.resultsDir, spawn.ID+".json")
	d, err := json.MarshalIndent(spawn, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, d, 0644)
}

func (sm *SpawnManager) loadResult(taskID string) (*SpawnedAgent, error) {
	p := filepath.Join(sm.resultsDir, taskID+".json")
	d, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var s SpawnedAgent
	if err := json.Unmarshal(d, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (sm *SpawnManager) WaitForCompletion(taskID string, pollInterval, timeout time.Duration) (*SpawnedAgent, error) {
	deadline := time.Now().Add(timeout)
	for {
		s, err := sm.GetStatus(taskID)
		if err != nil {
			return nil, err
		}
		if s.Status != "running" {
			return s, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for task %s", taskID)
		}
		time.Sleep(pollInterval)
	}
}

func (sm *SpawnManager) SetOnComplete(fn func(*SpawnedAgent)) { sm.onComplete = fn }

func (sm *SpawnManager) EnablePendingQueue() {
	sm.onComplete = func(spawn *SpawnedAgent) {
		sm.pendingMu.Lock()
		defer sm.pendingMu.Unlock()
		sm.pending = append(sm.pending, spawn)
	}
}

func (sm *SpawnManager) DrainPending() []*SpawnedAgent {
	sm.pendingMu.Lock()
	defer sm.pendingMu.Unlock()
	r := sm.pending
	sm.pending = nil
	return r
}

func (sm *SpawnManager) CleanupCompleted(olderThan time.Duration) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	cutoff := time.Now().Add(-olderThan)
	n := 0
	for id, s := range sm.spawns {
		if s.Status != "running" && s.CompletedAt.Before(cutoff) {
			delete(sm.spawns, id)
			n++
		}
	}
	return n
}
