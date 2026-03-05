package engine

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// PostWriteHook runs build/test after file writes and returns error messages
// only on failure. Silent on success.
type PostWriteHook struct {
	repoRoot  string
	lastError string // for deduplication
	projectType string // "go", "node", "rust", "" (unknown)
}

// NewPostWriteHook creates a post-write hook that auto-detects project type.
func NewPostWriteHook(repoRoot string) *PostWriteHook {
	h := &PostWriteHook{repoRoot: repoRoot}
	h.detectProject()
	return h
}

func (h *PostWriteHook) detectProject() {
	// Check for Go project
	if _, err := os.Stat(filepath.Join(h.repoRoot, "go.mod")); err == nil {
		h.projectType = "go"
		return
	}
	// Future: package.json, Cargo.toml, etc.
	h.projectType = ""
}

// OnToolResult should be called after write_file or edit_file completes.
// Returns a non-empty string to inject into the conversation if build/test failed.
func (h *PostWriteHook) OnToolResult(toolName string) string {
	if toolName != "write_file" && toolName != "edit_file" {
		return ""
	}
	if h.projectType != "go" {
		return ""
	}
	return h.runGo()
}

func (h *PostWriteHook) runGo() string {
	// Build first
	buildCmd := exec.Command("go", "build", "./...")
	buildCmd.Dir = h.repoRoot
	var buildOut bytes.Buffer
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut

	if err := buildCmd.Run(); err != nil {
		msg := compactGoError("build", buildOut.String())
		return h.dedup(msg)
	}

	// Build passed, run tests
	testCmd := exec.Command("go", "test", "./...")
	testCmd.Dir = h.repoRoot
	var testOut bytes.Buffer
	testCmd.Stdout = &testOut
	testCmd.Stderr = &testOut

	if err := testCmd.Run(); err != nil {
		msg := compactGoError("test", testOut.String())
		return h.dedup(msg)
	}

	// All green — reset dedup state
	h.lastError = ""
	return ""
}

func (h *PostWriteHook) dedup(msg string) string {
	if msg == h.lastError {
		return "⚠️ same error as last build"
	}
	h.lastError = msg
	return msg
}

// compactGoError extracts the first few meaningful error lines from go build/test output.
var goErrorLine = regexp.MustCompile(`^(.+\.go:\d+:\d+:.+)$`)

func compactGoError(phase string, output string) string {
	lines := strings.Split(output, "\n")
	var errors []string
	seen := make(map[string]bool)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if goErrorLine.MatchString(line) {
			// Strip the repo prefix for brevity
			if !seen[line] {
				seen[line] = true
				errors = append(errors, line)
			}
			if len(errors) >= 5 {
				break
			}
		}
	}
	if len(errors) == 0 {
		// Fallback: first 3 non-empty lines
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				errors = append(errors, line)
				if len(errors) >= 3 {
					break
				}
			}
		}
	}
	return fmt.Sprintf("⚠️ %s failed:\n%s", phase, strings.Join(errors, "\n"))
}
