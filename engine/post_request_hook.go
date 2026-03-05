package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// PostRequestHook checks if changes are properly pushed and deployed after a request.
// It can automatically run git push and deploy commands if a high-tier model approves.
type PostRequestHook struct {
	repoRoot      string
	client        *anthropic.Client
	modelStore    modelStore
	highTierModel string
	shellTool     agent.Tool

	// State tracking
	mu            sync.Mutex
	changedFiles  []string
	sessionBranch string
}

// modelStore interface to avoid circular import
type modelStore interface {
	Resolve(modelID string) (*modelInfo, error)
}

// modelInfo interface for resolved model
type modelInfo interface {
	ID() string
	Provider() string
}

// NewPostRequestHook creates a hook for post-request verification and deployment.
func NewPostRequestHook(repoRoot string, client *anthropic.Client, highTierModels []string, modelStore modelStore) *PostRequestHook {
	model := "claude-opus-4-5-20250414" // default to opus for deployment decisions
	if len(highTierModels) > 0 {
		model = highTierModels[0]
	}

	// Create a shell tool for executing deploy commands
	shellTool := agent.Tool{
		Name:        "shell",
		Description: "Execute shell commands",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to execute",
				},
			},
			Required: []string{"command"},
		},
		Run: func(ctx context.Context, input string) (string, error) {
			var params struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal([]byte(input), &params); err != nil {
				return "", fmt.Errorf("invalid input: %w", err)
			}
			cmd := exec.CommandContext(ctx, "bash", "-c", params.Command)
			cmd.Dir = repoRoot
			output, err := cmd.CombinedOutput()
			if err != nil {
				return string(output), fmt.Errorf("command failed: %w", err)
			}
			return string(output), nil
		},
	}

	return &PostRequestHook{
		repoRoot:      repoRoot,
		client:        client,
		modelStore:    modelStore,
		highTierModel: model,
		shellTool:     shellTool,
	}
}

// NewPostRequestVerifier is an alias for NewPostRequestHook for compatibility.
func NewPostRequestVerifier(repoRoot string, client *anthropic.Client, highTierModels []string) *PostRequestHook {
	return NewPostRequestHook(repoRoot, client, highTierModels, nil)
}

// Verify is an alias for Run for compatibility.
func (h *PostRequestHook) Verify(ctx context.Context) (*VerifyResult, error) {
	return h.Run(ctx)
}

// RecordChange records that a file was changed during this session.
func (h *PostRequestHook) RecordChange(filePath string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.changedFiles = append(h.changedFiles, filePath)
}

// SetSessionBranch records the session branch name for merge suggestions.
func (h *PostRequestHook) SetSessionBranch(branch string) {
	if h == nil {
		return
	}
	h.sessionBranch = branch
}

// ChangedFiles returns the list of files changed this session.
func (h *PostRequestHook) ChangedFiles() []string {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string{}, h.changedFiles...)
}

// VerifyResult contains the verification and deployment outcome.
type VerifyResult struct {
	HasChanges      bool
	UnpushedCommits int
	UncommittedFiles []string
	NeedsDeploy     bool
	DeployMechanism string
	LLMAnalysis     string
	ActionsTaken    []string
	Errors          []string
}

// Run executes the post-request verification and deployment.
// It checks git state, then asks a high-tier model to decide and execute actions.
func (h *PostRequestHook) Run(ctx context.Context) (*VerifyResult, error) {
	result := &VerifyResult{}

	// Gather git state
	result = h.gatherGitState(result)

	// If nothing to do, return early
	if !result.HasChanges && result.UnpushedCommits == 0 {
		return result, nil
	}

	// Detect deploy mechanism
	result.DeployMechanism, result.NeedsDeploy = h.detectDeployMechanism()

	// Ask the high-tier model to evaluate and take action
	analysis, actions, errors := h.runDeployAgent(ctx, result)
	result.LLMAnalysis = analysis
	result.ActionsTaken = actions
	result.Errors = append(result.Errors, errors...)

	return result, nil
}

// gatherGitState checks for uncommitted and unpushed changes.
func (h *PostRequestHook) gatherGitState(result *VerifyResult) *VerifyResult {
	// Check for uncommitted changes
	status := h.runGit("status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		result.HasChanges = true
		for _, line := range strings.Split(status, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				result.UncommittedFiles = append(result.UncommittedFiles, line)
			}
		}
	}

	// Check for unpushed commits
	unpushed := h.runGit("log", "@{u}..", "--oneline")
	if strings.TrimSpace(unpushed) != "" {
		lines := strings.Split(strings.TrimSpace(unpushed), "\n")
		result.UnpushedCommits = len(lines)
	}

	return result
}

// detectDeployMechanism checks for common deploy patterns.
func (h *PostRequestHook) detectDeployMechanism() (mechanism string, needsDeploy bool) {
	deployFiles := []struct {
		path    string
		command string
	}{
		{"deploy.sh", "./deploy.sh"},
		{"scripts/deploy.sh", "./scripts/deploy.sh"},
		{"Makefile", "make deploy"},
		{".github/workflows/deploy.yml", "git push (auto-deploys via GitHub Actions)"},
		{".github/workflows/ci.yml", "git push (auto-deploys via GitHub Actions)"},
		{"fly.toml", "fly deploy"},
		{"vercel.json", "vercel --prod"},
		{"netlify.toml", "netlify deploy --prod"},
		{"Dockerfile", "docker build && docker push"},
		{"update.sh", "./update.sh"},
	}

	for _, df := range deployFiles {
		path := filepath.Join(h.repoRoot, df.path)
		if _, err := os.Stat(path); err == nil {
			return df.command, true
		}
	}

	return "", false
}

// runDeployAgent asks the high-tier model to evaluate state and execute actions.
// Returns (analysis, actions_taken, errors).
func (h *PostRequestHook) runDeployAgent(ctx context.Context, result *VerifyResult) (string, []string, []string) {
	if h.client == nil {
		return "No client available for deploy check", nil, []string{"no API client"}
	}

	// Get recent commits and diff summary for context
	recentCommits := h.runGit("log", "--oneline", "-5")
	diffStat := h.runGit("diff", "--stat", "@{u}..")
	branchName := h.runGit("rev-parse", "--abbrev-ref", "HEAD")

	// Build the system prompt
	systemPrompt := `You are a deployment verification agent. Your job is to ensure code changes are properly pushed and deployed.

You have access to a shell tool to run commands. Use it to:
1. Push changes if needed (git push)
2. Run deployment commands if appropriate
3. Verify the deployment succeeded

IMPORTANT RULES:
- Only push if there are unpushed commits AND the changes look complete (not half-finished work)
- Only deploy if there's a clear deploy mechanism and changes affect production code
- Be conservative - if unsure, just report the state without taking action
- Always explain what you're doing and why

After completing your actions, provide a brief summary of:
- What state you found
- What actions you took (if any)
- The current state after your actions`

	// Build the user message with current state
	var userMsg strings.Builder
	userMsg.WriteString("Current git/deploy state:\n\n")
	userMsg.WriteString(fmt.Sprintf("Branch: %s\n", branchName))
	userMsg.WriteString(fmt.Sprintf("Session branch: %s\n", h.sessionBranch))
	userMsg.WriteString(fmt.Sprintf("Files changed this session: %s\n", strings.Join(h.changedFiles, ", ")))

	if result.HasChanges {
		userMsg.WriteString(fmt.Sprintf("\nUncommitted files (%d):\n", len(result.UncommittedFiles)))
		for _, f := range result.UncommittedFiles {
			userMsg.WriteString(fmt.Sprintf("  %s\n", f))
		}
	} else {
		userMsg.WriteString("\nWorking tree: clean (all changes committed)\n")
	}

	if result.UnpushedCommits > 0 {
		userMsg.WriteString(fmt.Sprintf("\nUnpushed commits: %d\n", result.UnpushedCommits))
		userMsg.WriteString("Recent commits:\n")
		userMsg.WriteString(recentCommits)
		userMsg.WriteString("\nChanges:\n")
		userMsg.WriteString(diffStat)
	} else {
		userMsg.WriteString("\nPush status: up-to-date with remote\n")
	}

	if result.NeedsDeploy {
		userMsg.WriteString(fmt.Sprintf("\nDeploy mechanism detected: %s\n", result.DeployMechanism))
	} else {
		userMsg.WriteString("\nNo obvious deploy mechanism detected.\n")
	}

	userMsg.WriteString("\nPlease evaluate this state and take appropriate action (push, deploy, or just report).")

	// Create messages for the agent
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(userMsg.String())),
	}

	// Build the agent with shell tool
	agent_ := agent.NewWithSystemBlocks(h.client, []anthropic.TextBlockParam{{Text: systemPrompt}})
	agent_.AddTool(h.shellTool)

	// Run the agent with timeout
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	resp, err := agent_.Run(ctx, h.highTierModel, &messages)
	if err != nil {
		return fmt.Sprintf("Deploy agent failed: %v", err), nil, []string{err.Error()}
	}

	// Extract actions taken from the response
	var actions []string
	var errors []string

	// The response text contains the summary
	analysis := resp.Text

	// Parse out any mentioned actions
	lowerText := strings.ToLower(resp.Text)
	if strings.Contains(lowerText, "pushed") || strings.Contains(lowerText, "push successful") {
		actions = append(actions, "git push")
	}
	if strings.Contains(lowerText, "deployed") || strings.Contains(lowerText, "deployment successful") {
		actions = append(actions, "deploy")
	}
	if strings.Contains(lowerText, "error") || strings.Contains(lowerText, "failed") {
		if !strings.Contains(lowerText, "no errors") && !strings.Contains(lowerText, "no action needed") {
			errors = append(errors, "deploy agent reported issues")
		}
	}

	return analysis, actions, errors
}

// runGit executes a git command and returns output.
func (h *PostRequestHook) runGit(args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = h.repoRoot
	output, _ := cmd.CombinedOutput()
	return string(output)
}

// Format returns a human-readable summary of the verification.
func (r *VerifyResult) Format() string {
	if !r.HasChanges && r.UnpushedCommits == 0 && len(r.ActionsTaken) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n┌─ Post-Request Verification ───\n")

	if len(r.UncommittedFiles) > 0 {
		sb.WriteString("│ ⚠️  Uncommitted changes:\n")
		for _, f := range r.UncommittedFiles {
			sb.WriteString(fmt.Sprintf("│    • %s\n", f))
		}
	}

	if r.UnpushedCommits > 0 {
		sb.WriteString(fmt.Sprintf("│ ⚠️  %d unpushed commit(s)\n", r.UnpushedCommits))
	}

	if r.NeedsDeploy && r.DeployMechanism != "" {
		sb.WriteString(fmt.Sprintf("│ 🚀 Deploy: %s\n", r.DeployMechanism))
	}

	if len(r.ActionsTaken) > 0 {
		sb.WriteString("│ ✅ Actions taken:\n")
		for _, a := range r.ActionsTaken {
			sb.WriteString(fmt.Sprintf("│    • %s\n", a))
		}
	}

	if len(r.Errors) > 0 {
		sb.WriteString("│ ❌ Issues:\n")
		for _, e := range r.Errors {
			sb.WriteString(fmt.Sprintf("│    • %s\n", e))
		}
	}

	if r.LLMAnalysis != "" {
		sb.WriteString("│\n│ Analysis:\n")
		for _, line := range strings.Split(r.LLMAnalysis, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				sb.WriteString(fmt.Sprintf("│   %s\n", line))
			}
		}
	}

	sb.WriteString("└───────────────────────────────\n")
	return sb.String()
}

// ShouldRun returns true if the post-request hook should run.
// It checks if there were actual file changes or if there's work to verify.
func (h *PostRequestHook) ShouldRun() bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.changedFiles) > 0
}

// QuickVerify does a fast check without LLM involvement.
// Returns (hasIssues, summary).
func (h *PostRequestHook) QuickVerify() (bool, string) {
	if h == nil {
		return false, ""
	}

	var issues []string

	// Check for uncommitted changes
	status := h.runGit("status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		lines := strings.Split(strings.TrimSpace(status), "\n")
		issues = append(issues, fmt.Sprintf("%d uncommitted file(s)", len(lines)))
	}

	// Check for unpushed commits
	unpushed := h.runGit("log", "@{u}..", "--oneline")
	if strings.TrimSpace(unpushed) != "" {
		lines := strings.Split(strings.TrimSpace(unpushed), "\n")
		issues = append(issues, fmt.Sprintf("%d unpushed commit(s)", len(lines)))
	}

	if len(issues) == 0 {
		return false, "✓ All changes pushed"
	}

	return true, "⚠️ " + strings.Join(issues, ", ")
}
