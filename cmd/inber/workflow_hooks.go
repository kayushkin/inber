package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorkflowHooks orchestrates auto-branch, auto-commit, auto-format, and auto-test.
type WorkflowHooks struct {
	repoRoot       string
	sessionID      string
	agentName      string
	projectType    string // "go", "node", "rust", ""
	
	// Config flags
	autoBranch     bool
	autoCommit     bool
	autoFormat     bool
	smartTests     bool
	
	// State
	sessionBranch  string
	lastError      string // for deduplication
	changedFiles   []string
}

// NewWorkflowHooks creates workflow automation for a session.
func NewWorkflowHooks(repoRoot, sessionID, agentName string, cfg AutoWorkflowConfig) *WorkflowHooks {
	h := &WorkflowHooks{
		repoRoot:    repoRoot,
		sessionID:   sessionID,
		agentName:   agentName,
		autoBranch:  cfg.AutoBranch,
		autoCommit:  cfg.AutoCommit,
		autoFormat:  cfg.AutoFormat,
		smartTests:  cfg.SmartTests,
	}
	h.detectProject()
	return h
}

func (h *WorkflowHooks) detectProject() {
	if _, err := os.Stat(filepath.Join(h.repoRoot, "go.mod")); err == nil {
		h.projectType = "go"
		return
	}
	if _, err := os.Stat(filepath.Join(h.repoRoot, "package.json")); err == nil {
		h.projectType = "node"
		return
	}
	if _, err := os.Stat(filepath.Join(h.repoRoot, "Cargo.toml")); err == nil {
		h.projectType = "rust"
		return
	}
	h.projectType = ""
}

// InitSession sets up the session branch and returns info message.
func (h *WorkflowHooks) InitSession() (string, error) {
	if !h.autoBranch {
		return "", nil
	}
	
	// Create branch name: inber/<agent>-<session-id>
	shortID := h.sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	branchName := fmt.Sprintf("inber/%s-%s", h.agentName, shortID)
	h.sessionBranch = branchName
	
	// Check if branch already exists (resume case)
	cmd := exec.Command("git", "-C", h.repoRoot, "rev-parse", "--verify", branchName)
	if cmd.Run() == nil {
		// Branch exists, check it out
		cmd = exec.Command("git", "-C", h.repoRoot, "checkout", branchName)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("failed to checkout branch %s: %w", branchName, err)
		}
		return fmt.Sprintf("Resumed session branch: %s", branchName), nil
	}
	
	// Create new branch
	cmd = exec.Command("git", "-C", h.repoRoot, "checkout", "-b", branchName)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}
	
	return fmt.Sprintf("Created session branch: %s", branchName), nil
}

// OnToolResult runs after a tool completes.
// Returns a message to inject into conversation (e.g., build errors).
func (h *WorkflowHooks) OnToolResult(toolName, toolInput, output string, isError bool) string {
	if isError {
		return "" // Don't process failed tool calls
	}
	
	// Only process file write tools
	if toolName != "write_file" && toolName != "edit_file" {
		return ""
	}
	
	// Extract file path from tool input
	filePath := h.extractFilePath(toolName, toolInput)
	if filePath == "" {
		return ""
	}
	
	// Track changed files
	h.changedFiles = append(h.changedFiles, filePath)
	
	var messages []string
	
	// 1. Auto-format
	if h.autoFormat {
		h.formatFile(filePath)
	}
	
	// 2. Auto-build/test
	if h.projectType != "" {
		if msg := h.buildAndTest(filePath); msg != "" {
			messages = append(messages, msg)
		}
	}
	
	// 3. Auto-commit
	if h.autoCommit && len(messages) == 0 {
		// Only commit if build/test passed
		if msg := h.commitFile(toolName, filePath); msg != "" {
			// Silent on success, only show errors
			if strings.Contains(msg, "error") || strings.Contains(msg, "failed") {
				messages = append(messages, msg)
			}
		}
	}
	
	return strings.Join(messages, "\n")
}

func (h *WorkflowHooks) extractFilePath(toolName, input string) string {
	// Parse JSON input to extract "path" field
	// Simple heuristic: look for "path":"..."
	if idx := strings.Index(input, `"path"`); idx != -1 {
		rest := input[idx+7:] // skip `"path":`
		if idx2 := strings.Index(rest, `"`); idx2 != -1 {
			rest = rest[idx2+1:]
			if idx3 := strings.Index(rest, `"`); idx3 != -1 {
				return rest[:idx3]
			}
		}
	}
	return ""
}

func (h *WorkflowHooks) formatFile(filePath string) {
	absPath := filepath.Join(h.repoRoot, filePath)
	
	switch h.projectType {
	case "go":
		exec.Command("gofmt", "-w", absPath).Run()
	case "node":
		exec.Command("npx", "prettier", "--write", absPath).Run()
	case "rust":
		exec.Command("rustfmt", absPath).Run()
	}
}

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

func (h *WorkflowHooks) buildAndTestGo(filePath string) string {
	// Build
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = h.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return h.dedup(compactGoError("build", string(output)))
	}
	
	// Test (smart selection if enabled)
	testCmd := []string{"test"}
	if h.smartTests && strings.HasSuffix(filePath, ".go") {
		// Test only the package containing this file
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
	
	return "" // Success (silent)
}

func (h *WorkflowHooks) buildAndTestNode() string {
	// npm test or yarn test
	cmd := exec.Command("npm", "test")
	cmd.Dir = h.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return h.dedup(fmt.Sprintf("⚠️ npm test failed:\n%s", string(output)))
	}
	return ""
}

func (h *WorkflowHooks) buildAndTestRust() string {
	cmd := exec.Command("cargo", "test")
	cmd.Dir = h.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return h.dedup(fmt.Sprintf("⚠️ cargo test failed:\n%s", string(output)))
	}
	return ""
}

func (h *WorkflowHooks) dedup(msg string) string {
	if msg == h.lastError {
		return "⚠️ same error as last build"
	}
	h.lastError = msg
	return msg
}

func (h *WorkflowHooks) commitFile(toolName, filePath string) string {
	// Generate smart commit message
	var msg string
	if toolName == "write_file" {
		msg = fmt.Sprintf("Create %s", filepath.Base(filePath))
	} else {
		msg = fmt.Sprintf("Update %s", filepath.Base(filePath))
	}
	
	// Stage file
	cmd := exec.Command("git", "-C", h.repoRoot, "add", filePath)
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("warning: failed to stage %s: %v", filePath, err)
	}
	
	// Commit
	cmd = exec.Command("git", "-C", h.repoRoot, "commit", "-m", msg)
	if err := cmd.Run(); err != nil {
		// Check if it's just "nothing to commit"
		if strings.Contains(err.Error(), "nothing to commit") {
			return "" // Silent
		}
		return fmt.Sprintf("warning: failed to commit %s: %v", filePath, err)
	}
	
	return "" // Success (silent)
}

// FinishSession returns a summary message for the user.
func (h *WorkflowHooks) FinishSession() string {
	if !h.autoBranch || h.sessionBranch == "" {
		return ""
	}
	
	fileCount := len(h.changedFiles)
	if fileCount == 0 {
		return fmt.Sprintf("Session branch: %s (no changes)", h.sessionBranch)
	}
	
	return fmt.Sprintf(`Session complete (%d file%s changed).
Branch: %s
Merge with: git merge --squash %s`,
		fileCount,
		plural(fileCount),
		h.sessionBranch,
		h.sessionBranch,
	)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// AutoWorkflowConfig controls which auto-workflows are enabled.
type AutoWorkflowConfig struct {
	AutoBranch bool // Create branch per session
	AutoCommit bool // Commit after every write
	AutoFormat bool // Run formatter on write
	SmartTests bool // Only run relevant tests
}

// DefaultAutoWorkflowConfig returns safe defaults.
func DefaultAutoWorkflowConfig() AutoWorkflowConfig {
	return AutoWorkflowConfig{
		AutoBranch: true,  // Safe and helpful
		AutoCommit: true,  // Saves tokens
		AutoFormat: true,  // Silent improvement
		SmartTests: false, // Needs more testing
	}
}
