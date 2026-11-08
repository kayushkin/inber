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
				// Process exists. Is it running the same program we are?
				if runsThisExecutable(oldPID) {
					log.Warn("killing stale inber-server", map[string]interface{}{
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

// runsThisExecutable reports whether the process with the given pid runs the
// same executable file as this process, read from /proc/<pid>/exe.
//
// The pid comes out of a file an unclean exit leaves behind, and Linux reuses
// pids, so the live process at that number may be anything. This used to ask
// whether its command line contained "inber" and "serve", which an ordinary
// shell running a command that mentions this repo satisfies. The executable
// path is a fact the kernel asserts. When either path cannot be read, the
// answer is false: refusing to kill leaves two servers to fail on the port,
// which is loud; killing a stranger is not.
func runsThisExecutable(pid int) bool {
	log := logger.WithComponent("pidfile")
	ownExecutable, err := os.Readlink("/proc/self/exe")
	if err != nil {
		log.Warn("cannot read own executable; not killing the pid in the pid file", map[string]interface{}{
			"pid": pid, "error": err.Error(),
		})
		return false
	}
	otherExecutable, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		log.Warn("cannot read executable of the pid in the pid file; not killing it", map[string]interface{}{
			"pid": pid, "error": err.Error(),
		})
		return false
	}
	return sameExecutablePath(ownExecutable, otherExecutable)
}

// deletedExecutableSuffix is what the kernel appends to /proc/<pid>/exe when
// the file has been replaced or removed since the process started, as a
// deploy does to the incumbent's binary.
const deletedExecutableSuffix = " (deleted)"

// sameExecutablePath compares two /proc/<pid>/exe link targets, treating a
// binary replaced in place by a deploy as the same path.
func sameExecutablePath(ownExecutable, otherExecutable string) bool {
	return strings.TrimSuffix(ownExecutable, deletedExecutableSuffix) ==
		strings.TrimSuffix(otherExecutable, deletedExecutableSuffix)
}
