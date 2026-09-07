package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/internal/textutil"
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
	// Expected: ~/repos/.pools/<project>/slot-<N>
	project, slot, err := detectSlot(cwd)
	if err != nil {
		return "", fmt.Errorf("not in a forge slot: %w", err)
	}

	return postDeploy(deployBaseURL(), project, slot, agentName())
}

// deployBaseURL is the service that serves POST /api/forge/deploy.
//
// ⚠️ The default is the historical one and is believed WRONG: dash has no
// /api/forge/deploy handler, and the route is registered by forge
// (forge/cmd/forge/api.go). Which service this should name — and what to do
// about forge's handler being a stub that answers 200 — is an open question
// carried on its own card; it is not decided here. What this file no longer
// does is report a started deploy when the answer says nothing was started.
func deployBaseURL() string {
	if v := os.Getenv("BUS_AGENT_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://localhost:8101"
}

// postDeploy asks base to start a deploy of one slot and reports what it said.
//
// It builds the path, so a test that drives this pins the route literal as
// well as the handling of the answer — the call site is not left holding an
// untested string.
func postDeploy(base, project string, slot int, triggeredBy string) (string, error) {
	payload := map[string]interface{}{
		"project":      project,
		"slot":         slot,
		"triggered_by": triggeredBy,
	}
	body, _ := json.Marshal(payload)

	url := base + "/api/forge/deploy"
	resp, err := http.Post(url, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("deploy request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("deploy: reading the response from %s: %w", url, err)
	}

	// A body that does not decode is a fault, not an empty result. Reading it
	// past the error is how this used to report a deploy that never happened:
	// a service with no such route answers 200 with its single-page-app HTML,
	// the decode failed into an untouched nil map, the status was 200 so the
	// error branch below was skipped, and the caller was told the deploy had
	// started.
	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("deploy: %s answered %s with a body that is not a deploy result (%s): %s",
			url, resp.Status, resp.Header.Get("Content-Type"), firstLine(raw))
	}

	if resp.StatusCode != 200 {
		errMsg, _ := result["error"].(string)
		if errMsg == "" {
			errMsg = firstLine(raw)
		}
		return "", fmt.Errorf("deploy failed: %s — %s", resp.Status, errMsg)
	}

	// No deploy id means nothing was started, whatever the status said. The
	// route is a stub on at least one service that serves it, and a stub
	// answers 200; reporting "id: 0" as a started deploy is the same lie one
	// step further in.
	deployID, ok := result["deploy_id"].(float64)
	if !ok {
		return "", fmt.Errorf("deploy: %s answered %s but named no deploy_id, so nothing was started: %s",
			url, resp.Status, firstLine(raw))
	}
	return fmt.Sprintf("Deploy started (id: %.0f). Your changes will be live at http://%d.dev.kayushkin.com shortly.", deployID, slot), nil
}

// firstLine renders a response body for an error message without letting a
// large or multi-line one into the log.
func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return textutil.Truncate(s, 200)
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
