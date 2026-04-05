package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kayushkin/inber/logger"
)

// PIDFile manages a PID file to prevent duplicate server instances.
type PIDFile struct {
	path string
}

// NewPIDFile creates a PID file manager at ~/.inber/server/inber.pid.
func NewPIDFile(dataDir string) *PIDFile {
	return &PIDFile{path: filepath.Join(dataDir, "inber.pid")}
}

// Acquire checks for a stale instance, kills it if found, and writes our PID.
func (p *PIDFile) Acquire() error {
	log := logger.WithComponent("pidfile")

	// Read existing PID file.
	data, err := os.ReadFile(p.path)
	if err == nil {
		pidStr := strings.TrimSpace(string(data))
		if oldPID, err := strconv.Atoi(pidStr); err == nil && oldPID > 0 {
			// Check if the process is still alive.
			proc, err := os.FindProcess(oldPID)
			if err == nil && proc.Signal(syscall.Signal(0)) == nil {
				// Process exists. Is it actually an inber serve?
				if isInberServe(oldPID) {
					log.Warn("killing stale inber serve", map[string]interface{}{
						"pid": oldPID,
					})
					// Graceful shutdown first.
					proc.Signal(syscall.SIGTERM)

					// Wait up to 5s for exit.
					gone := false
					for i := 0; i < 50; i++ {
						time.Sleep(100 * time.Millisecond)
						if proc.Signal(syscall.Signal(0)) != nil {
							gone = true
							break
						}
					}
					if !gone {
						log.Warn("stale instance didn't exit gracefully, sending SIGKILL", map[string]interface{}{
							"pid": oldPID,
						})
						proc.Signal(syscall.SIGKILL)
						time.Sleep(200 * time.Millisecond)
					}

					log.Info("stale instance terminated — any in-flight API calls are lost", map[string]interface{}{
						"killed_pid": oldPID,
					})
				}
			}
		}
	}

	// Write our PID.
	if err := os.MkdirAll(filepath.Dir(p.path), 0755); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}
	return os.WriteFile(p.path, []byte(strconv.Itoa(os.Getpid())), 0644)
}

// Release removes the PID file on clean shutdown.
func (p *PIDFile) Release() {
	os.Remove(p.path)
}

// isInberServe checks if a PID is an "inber serve" process by reading /proc/PID/cmdline.
func isInberServe(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	cmdline := string(data)
	return strings.Contains(cmdline, "inber") && strings.Contains(cmdline, "serve")
}
