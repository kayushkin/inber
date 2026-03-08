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
)

// RegistryAgent represents an agent entry from the bus-agent registry.
type RegistryAgent struct {
	Name         string `json:"name"`
	Orchestrator string `json:"orchestrator"`
	Enabled      bool   `json:"enabled"`
}

// fetchRegistryAgents queries the bus-agent API for registered agents.
// Falls back to an empty list if the registry is unavailable.
func fetchRegistryAgents() []RegistryAgent {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:8101/api/agents")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var agents []RegistryAgent
	if err := json.Unmarshal(data, &agents); err != nil {
		return nil
	}
	return agents
}

// validAgentsDescription returns a description string with the list of valid agents.
func validAgentsDescription() string {
	agents := fetchRegistryAgents()
	if len(agents) == 0 {
		return "Agent name to spawn. Must match a registered agent."
	}
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.Name
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
					names := make([]string, len(agents))
					for i, a := range agents {
						if a.Enabled {
							names[i] = a.Name
						}
					}
					return "", fmt.Errorf("unknown agent %q. Valid options: %s", in.Agent, strings.Join(names, ", "))
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
				taskPreview = taskPreview[:97] + "..."
			}

			target := in.Agent
			if in.Orchestrator != "" {
				target = in.Agent + "@" + in.Orchestrator
			}

			return fmt.Sprintf("🚀 Spawned %s\n\nTask: %s\n\nThe result will be delivered when complete.", target, taskPreview), nil
		},
	}
}
