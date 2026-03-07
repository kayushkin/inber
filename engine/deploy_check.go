package engine

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// DeployCheckConfig controls the post-request deploy verification hook.
type DeployCheckConfig struct {
	Enabled bool // whether to run deploy checks after requests
}

// DefaultDeployCheckConfig returns safe defaults (enabled).
func DefaultDeployCheckConfig() DeployCheckConfig {
	return DeployCheckConfig{
		Enabled: true,
	}
}

// deployCheck runs after a request completes to verify changes are pushed and deployed.
// It gathers git status, then asks a high-tier model to evaluate the state.
// Returns a message to display (or empty if nothing needed) and whether the model
// wants to take action (in which case the returned message is the model's instruction).
func (e *Engine) deployCheck() (string, bool) {
	if e.workflowHooks == nil {
		return "", false
	}

	// Only check if there were actual file changes this session
	if len(e.workflowHooks.changedFiles) == 0 {
		return "", false
	}

	// Gather git state
	state := e.gatherDeployState()
	if state.clean && state.pushed {
		// Everything is committed and pushed — no action needed
		return "", false
	}

	// Build the prompt for the high-tier model
	prompt := buildDeployCheckPrompt(state, e.AgentName)

	// Ask a high-tier model to evaluate
	response, err := e.runDeployCheckModel(prompt)
	if err != nil {
		Log.Warn("deploy check failed: %v", err)
		return "", false
	}

	return response, true
}

// deployState captures the git/deploy state of the repo.
type deployState struct {
	clean       bool     // no uncommitted changes
	pushed      bool     // local branch is up-to-date with remote
	branch      string   // current branch name
	unpushed    int      // number of unpushed commits
	uncommitted []string // list of uncommitted files
	changedFiles []string // files changed this session
	hasDeployScript bool // whether a deploy script exists
	deployHint  string   // detected deploy mechanism
}

// gatherDeployState collects git and deploy information from the repo.
func (e *Engine) gatherDeployState() deployState {
	state := deployState{
		changedFiles: e.workflowHooks.changedFiles,
	}

	// Current branch
	branch, err := e.workflowHooks.git("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return state
	}
	state.branch = branch

	// Uncommitted changes
	status, _ := e.workflowHooks.git("status", "--porcelain")
	state.clean = status == ""
	if !state.clean {
		for _, line := range strings.Split(status, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				state.uncommitted = append(state.uncommitted, line)
			}
		}
	}

	// Unpushed commits
	unpushed, err := e.workflowHooks.git("log", "--oneline", "@{upstream}..HEAD")
	if err != nil {
		// No upstream or other error — assume unpushed
		if state.clean {
			// Check if there are any commits at all vs upstream
			_, fetchErr := e.workflowHooks.git("fetch", "--dry-run")
			if fetchErr != nil {
				state.pushed = true // can't determine, assume ok
			}
		}
	} else {
		state.pushed = unpushed == ""
		if !state.pushed {
			state.unpushed = len(strings.Split(strings.TrimSpace(unpushed), "\n"))
		}
	}

	// Detect deploy mechanism
	state.detectDeployMechanism(e.repoRoot)

	return state
}

// detectDeployMechanism checks for common deploy scripts/configs.
func (s *deployState) detectDeployMechanism(repoRoot string) {
	deployFiles := []struct {
		path string
		hint string
	}{
		{"deploy.sh", "deploy script: ./deploy.sh"},
		{"Makefile", "Makefile (check for deploy target)"},
		{"docker-compose.yml", "docker-compose"},
		{"fly.toml", "fly.io"},
		{".github/workflows", "GitHub Actions CI/CD"},
		{"Procfile", "Heroku"},
		{"render.yaml", "Render"},
		{"systemd/", "systemd service"},
		{"update.sh", "update script: ./update.sh"},
	}

	for _, df := range deployFiles {
		cmd := exec.Command("test", "-e", df.path)
		cmd.Dir = repoRoot
		if err := cmd.Run(); err == nil {
			s.hasDeployScript = true
			s.deployHint = df.hint
			return
		}
	}
}

// buildDeployCheckPrompt creates the prompt for the deploy verification model.
func buildDeployCheckPrompt(state deployState, agentName string) string {
	var sb strings.Builder

	sb.WriteString("You are a deploy verification agent. An AI agent just finished making code changes. ")
	sb.WriteString("Review the git/deploy state below and respond with a brief assessment.\n\n")

	sb.WriteString(fmt.Sprintf("Agent: %s\n", agentName))
	sb.WriteString(fmt.Sprintf("Branch: %s\n", state.branch))
	sb.WriteString(fmt.Sprintf("Files changed this session: %s\n", strings.Join(state.changedFiles, ", ")))

	if state.clean {
		sb.WriteString("Working tree: clean (all changes committed)\n")
	} else {
		sb.WriteString(fmt.Sprintf("Uncommitted files (%d):\n", len(state.uncommitted)))
		for _, f := range state.uncommitted {
			sb.WriteString(fmt.Sprintf("  %s\n", f))
		}
	}

	if state.pushed {
		sb.WriteString("Push status: up-to-date with remote\n")
	} else {
		sb.WriteString(fmt.Sprintf("Push status: %d unpushed commit(s)\n", state.unpushed))
	}

	if state.hasDeployScript {
		sb.WriteString(fmt.Sprintf("Deploy mechanism detected: %s\n", state.deployHint))
	} else {
		sb.WriteString("Deploy mechanism: none detected\n")
	}

	sb.WriteString("\nRespond with:\n")
	sb.WriteString("1. A one-line status summary\n")
	sb.WriteString("2. What actions are needed (push, deploy, etc.) — or 'No action needed' if everything is done\n")
	sb.WriteString("Keep it brief. 2-3 lines max.\n")

	return sb.String()
}

// runDeployCheckModel calls a high-tier model to evaluate deploy state.
func (e *Engine) runDeployCheckModel(prompt string) (string, error) {
	model := e.Model

	// Create a lightweight model client for this check
	mc, err := agent.NewModelClient(model, e.modelStore)
	if err != nil {
		return "", fmt.Errorf("failed to create deploy check model client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if mc.AnthropicClient == nil {
		return "", fmt.Errorf("no anthropic client available for deploy check")
	}

	resp, err := mc.AnthropicClient.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(512),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", err
	}

	var text string
	for _, block := range resp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return strings.TrimSpace(text), nil
}
