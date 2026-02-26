package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// ActiveSession represents a currently running session.
type ActiveSession struct {
	PID       int       `json:"pid"`
	Agent     string    `json:"agent"`
	Model     string    `json:"model"`
	StartTime time.Time `json:"start_time"`
	LogFile   string    `json:"log_file"`
	SessionID string    `json:"session_id"`
	Command   string    `json:"command"` // "chat" or "run"
}

// ActiveDir returns the path to the active sessions directory.
func ActiveDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".inber", "active")
}

// RegisterActive writes a lock file for the current session.
func RegisterActive(repoRoot string, sess *Session, command string) (string, error) {
	dir := ActiveDir(repoRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create active dir: %w", err)
	}

	active := ActiveSession{
		PID:       os.Getpid(),
		Agent:     sess.AgentName(),
		Model:     sess.model,
		StartTime: sess.start,
		LogFile:   sess.FilePath(),
		SessionID: sess.SessionID(),
		Command:   command,
	}

	data, err := json.MarshalIndent(active, "", "  ")
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, sess.SessionID()+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// UnregisterActive removes the lock file for a session.
func UnregisterActive(repoRoot, sessionID string) {
	path := filepath.Join(ActiveDir(repoRoot), sessionID+".json")
	os.Remove(path)
}

// ListActive reads all active session files and returns live ones, cleaning up stale entries.
func ListActive(repoRoot string) ([]ActiveSession, error) {
	dir := ActiveDir(repoRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var active []ActiveSession
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}

		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var sess ActiveSession
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}

		// Check if PID is still alive
		if !isProcessAlive(sess.PID) {
			os.Remove(path) // clean up stale
			continue
		}

		active = append(active, sess)
	}

	return active, nil
}

func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
