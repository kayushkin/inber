package engine

import (
	"fmt"
	"strings"

	"github.com/kayushkin/repodetect"
)

// WorkflowHooks orchestrates auto-commit, auto-format, and auto-test.
type WorkflowHooks struct {
	repoRoot    string
	sessionID   string
	agentName   string
	projectType string // "go", "node", "rust", ""

	// Config flags
	autoCommit          bool
	autoFormat          bool
	smartTests          bool
	verifyDeployed      bool
	pushToDefaultBranch bool

	// State
	lastError    string // for deduplication
	changedFiles []string
}

// NewWorkflowHooks creates workflow automation for a session.
func NewWorkflowHooks(repoRoot, sessionID, agentName string, cfg AutoWorkflowConfig) *WorkflowHooks {
	h := &WorkflowHooks{
		repoRoot:            repoRoot,
		sessionID:           sessionID,
		agentName:           agentName,
		autoCommit:          cfg.AutoCommit,
		autoFormat:          cfg.AutoFormat,
		smartTests:          cfg.SmartTests,
		verifyDeployed:      cfg.VerifyDeployed,
		pushToDefaultBranch: cfg.PushToDefaultBranch,
	}
	h.detectProject()
	return h
}

// detectProject sets the project type used to pick build/format/test commands.
// It delegates to the shared repodetect ruleset (the single source of truth,
// its own module) rather than a local first-match copy: detection now sees every
// language present (a Go service with a React frontend reports both), and
// PrimaryLanguage collapses that to the one command-selection key the workflow
// hooks need. Preference order is backend-first (go > rust > python > node), so
// a mixed backend+frontend repo selects the backend's commands.
func (h *WorkflowHooks) detectProject() {
	sig, err := repodetect.Detect(h.repoRoot)
	if err != nil {
		h.projectType = ""
		return
	}
	h.projectType = sig.PrimaryLanguage()
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

// extractFilePath extracts the file path from tool input JSON.
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

// FinishSession returns a summary of work done.
func (h *WorkflowHooks) FinishSession() string {
	fileCount := len(h.changedFiles)

	// Handle git operations (commit, push)
	gitParts := h.finishSessionGit()

	var parts []string
	parts = append(parts, gitParts...)

	if fileCount > 0 {
		parts = append(parts, fmt.Sprintf("Session complete (%d file%s changed).", fileCount, plural(fileCount)))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

// plural returns "s" for pluralization if n != 1.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// AutoWorkflowConfig controls which auto-workflows are enabled.
type AutoWorkflowConfig struct {
	// AutoCommit is the session's authority to write git history on its own:
	// the commit after each write, and the sweep-up commit at session close.
	AutoCommit     bool
	AutoFormat     bool // Run formatter on write
	SmartTests     bool // Only run relevant tests
	VerifyDeployed bool // Check push/deploy status at session end
	// PushToDefaultBranch lets the end-of-session push publish to the branch
	// origin treats as its default. Off unless asked for: the session ends
	// with nobody watching, and a push to the shared branch cannot be undone.
	PushToDefaultBranch bool
}

// DefaultAutoWorkflowConfig returns safe defaults.
func DefaultAutoWorkflowConfig() AutoWorkflowConfig {
	return AutoWorkflowConfig{
		AutoCommit:          true,
		AutoFormat:          true,
		SmartTests:          false,
		VerifyDeployed:      false,
		PushToDefaultBranch: false,
	}
}