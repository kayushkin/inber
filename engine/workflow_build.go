package engine

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// compactGoError extracts the first few meaningful error lines from go build/test output.
var goErrorLine = regexp.MustCompile(`^(.+\.go:\d+:\d+:.+)$`)

func compactGoError(phase string, output string) string {
	lines := strings.Split(output, "\n")
	var errors []string
	seen := make(map[string]bool)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if goErrorLine.MatchString(line) {
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

// buildAndTest handles build and test logic for different project types.
func (h *WorkflowHooks) buildAndTest(filePath string) string {
	switch h.projectType {
	case "go":
		return h.buildAndTestGo(filePath)
	case "node":
		return h.buildAndTestNode()
	case "rust":
		return h.buildAndTestRust()
	default:
		return ""
	}
}

// buildAndTestGo handles Go-specific build and test logic.
func (h *WorkflowHooks) buildAndTestGo(filePath string) string {
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = h.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return h.dedup(compactGoError("build", string(output)))
	}

	testCmd := []string{"test"}
	if h.smartTests && strings.HasSuffix(filePath, ".go") {
		pkg := "./" + filepath.Dir(filePath)
		testCmd = append(testCmd, pkg)
	} else {
		testCmd = append(testCmd, "./...")
	}

	cmd = exec.Command("go", testCmd...)
	cmd.Dir = h.repoRoot
	output, err = cmd.CombinedOutput()
	if err != nil {
		return h.dedup(compactGoError("test", string(output)))
	}

	return ""
}

// buildAndTestNode handles Node.js-specific build and test logic.
func (h *WorkflowHooks) buildAndTestNode() string {
	cmd := exec.Command("npm", "test")
	cmd.Dir = h.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return h.dedup(fmt.Sprintf("⚠️ npm test failed:\n%s", strings.TrimSpace(string(output))))
	}
	return ""
}

// buildAndTestRust handles Rust-specific build and test logic.
func (h *WorkflowHooks) buildAndTestRust() string {
	cmd := exec.Command("cargo", "test")
	cmd.Dir = h.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return h.dedup(fmt.Sprintf("⚠️ cargo test failed:\n%s", strings.TrimSpace(string(output))))
	}
	return ""
}

// dedup prevents repeated identical error messages.
func (h *WorkflowHooks) dedup(msg string) string {
	if msg == h.lastError {
		return "⚠️ same error as last build"
	}
	h.lastError = msg
	return msg
}