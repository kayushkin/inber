package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// Deploy returns a tool that lets agents deploy their current slot to dev preview.
// Agents call deploy() with no args — it auto-detects the project and slot from cwd.
// Only deploys to dev (slot preview). Prod deploys are orchestrator/dashboard only.
func Deploy() agent.Tool {
	return agent.Tool{
		Name:        "deploy",
		Description: "Deploy your current changes to the dev preview. Auto-detects project and slot from your working directory. Commits any uncommitted changes first. Only deploys to dev preview (N.dev.kayushkin.com), not production.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{},
		},
		Run: func(ctx context.Context, input string) (string, error) {
			return runDeploy(ctx)
		},
	}
}

func runDeploy(ctx context.Context) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}

	// Detect project and slot from cwd path
	// Expected: ~/life/repos/.pools/<project>/slot-<N>
	project, slot, err := detectSlot(cwd)
	if err != nil {
		return "", fmt.Errorf("not in a forge slot: %w", err)
	}

	// Call bus-agent API
	busURL := os.Getenv("BUS_AGENT_URL")
	if busURL == "" {
		busURL = "http://localhost:8101"
	}

	payload := map[string]interface{}{
		"project":      project,
		"slot":         slot,
		"triggered_by": agentName(),
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(busURL+"/api/forge/deploy", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("deploy request failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode != 200 {
		errMsg, _ := result["error"].(string)
		return "", fmt.Errorf("deploy failed: %s", errMsg)
	}

	deployID, _ := result["deploy_id"].(float64)
	return fmt.Sprintf("Deploy started (id: %.0f). Your changes will be live at http://%d.dev.kayushkin.com shortly.", deployID, slot), nil
}

// detectSlot parses the cwd to find project and slot number.
func detectSlot(cwd string) (project string, slot int, err error) {
	// Walk up looking for a slot-N directory inside .pools/<project>/
	dir := cwd
	for {
		base := filepath.Base(dir)
		parent := filepath.Dir(dir)
		parentBase := filepath.Base(parent)

		if strings.HasPrefix(base, "slot-") {
			// parent should be the project dir inside .pools
			grandparent := filepath.Base(filepath.Dir(parent))
			if grandparent == ".pools" {
				fmt.Sscanf(base, "slot-%d", &slot)
				project = parentBase
				return project, slot, nil
			}
		}

		if dir == parent {
			break
		}
		dir = parent
	}
	return "", 0, fmt.Errorf("cwd %s is not inside a forge slot (.pools/<project>/slot-N)", cwd)
}

// agentName tries to get the current agent name from env.
func agentName() string {
	if name := os.Getenv("INBER_AGENT"); name != "" {
		return name
	}
	return "agent"
}
