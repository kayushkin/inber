package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// PostRequestVerifier checks if changes are properly pushed and deployed after a request.
type PostRequestVerifier struct {
	repoRoot    string
	client      *anthropic.Client
	highTierModel string
}

// NewPostRequestVerifier creates a verifier for post-request checks.
func NewPostRequestVerifier(repoRoot string, client *anthropic.Client, highTierModels []string) *PostRequestVerifier {
	model := "claude-sonnet-4-5-20250929" // default
	if len(highTierModels) > 0 {
		model = highTierModels[0]
	}
	return &PostRequestVerifier{
		repoRoot:      repoRoot,
		client:        client,
		highTierModel: model,
	}
}

// VerifyResult contains the verification outcome.
type VerifyResult struct {
	HasChanges     bool
	UnpushedCommits int
	NeedsDeploy    bool
	DeployCommand  string
	Issues         []string
	Suggestions    []string
	LLMAnalysis    string
}

// Verify checks if the request's changes are properly pushed and deployed.
func (v *PostRequestVerifier) Verify(ctx context.Context) (*VerifyResult, error) {
	result := &VerifyResult{}

	// Check for uncommitted changes
	uncommitted := v.runGit("status", "--porcelain")
	if strings.TrimSpace(uncommitted) != "" {
		result.HasChanges = true
		result.Issues = append(result.Issues, "Uncommitted changes exist")
	}

	// Check for unpushed commits
	unpushed := v.runGit("log", "@{u}..", "--oneline")
	if strings.TrimSpace(unpushed) != "" {
		lines := strings.Split(strings.TrimSpace(unpushed), "\n")
		result.UnpushedCommits = len(lines)
		result.Issues = append(result.Issues, fmt.Sprintf("%d unpushed commit(s)", len(lines)))
	}

	// Detect deploy mechanism
	result.DeployCommand, result.NeedsDeploy = v.detectDeployMechanism()

	// If there are issues, get LLM analysis
	if len(result.Issues) > 0 || result.NeedsDeploy {
		analysis, err := v.getLLMAnalysis(ctx, result)
		if err != nil {
			Log.Warn("post-request LLM analysis failed: %v", err)
		} else {
			result.LLMAnalysis = analysis
		}
	}

	return result, nil
}

// detectDeployMechanism checks for common deploy patterns.
func (v *PostRequestVerifier) detectDeployMechanism() (command string, needsDeploy bool) {
	// Check for common deploy scripts/configs
	deployFiles := []struct {
		path    string
		command string
	}{
		{"deploy.sh", "./deploy.sh"},
		{"scripts/deploy.sh", "./scripts/deploy.sh"},
		{"Makefile", "make deploy"},
		{".github/workflows/deploy.yml", "git push (auto-deploys via GitHub Actions)"},
		{"fly.toml", "fly deploy"},
		{"vercel.json", "vercel --prod"},
		{"netlify.toml", "netlify deploy --prod"},
		{"Dockerfile", "docker build && docker push"},
		{"systemd/", "systemctl restart <service>"},
	}

	for _, df := range deployFiles {
		path := filepath.Join(v.repoRoot, df.path)
		if _, err := os.Stat(path); err == nil {
			return df.command, true
		}
	}

	return "", false
}

// getLLMAnalysis asks the high-tier model to verify and suggest actions.
func (v *PostRequestVerifier) getLLMAnalysis(ctx context.Context, result *VerifyResult) (string, error) {
	if v.client == nil {
		return "", nil
	}

	// Gather context
	gitStatus := v.runGit("status")
	gitLog := v.runGit("log", "--oneline", "-5")
	
	prompt := fmt.Sprintf(`You are verifying that a coding task was completed properly. Check if changes are pushed and deployed.

Current state:
- Repository: %s
- Unpushed commits: %d
- Deploy mechanism: %s
- Issues detected: %v

Git status:
%s

Recent commits:
%s

Respond with:
1. A brief assessment (1-2 sentences)
2. Specific actions needed (if any)
3. Commands to run (if any)

Be concise. If everything looks good, just say so.`,
		v.repoRoot,
		result.UnpushedCommits,
		result.DeployCommand,
		result.Issues,
		gitStatus,
		gitLog,
	)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := v.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(v.highTierModel),
		MaxTokens: 500,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	var text string
	for _, block := range resp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return text, nil
}

// runGit executes a git command and returns output.
func (v *PostRequestVerifier) runGit(args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = v.repoRoot
	output, _ := cmd.CombinedOutput()
	return string(output)
}

// Format returns a human-readable summary of the verification.
func (r *VerifyResult) Format() string {
	if len(r.Issues) == 0 && !r.NeedsDeploy {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n┌─ Post-Request Verification ───\n")

	if len(r.Issues) > 0 {
		sb.WriteString("│ ⚠️  Issues:\n")
		for _, issue := range r.Issues {
			sb.WriteString(fmt.Sprintf("│    • %s\n", issue))
		}
	}

	if r.NeedsDeploy && r.DeployCommand != "" {
		sb.WriteString(fmt.Sprintf("│ 🚀 Deploy with: %s\n", r.DeployCommand))
	}

	if r.LLMAnalysis != "" {
		sb.WriteString("│\n│ Analysis:\n")
		for _, line := range strings.Split(r.LLMAnalysis, "\n") {
			sb.WriteString(fmt.Sprintf("│   %s\n", line))
		}
	}

	sb.WriteString("└───────────────────────────────\n")
	return sb.String()
}
