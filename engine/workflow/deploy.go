package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// VerifyDeployment checks if changes are pushed and deployed, returns issues if any.
func (h *WorkflowHooks) VerifyDeployment() (bool, string) {
	if !h.verifyDeployed {
		return false, ""
	}

	if len(h.changedFiles) == 0 {
		return false, ""
	}

	var issues []string

	// Check git status (uncommitted changes, unpushed commits)
	gitIssues := h.checkGitStatus()
	issues = append(issues, gitIssues...)

	// Check deployment readiness
	deployIssues := h.checkDeploymentStatus()
	if len(deployIssues) > 0 {
		issues = append(issues, strings.Join(deployIssues, "\n"))
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

// checkDeploymentStatus checks for deployment scripts and services.
func (h *WorkflowHooks) checkDeploymentStatus() []string {
	var deploymentChecks []string

	// Check for deployment script
	deployScript := h.findDeploymentScript()
	if deployScript != "" {
		deploymentChecks = append(deploymentChecks, fmt.Sprintf("Deployment script found: %s", deployScript))
	}

	// Check for systemd services (common for Go projects)
	serviceName := h.getServiceName()
	if serviceName != "" {
		cmd := exec.Command("systemctl", "is-active", serviceName)
		if err := cmd.Run(); err != nil {
			deploymentChecks = append(deploymentChecks, fmt.Sprintf("Service '%s' is not active", serviceName))
		}
	}

	return deploymentChecks
}

// findDeploymentScript looks for common deployment scripts.
func (h *WorkflowHooks) findDeploymentScript() string {
	for _, candidate := range []string{"update.sh", "deploy.sh", "scripts/deploy.sh"} {
		fullPath := filepath.Join(h.repoRoot, candidate)
		if _, err := os.Stat(fullPath); err == nil {
			return candidate
		}
	}
	return ""
}

// getServiceName extracts service name from go.mod for systemd checks.
func (h *WorkflowHooks) getServiceName() string {
	if h.projectType != "go" {
		return ""
	}

	modPath := filepath.Join(h.repoRoot, "go.mod")
	content, err := os.ReadFile(modPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		if len(parts) >= 2 {
			return filepath.Base(parts[1])
		}
	}
	return ""
}