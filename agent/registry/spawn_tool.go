package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// SpawnAgentTool creates a tool that delegates tasks to other agents.
// Purely declarative: emits INBER_SPAWN:{json} to stderr for bus-agent to route.
// Always async — returns immediately.
func (r *Registry) SpawnAgentTool() agent.Tool {
	type input struct {
		Agent string `json:"agent"`
		Task  string `json:"task"`
	}

	return agent.Tool{
		Name:        "spawn_agent",
		Description: "Delegate a task to another agent. Always async — returns immediately. The result will be delivered when the agent completes.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Required: []string{"agent", "task"},
			Properties: map[string]any{
				"agent": map[string]any{
					"type":        "string",
					"description": "Agent name to spawn (must match a configured agent)",
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

			// Validate agent exists
			if _, err := r.GetConfig(in.Agent); err != nil {
				return "", fmt.Errorf("unknown agent %q", in.Agent)
			}

			// Emit spawn request to stderr for bus-agent to pick up
			spawn := map[string]string{
				"agent": in.Agent,
				"task":  in.Task,
			}
			spawnJSON, _ := json.Marshal(spawn)
			fmt.Fprintf(os.Stderr, "INBER_SPAWN:%s\n", spawnJSON)

			taskPreview := in.Task
			if len(taskPreview) > 100 {
				taskPreview = taskPreview[:97] + "..."
			}

			return fmt.Sprintf("🚀 Spawned %s\n\nTask: %s\n\nThe result will be delivered when complete.", in.Agent, taskPreview), nil
		},
	}
}
