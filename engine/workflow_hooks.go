package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorkflowHooks orchestrates auto-branch, auto-commit, auto-format, and auto-test.
type WorkflowHooks struct {
	repoRoot    string
	sessionID   string
	agentName   string
	projectType string // "go", "node", "rust", ""

	// Config flags
	autoBranch     bool
	autoCommit     bool
	autoFormat     bool
	smartTests     bool
	verifyDeployed bool

	// State
	sessionBranch string
	originalBranch string
	lastError     string // for deduplication
	changedFiles  []string
}

// NewWorkflowHooks creates workflow automation for a session.
func NewWorkflowHooks(repoRoot, sessionID, agentName string, cfg AutoWorkflowConfig) *WorkflowHooks {
	h := &WorkflowHooks{
		repoRoot:       repoRoot,
		sessionID:      sessionID,
		agentName:      agentName,
		autoBranch:     cfg.AutoBranch,
		autoCommit:     cfg.AutoCommit,
		autoFormat:     cfg.AutoFormat,
		smartTests:     cfg.SmartTests,
		verifyDeployed: cfg.VerifyDeployed,
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

// git runs a git command and returns combined output. Never panics.
func (h *WorkflowHooks) git(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", h.repoRoot}, args...)...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// InitSession sets up the session branch. Handles dirty worktrees,
// detached HEAD, merge conflicts, and other git weirdness gracefully.
// Returns (info message, injection for model to see, error).
func (h *WorkflowHooks) InitSession() (string, error) {
	if !h.autoBranch {
		return "", nil
	}

	// Remember where we started
	currentBranch, err := h.git("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		// Not a git repo or broken state — disable branching silently
		Log.Warn("auto-branch: not a git repo or broken state, disabling: %s", currentBranch)
		h.autoBranch = false
		return "", nil
	}
	h.originalBranch = currentBranch

	// Target branch name
	shortID := h.sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	branchName := fmt.Sprintf("inber/%s-%s", h.agentName, shortID)
	h.sessionBranch = branchName

	// Check for uncommitted changes
	status, _ := h.git("status", "--porcelain")
	hasChanges := status != ""

	// If dirty worktree, stash before switching
	if hasChanges {
		Log.Info("auto-branch: stashing uncommitted changes before switching")
		out, err := h.git("stash", "push", "-m", fmt.Sprintf("inber-auto-stash-%s", shortID))
		if err != nil {
			// Stash failed — work with what we have
			Log.Warn("auto-branch: stash failed (%s), trying branch switch anyway", out)
		}
	}

	// Try to switch to branch (existing or new)
	if out, err := h.git("rev-parse", "--verify", branchName); err == nil {
		_ = out
		// Branch exists — resume
		out, err := h.git("checkout", branchName)
		if err != nil {
			return h.recoverBranch(branchName, hasChanges, fmt.Sprintf("checkout failed: %s", out))
		}
		if hasChanges {
			h.git("stash", "pop") // best effort
		}
		return fmt.Sprintf("Resumed session branch: %s", branchName), nil
	}

	// Create new branch
	out, err := h.git("checkout", "-b", branchName)
	if err != nil {
		return h.recoverBranch(branchName, hasChanges, fmt.Sprintf("create branch failed: %s", out))
	}

	if hasChanges {
		h.git("stash", "pop") // best effort
	}
	return fmt.Sprintf("Created session branch: %s", branchName), nil
}

// recoverBranch handles git failures during branch setup.
// Tries to get back to a working state and disables auto-branching.
func (h *WorkflowHooks) recoverBranch(branchName string, hadStash bool, reason string) (string, error) {
	Log.Warn("auto-branch: %s — recovering", reason)

	// Try to get back to original branch
	if h.originalBranch != "" {
		h.git("checkout", h.originalBranch)
	}

	// Pop stash if we stashed
	if hadStash {
		h.git("stash", "pop")
	}

	// Disable auto-branch for this session
	h.autoBranch = false
	h.sessionBranch = ""

	// Don't fail the session — just warn
	return fmt.Sprintf("⚠️ auto-branch disabled: %s (continuing on %s)", reason, h.originalBranch), nil
}

// OnToolResult runs after a tool completes.
// Returns a message to inject into conversation (e.g., build errors, git issues).
func (h *WorkflowHooks) OnToolResult(toolName, toolInput, output string, isError bool) string {
	if isError {
		return ""
	}

	// Only process file write tools
	if toolName != "write_file" && toolName != "edit_file" {
		return ""
	}

	filePath := h.extractFilePath(toolName, toolInput)
	if filePath == "" {
		return ""
	}

	h.changedFiles = append(h.changedFiles, filePath)

	var messages []string

	// 1. Auto-format
	if h.autoFormat {
		if msg := h.formatFile(filePath); msg != "" {
			messages = append(messages, msg)
		}
	}

	// 2. Auto-build/test
	if h.projectType != "" {
		if msg := h.buildAndTest(filePath); msg != "" {
			messages = append(messages, msg)
		}
	}

	// 3. Auto-commit (only if build/test passed)
	if h.autoCommit && len(messages) == 0 {
		if msg := h.commitFile(toolName, filePath); msg != "" {
			messages = append(messages, msg)
		}
	}

	return strings.Join(messages, "\n")
}

func (h *WorkflowHooks) extractFilePath(toolName, input string) string {
	if idx := strings.Index(input, `"path"`); idx != -1 {
		rest := input[idx+7:]
		if idx2 := strings.Index(rest, `"`); idx2 != -1 {
			rest = rest[idx2+1:]
			if idx3 := strings.Index(rest, `"`); idx3 != -1 {
				return rest[:idx3]
			}
		}
	}
	return ""
}

func (h *WorkflowHooks) formatFile(filePath string) string {
	absPath := filepath.Join(h.repoRoot, filePath)

	var cmd *exec.Cmd
	switch h.projectType {
	case "go":
		cmd = exec.Command("gofmt", "-w", absPath)
	case "node":
		cmd = exec.Command("npx", "prettier", "--write", absPath)
	case "rust":
		cmd = exec.Command("rustfmt", absPath)
	default:
		return ""
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("⚠️ format failed for %s:\n%s", filePath, strings.TrimSpace(string(out)))
	}
	return ""
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

func (h *WorkflowHooks) buildAndTestNode() string {
	cmd := exec.Command("npm", "test")
	cmd.Dir = h.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return h.dedup(fmt.Sprintf("⚠️ npm test failed:\n%s", strings.TrimSpace(string(output))))
	}
	return ""
}

func (h *WorkflowHooks) buildAndTestRust() string {
	cmd := exec.Command("cargo", "test")
	cmd.Dir = h.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return h.dedup(fmt.Sprintf("⚠️ cargo test failed:\n%s", strings.TrimSpace(string(output))))
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
	var msg string
	if toolName == "write_file" {
		msg = fmt.Sprintf("Create %s", filepath.Base(filePath))
	} else {
		msg = fmt.Sprintf("Update %s", filepath.Base(filePath))
	}

	// Stage file
	out, err := h.git("add", filePath)
	if err != nil {
		return fmt.Sprintf("⚠️ git add failed for %s:\n%s\nFix this before continuing.", filePath, out)
	}

	// Check if there's actually something to commit
	out, _ = h.git("diff", "--cached", "--quiet")
	if out == "" {
		// diff --cached --quiet exits 0 if nothing staged
		// But we need to check the exit code, which we lost. Just try commit.
	}

	// Commit
	out, err = h.git("commit", "-m", msg)
	if err != nil {
		outStr := strings.TrimSpace(out)
		// "nothing to commit" is fine
		if strings.Contains(outStr, "nothing to commit") ||
			strings.Contains(outStr, "no changes added") {
			return ""
		}
		// Actual git error — surface it to the model
		return fmt.Sprintf("⚠️ git commit failed:\n%s\nFix this git issue before continuing.", outStr)
	}

	return ""
}

// FinishSession returns a summary. Tries to return to original branch.
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

// VerifyDeployment checks if changes are pushed and deployed, returns issues if any.
// Returns: (issuesFound, description)
func (h *WorkflowHooks) VerifyDeployment() (bool, string) {
	if !h.verifyDeployed {
		return false, ""
	}

	// No files changed? Nothing to verify.
	if len(h.changedFiles) == 0 {
		return false, ""
	}

	var issues []string

	// Check 1: Are there uncommitted changes?
	status, err := h.git("status", "--porcelain")
	if err == nil && status != "" {
		issues = append(issues, fmt.Sprintf("Uncommitted changes:\n%s", status))
	}

	// Check 2: Are committed changes pushed?
	// Compare local branch with remote tracking branch
	currentBranch, err := h.git("rev-parse", "--abbrev-ref", "HEAD")
	if err == nil && currentBranch != "" {
		// Check if we're ahead of remote
		ahead, err := h.git("rev-list", "--count", "@{u}..")
		if err == nil && ahead != "" && ahead != "0" {
			issues = append(issues, fmt.Sprintf("Branch '%s' has %s unpushed commits", currentBranch, ahead))
		}
	}

	// Check 3: Is there a deployment script? Has it been run?
	// Common patterns: update.sh, deploy.sh, Makefile with deploy target
	deployScript := ""
	for _, candidate := range []string{"update.sh", "deploy.sh", "scripts/deploy.sh"} {
		fullPath := filepath.Join(h.repoRoot, candidate)
		if _, err := os.Stat(fullPath); err == nil {
			deployScript = candidate
			break
		}
	}

	// Check for systemd services (common for Go projects)
	serviceName := ""
	if h.projectType == "go" {
		// Try to detect service name from go.mod
		modPath := filepath.Join(h.repoRoot, "go.mod")
		if content, err := os.ReadFile(modPath); err == nil {
			lines := strings.Split(string(content), "\n")
			if len(lines) > 0 {
				// First line: "module github.com/user/project"
				parts := strings.Fields(lines[0])
				if len(parts) >= 2 {
					modulePath := parts[1]
					projectName := filepath.Base(modulePath)
					serviceName = projectName
				}
			}
		}
	}

	var deploymentChecks []string
	if deployScript != "" {
		deploymentChecks = append(deploymentChecks, fmt.Sprintf("Deployment script found: %s", deployScript))
	}
	if serviceName != "" {
		// Check if service is running
		cmd := exec.Command("systemctl", "is-active", serviceName)
		if err := cmd.Run(); err != nil {
			deploymentChecks = append(deploymentChecks, fmt.Sprintf("Service '%s' is not active", serviceName))
		}
	}

	if len(deploymentChecks) > 0 {
		issues = append(issues, strings.Join(deploymentChecks, "\n"))
	}

	if len(issues) == 0 {
		return false, ""
	}

	description := fmt.Sprintf("Deployment verification found issues:\n\n%s\n\n"+
		"Changed files in this session:\n%s",
		strings.Join(issues, "\n\n"),
		strings.Join(h.changedFiles, "\n"),
	)

	return true, description
}

// AutoWorkflowConfig controls which auto-workflows are enabled.
type AutoWorkflowConfig struct {
	AutoBranch     bool // Create branch per session
	AutoCommit     bool // Commit after every write
	AutoFormat     bool // Run formatter on write
	SmartTests     bool // Only run relevant tests
	VerifyDeployed bool // Check push/deploy status at session end
}

// DefaultAutoWorkflowConfig returns safe defaults.
func DefaultAutoWorkflowConfig() AutoWorkflowConfig {
	return AutoWorkflowConfig{
		AutoBranch:     true,
		AutoCommit:     true,
		AutoFormat:     true,
		SmartTests:     false,
		VerifyDeployed: false, // opt-in for now
	}
}
