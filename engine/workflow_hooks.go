package engine

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/kayushkin/inber/tools"
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

// fileWritingToolNames returns the names of the tools that change files on
// disk, read off the tools themselves so a rename has to move this set rather
// than slip past it. The last rename (`write_file` -> `write_files`, inber
// `2b105ae`) is what left the gate below matching nothing.
var fileWritingToolNames = sync.OnceValue(func() map[string]bool {
	return map[string]bool{
		tools.WriteFiles().Name: true,
		tools.EditFiles().Name:  true,
		// The pre-tool-store spellings. Still the only names the automation
		// gate below answers to, so they have to be recorded as well.
		"write_file": true,
		"edit_file":  true,
	}
})

// OnToolResult runs after a tool completes.
// Returns a message to inject into conversation (e.g., build errors, git issues).
func (h *WorkflowHooks) OnToolResult(toolName, toolInput, output string, isError bool) string {
	if isError {
		return ""
	}

	// Only process file write tools
	if !fileWritingToolNames()[toolName] {
		return ""
	}

	filePaths := h.extractFilePaths(toolInput)
	if len(filePaths) == 0 {
		return ""
	}

	// Record what the session changed. This is a fact about the session and is
	// kept whatever else runs: the close-time commit stages exactly this list,
	// so a session whose writes go unrecorded either commits nothing or, as it
	// did before, commits the whole working tree including other agents' work.
	h.changedFiles = append(h.changedFiles, filePaths...)

	// The automations below — formatter, build-and-test, per-write commit —
	// stay gated on the OLD tool names, so they remain as unreachable as they
	// have been since the rename. Re-arming them is a separate and much larger
	// change (a full `go build ./... && go test ./...` injected into the
	// conversation after every write on a live server): todo af237d64 decides
	// which of the three the user wants back. Recording a path costs nothing
	// and decides nothing, which is why it is not waiting on that answer.
	if toolName != "write_file" && toolName != "edit_file" {
		return ""
	}
	filePath := filePaths[0]

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

// writeToolInput is every shape inber's write and edit tools accept, in one
// struct. Both take a single file at the top level OR a batch — `files[]` for
// writes, `edits[]` for edits — and their own descriptions call the batch the
// preferred form (tool-store `tools/fs.go`).
type writeToolInput struct {
	Path  string `json:"path"`
	Files []struct {
		Path string `json:"path"`
	} `json:"files"`
	Edits []struct {
		Path string `json:"path"`
	} `json:"edits"`
}

// extractFilePaths returns every file path a write or edit tool call names.
//
// It parses the input rather than scanning it for the first `"path"`, which is
// what it used to do: a batch write of twenty files would have yielded one path
// and hidden the other nineteen from everything downstream, including the
// close-time commit's idea of what the session changed.
func (h *WorkflowHooks) extractFilePaths(input string) []string {
	var parsed writeToolInput
	if err := json.Unmarshal([]byte(input), &parsed); err != nil {
		// Say so. A write whose input cannot be read is a write whose paths go
		// unrecorded, and the close-time commit then leaves those files sitting
		// in the tree and blames them on somebody else. The likeliest cause is
		// not malformed JSON but an empty string: the input is looked up in
		// `toolInputsCache` by tool id, and a miss there is indistinguishable
		// from a tool that named no files.
		Log.Warn("auto-workflow: could not read the input of a file-writing tool (%v) — its writes will not be attributed to this session", err)
		return nil
	}

	var paths []string
	if parsed.Path != "" {
		paths = append(paths, parsed.Path)
	}
	for _, f := range parsed.Files {
		if f.Path != "" {
			paths = append(paths, f.Path)
		}
	}
	for _, e := range parsed.Edits {
		if e.Path != "" {
			paths = append(paths, e.Path)
		}
	}
	return paths
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