package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/internal/textutil"
)

// RegistryAgent represents an agent entry from the bus-agent registry.
type RegistryAgent struct {
	Name         string `json:"name"`
	Orchestrator string `json:"orchestrator"`
	Enabled      bool   `json:"enabled"`
}

// registryAgentsURL is the endpoint fetchRegistryAgents queries.
//
// ⚠️ Nothing serves this. Port 8101 belongs to dash-server, which registers no
// /api/agents route at all, so the request reaches dash's SPA fallback and comes
// back 200 with index.html — HTML that no JSON decode can accept. The URL was
// written for "bus-agent" in 2026-03 (commit ff975ce); dash owns the port now.
//
// The consequence is not a formatting bug. Every caller below treats an empty
// result as "no registry to validate against" and skips validation entirely, so
// this dead URL has silently disabled spawn-target validation ever since. Repointing
// it is NOT mechanical and is deliberately not done here: the live registry is
// agent-store (:8300/agents), whose rows carry `slug` and `display_name` but no
// `name` and no `orchestrator` at all, so RegistryAgent below cannot decode one and
// validOrchestrators has no source. Which field is the spawn identity — and whether
// a spawn should carry agent-store's id rather than any name — is a contract call.
// Noteboard `4d12d490` holds the measurement and the decision.
const registryAgentsURL = "http://localhost:8101/api/agents"

// fetchRegistryAgents queries the agent registry for spawn targets.
//
// Returns nil on every failure, which every caller reads as "no registry", and that
// fail-open direction is deliberately left alone — choosing between failing closed,
// staying open, and caching the last good answer is a trust decision that belongs to
// the owner (noteboard `4d12d490`), not to a refactor.
//
// What changed is that a failure is no longer SILENT. Three separate errors used to
// return nil with no log, no counter and no error value, which is why a URL that has
// answered with HTML since March never surfaced anywhere. Each class now names itself
// on stderr, so the next outage is visible at the point of failure rather than
// inferred later from a spawn that should have been rejected and was not.
func fetchRegistryAgents() []RegistryAgent {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(registryAgentsURL)
	if err != nil {
		reportRegistryFailure("unreachable", err)
		return nil
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		reportRegistryFailure("body read failed", err)
		return nil
	}
	agents, err := decodeRegistryAgents(resp.StatusCode, data)
	if err != nil {
		reportRegistryFailure("bad response", err)
		return nil
	}
	return agents
}

// reportRegistryFailure names a registry failure on stderr.
//
// The prefix is deliberately not "INBER_SPAWN:" — bus-agent parses that prefix off
// this same stream as a routing instruction, so a diagnostic wearing it would be read
// as a spawn request.
func reportRegistryFailure(kind string, err error) {
	fmt.Fprintf(os.Stderr, "inber: agent registry %s (%s): %v — spawn targets will NOT be validated\n",
		kind, registryAgentsURL, err)
}

// decodeRegistryAgents turns a registry response into agents, or says why it cannot.
//
// Split out of the fetch above so the failure classes can be tested without a live
// registry — and because the status check is the half that was missing entirely: the
// old code decoded the body of a 401 or a 500 exactly as it decoded a 200, so an
// authentication failure and an empty registry were indistinguishable.
func decodeRegistryAgents(statusCode int, body []byte) ([]RegistryAgent, error) {
	if statusCode < 200 || statusCode > 299 {
		return nil, fmt.Errorf("HTTP %d", statusCode)
	}
	var agents []RegistryAgent
	if err := json.Unmarshal(body, &agents); err != nil {
		// A 200 that is not JSON is the shape a SPA fallback returns, and saying so
		// costs one line and saves the next reader the walk this comment records.
		return nil, fmt.Errorf("HTTP %d body is not a JSON agent list (%w)", statusCode, err)
	}
	return agents, nil
}

// enabledAgentNames lists the agents a spawn request may actually name.
//
// Enabled-only, because that is precisely what the validator at the call site accepts.
// Both the tool's schema description and its rejection message are built from this one
// function so they cannot disagree — and they did, in opposite directions:
//
//   - The schema advertised EVERY agent, disabled ones included, so the model was told
//     a name was valid and then rejected for using it.
//   - The rejection message sized its slice to all agents and filled only the enabled
//     indices, leaving a zero value in every disabled agent's slot, so it rendered as
//     "Valid options: alpha, , gamma" — the empty slots being disabled agents.
//
// Appending only the names that survive the filter is what fixes both.
func enabledAgentNames(agents []RegistryAgent) []string {
	var names []string
	for _, a := range agents {
		if a.Enabled {
			names = append(names, a.Name)
		}
	}
	return names
}

// validAgentsDescription returns a description string with the list of valid agents.
func validAgentsDescription() string {
	names := enabledAgentNames(fetchRegistryAgents())
	// Reached both when the registry did not answer and when it answered with agents
	// of which none are enabled. Neither case has a name to offer, so neither may
	// print an empty list.
	if len(names) == 0 {
		return "Agent name to spawn. Must match a registered agent."
	}
	return fmt.Sprintf("Agent name to spawn. Valid options: %s", strings.Join(names, ", "))
}

// validOrchestrators returns the set of unique orchestrators from the registry.
func validOrchestrators() []string {
	agents := fetchRegistryAgents()
	seen := make(map[string]bool)
	var result []string
	for _, a := range agents {
		if !seen[a.Orchestrator] && a.Orchestrator != "" {
			seen[a.Orchestrator] = true
			result = append(result, a.Orchestrator)
		}
	}
	return result
}

// SpawnAgentTool creates a tool that delegates tasks to other agents.
// Purely declarative: emits INBER_SPAWN:{json} to stderr for bus-agent to route.
// Always async — returns immediately.
func (r *Registry) SpawnAgentTool() agent.Tool {
	type input struct {
		Agent        string `json:"agent"`
		Orchestrator string `json:"orchestrator,omitempty"`
		Task         string `json:"task"`
	}

	// Fetch valid agents/orchestrators at tool creation time
	agentDesc := validAgentsDescription()
	orchs := validOrchestrators()
	orchDesc := "Backend/orchestrator to use (e.g., 'inber', 'openclaw'). Optional — resolved from registry if omitted."
	if len(orchs) > 0 {
		orchDesc = fmt.Sprintf("Backend/orchestrator to use. Valid options: %s. Optional — resolved from registry if omitted.", strings.Join(orchs, ", "))
	}

	return agent.Tool{
		Name:        "spawn_agent",
		Description: "Delegate a task to another agent. Always async — returns immediately. The result will be delivered when the agent completes.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Required: []string{"agent", "task"},
			Properties: map[string]any{
				"agent": map[string]any{
					"type":        "string",
					"description": agentDesc,
				},
				"orchestrator": map[string]any{
					"type":        "string",
					"description": orchDesc,
				},
				"task": map[string]any{
					"type":        "string",
					"description": "Task description for the agent to complete",
				},
			},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			var in input
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}

			if in.Agent == "" {
				return "", fmt.Errorf("agent name required")
			}
			if in.Task == "" {
				return "", fmt.Errorf("task description required")
			}

			// Normalize to lowercase — registry is case-insensitive
			in.Agent = strings.ToLower(in.Agent)
			if in.Orchestrator != "" {
				in.Orchestrator = strings.ToLower(in.Orchestrator)
			}

			// Validate against registry if available
			agents := fetchRegistryAgents()
			if len(agents) > 0 {
				valid := false
				for _, a := range agents {
					if a.Name == in.Agent && a.Enabled {
						valid = true
						break
					}
				}
				if !valid {
					return "", fmt.Errorf("unknown agent %q. Valid options: %s",
						in.Agent, strings.Join(enabledAgentNames(agents), ", "))
				}
			}

			// Emit spawn request to stderr for bus-agent to pick up.
			spawn := map[string]string{
				"agent": in.Agent,
				"task":  in.Task,
			}
			if in.Orchestrator != "" {
				spawn["orchestrator"] = in.Orchestrator
			}
			spawnJSON, _ := json.Marshal(spawn)
			fmt.Fprintf(os.Stderr, "INBER_SPAWN:%s\n", spawnJSON)

			taskPreview := in.Task
			if len(taskPreview) > 100 {
				taskPreview = textutil.Truncate(taskPreview, 97) + "..."
			}

			target := in.Agent
			if in.Orchestrator != "" {
				target = in.Agent + "@" + in.Orchestrator
			}

			return fmt.Sprintf("🚀 Spawned %s\n\nTask: %s\n\nThe result will be delivered when complete.", target, taskPreview), nil
		},
	}
}
