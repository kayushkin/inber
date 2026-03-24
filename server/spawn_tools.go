package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// SessionsListTool creates a tool for checking sub-agent status.
func (g *Server) SessionsListTool(parentSessionKey string) agent.Tool {
	return agent.Tool{
		Name:        "sessions_list",
		Description: "List active sessions and their status, enriched with agent-store status. Shows all registered agents — those with sessions and those without.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			sessions := g.ListSessions()

			// Build agent-store status map.
			statusMap := map[string]string{}
			if g.agentStore != nil {
				if statuses, err := g.agentStore.ListStatuses(); err == nil {
					for _, s := range statuses {
						statusMap[s.AgentSlug] = s.Status
						if s.Status == "working" && s.Task != nil {
							statusMap[s.AgentSlug] = fmt.Sprintf("working: %q", *s.Task)
						}
					}
				}
			}

			// Track which agents have sessions.
			agentsWithSessions := map[string]bool{}

			var sb strings.Builder
			if len(sessions) > 0 {
				sb.WriteString("## Active Sessions\n")
				for _, s := range sessions {
					agentsWithSessions[s.Agent] = true
					marker := "  "
					if s.Key == parentSessionKey {
						marker = "→ "
					}
					sb.WriteString(fmt.Sprintf("%s%s (%s) [%s] %d msgs, active %s ago",
						marker, s.Key, s.Agent, s.Status,
						s.Messages, time.Since(s.LastActive).Round(time.Second)))
					if as, ok := statusMap[s.Agent]; ok {
						sb.WriteString(fmt.Sprintf(" agent-status=%s", as))
					}
					if s.ParentKey != "" {
						sb.WriteString(fmt.Sprintf(" parent=%s", s.ParentKey))
					}
					if len(s.Children) > 0 {
						sb.WriteString(fmt.Sprintf(" children=%v", s.Children))
					}
					sb.WriteString("\n")
				}
			}

			// Show registered agents without active sessions.
			var inactive []string
			for slug := range g.config.Agents {
				if !agentsWithSessions[slug] {
					status := "idle"
					if as, ok := statusMap[slug]; ok {
						status = as
					}
					inactive = append(inactive, fmt.Sprintf("  %s — %s (no session)", slug, status))
				}
			}
			if len(inactive) > 0 {
				if len(sessions) > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString("## Other Registered Agents\n")
				for _, line := range inactive {
					sb.WriteString(line)
					sb.WriteString("\n")
				}
			}

			if sb.Len() == 0 {
				return "No active sessions and no registered agents.", nil
			}
			return sb.String(), nil
		},
	}
}

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

			err := g.Inject(in.SessionKey, in.Message)
			if err != nil {
				return "", err
			}

			// Check if it was injected live or queued.
			val, ok := g.sessions.Load(in.SessionKey)
			if !ok {
				return "Message sent (session not found in memory, may have been queued to DB).", nil
			}
			sess := val.(*Session)
			sess.mu.Lock()
			status := sess.Status
			sess.mu.Unlock()

			if status == Running {
				return fmt.Sprintf("Message injected into %s (currently running). Agent will see it between tool calls.", in.SessionKey), nil
			}
			return fmt.Sprintf("Message queued for %s (currently %s). Will be delivered on next turn.", in.SessionKey, status), nil
		},
	}
}

// SpawnAgentTool creates the spawn_agent tool that calls server.Spawn directly.
// This replaces the old stderr-based INBER_SPAWN protocol.
func (g *Server) SpawnAgentTool(parentSessionKey string) agent.Tool {
	// Build available agents description.
	agentNames := make([]string, 0, len(g.config.Agents))
	for name := range g.config.Agents {
		agentNames = append(agentNames, name)
	}
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
				taskPreview = taskPreview[:97] + "..."
			}

			return fmt.Sprintf("🚀 Spawned %s (%s)\nTask: %s\nFork: %v\n\nResult will be delivered when complete.",
				in.Agent, resp.ChildKey, taskPreview, in.Fork), nil
		},
	}
}