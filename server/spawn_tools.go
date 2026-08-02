package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/internal/textutil"
)

// SteerAgentTool creates a tool for sending messages to running sub-agents.
func (g *Server) SteerAgentTool() agent.Tool {
	return agent.Tool{
		Name: "steer_agent",
		Description: "Send a message to a running sub-agent session. " +
			"If the agent is mid-turn, the message is injected immediately (seen between tool calls). " +
			"If idle, it's queued for the next turn.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Required: []string{"session_key", "message"},
			Properties: map[string]any{
				"session_key": map[string]any{
					"type":        "string",
					"description": "Session key of the target agent (from spawn response or sessions_list)",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Message to send to the agent",
				},
			},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			var in struct {
				SessionKey string `json:"session_key"`
				Message    string `json:"message"`
			}
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", err
			}

			// The route comes back from the delivery itself. Re-reading Status
			// afterwards to work it out, which is what this did, describes the
			// session as it is a moment later rather than the message as it
			// was routed — and it reported "injected, the agent will see it
			// between tool calls" for a message the pending queue had taken.
			route, err := g.Inject(in.SessionKey, in.Message)
			if err != nil {
				return "", err
			}

			if route == DeliveredMidTurn {
				return fmt.Sprintf("Message injected into %s mid-turn. The agent will see it between tool calls.", in.SessionKey), nil
			}
			return fmt.Sprintf("Message queued for %s. It will be delivered at the start of that session's next turn.", in.SessionKey), nil
		},
	}
}

// SpawnAgentTool creates the spawn_agent tool that calls server.Spawn directly.
// This replaces the old stderr-based INBER_SPAWN protocol.
func (g *Server) SpawnAgentTool(parentSessionKey string) agent.Tool {
	// Build available agents description. The sort is load-bearing, not tidiness:
	// this string is a tool description, and agent/agent_run.go anchors the
	// cache_control breakpoint on the last tool definition — so the whole tools
	// block, and every cached segment after it, is keyed on these bytes. Ranging
	// a Go map gives a fresh permutation on every call, so two sessions of the
	// same agent, a fork and its parent, and a session and its resume would each
	// hash a different prefix and share no cache entry.
	agentNames := sortedAgentNames(g.config.Agents)
	agentDesc := fmt.Sprintf("Agent name to spawn. Available: %s", fmt.Sprintf("%v", agentNames))

	return agent.Tool{
		Name: "spawn_agent",
		Description: "Spawn a sub-agent to work on a task. Returns immediately. " +
			"Results are delivered when the agent completes. " +
			"Use fork=true to give the child your current conversation context.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Required: []string{"agent", "task"},
			Properties: map[string]any{
				"agent": map[string]any{
					"type":        "string",
					"description": agentDesc,
				},
				"task": map[string]any{
					"type":        "string",
					"description": "Task for the agent to complete",
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Model override (optional)",
				},
				"fork": map[string]any{
					"type":        "boolean",
					"description": "If true, child inherits this session's conversation history",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Max runtime in seconds (default 300)",
				},
			},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			var in struct {
				Agent          string `json:"agent"`
				Task           string `json:"task"`
				Model          string `json:"model,omitempty"`
				Fork           bool   `json:"fork,omitempty"`
				TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
			}
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return "", err
			}
			if in.Agent == "" {
				return "", fmt.Errorf("agent name required")
			}
			if in.Task == "" {
				return "", fmt.Errorf("task required")
			}

			var timeout time.Duration
			if in.TimeoutSeconds > 0 {
				timeout = time.Duration(in.TimeoutSeconds) * time.Second
			}

			resp, err := g.Spawn(ctx, SpawnRequest{
				ParentKey: parentSessionKey,
				Agent:     in.Agent,
				Task:      in.Task,
				Model:     in.Model,
				Fork:      in.Fork,
				Timeout:   timeout,
			})
			if err != nil {
				return "", err
			}

			taskPreview := in.Task
			if len(taskPreview) > 100 {
				taskPreview = textutil.Truncate(taskPreview, 97) + "..."
			}

			return fmt.Sprintf("🚀 Spawned %s (%s)\nTask: %s\nFork: %v\n\nResult will be delivered when complete.",
				in.Agent, resp.ChildKey, taskPreview, in.Fork), nil
		},
	}
}