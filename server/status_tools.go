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

// AgentsStatusTool creates a tool for checking which agents are busy/idle.
func (g *Server) AgentsStatusTool() agent.Tool {
	return agent.Tool{
		Name:        "agents_status",
		Description: "Check the current status of all agents. Shows which agents are idle, working, or errored, and what task they're working on. Use before spawning to avoid queueing behind busy agents.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"agent_slug": map[string]any{
					"type":        "string",
					"description": "Optional: check a specific agent by slug",
				},
			},
		},
		Run: func(ctx context.Context, raw string) (string, error) {
			if g.agentStore == nil {
				return "Agent status tracking is not available (agent-store not connected).", nil
			}

			var in struct {
				AgentSlug string `json:"agent_slug"`
			}
			if raw != "" && raw != "{}" {
				json.Unmarshal([]byte(raw), &in)
			}

			if in.AgentSlug != "" {
				statuses, err := g.agentStore.GetStatus(in.AgentSlug)
				if err != nil {
					return "", fmt.Errorf("get status: %w", err)
				}
				if len(statuses) == 0 {
					return fmt.Sprintf("No status found for agent %q.", in.AgentSlug), nil
				}
				var sb strings.Builder
				for _, s := range statuses {
					sb.WriteString(formatAgentStatus(s.Emoji, s.DisplayName, s.AgentSlug, s.Status, s.Task, s.StartedAt))
					sb.WriteString("\n")
				}
				return sb.String(), nil
			}

			statuses, err := g.agentStore.ListStatuses()
			if err != nil {
				return "", fmt.Errorf("list statuses: %w", err)
			}
			if len(statuses) == 0 {
				return "No agent statuses tracked yet.", nil
			}

			var sb strings.Builder
			for _, s := range statuses {
				sb.WriteString(formatAgentStatus(s.Emoji, s.DisplayName, s.AgentSlug, s.Status, s.Task, s.StartedAt))
				sb.WriteString("\n")
			}
			return sb.String(), nil
		},
	}
}

func formatAgentStatus(emoji, displayName, slug, status string, task *string, startedAt *int64) string {
	label := slug
	if displayName != "" {
		label = fmt.Sprintf("%s (%s)", displayName, slug)
	}
	if emoji != "" {
		label = emoji + " " + label
	}

	switch status {
	case "working":
		taskStr := ""
		if task != nil && *task != "" {
			taskStr = fmt.Sprintf(": %q", *task)
		}
		since := ""
		if startedAt != nil && *startedAt > 0 {
			d := time.Since(time.Unix(*startedAt, 0)).Round(time.Second)
			since = fmt.Sprintf(" (since %s ago)", d)
		}
		return fmt.Sprintf("%s — working%s%s", label, taskStr, since)
	default:
		return fmt.Sprintf("%s — %s", label, status)
	}
}
