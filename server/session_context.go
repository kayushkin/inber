package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/kayushkin/inber/engine"
	sessionMod "github.com/kayushkin/inber/session"
)

// contextInjectorsFor returns context injectors for a given agent.
// Only the orchestrator (default agent) gets the live session status.
func (g *Server) contextInjectorsFor(sessionKey, agentName string) []engine.ContextInjector {
	if agentName != g.config.DefaultAgent {
		return nil // only orchestrator sees session status
	}
	return []engine.ContextInjector{
		g.agentFleetInjector(),
		g.sessionStatusInjector(sessionKey),
	}
}

// agentFleetInjector returns an injector that provides the full agent fleet with statuses.
// Injected into the system prompt so the orchestrator always knows its agents.
func (g *Server) agentFleetInjector() engine.ContextInjector {
	return func() []sessionMod.NamedBlock {
		if g.agentStore == nil {
			return nil
		}

		statuses, err := g.agentStore.ListStatuses()
		if err != nil {
			return nil
		}

		// Build a map of agent slug → status info
		type statusInfo struct {
			Status string
			Task   string
			Since  int64
		}
		statusMap := make(map[string]statusInfo)
		for _, s := range statuses {
			si := statusInfo{Status: s.Status}
			if s.Task != nil {
				si.Task = *s.Task
			}
			if s.StartedAt != nil {
				si.Since = *s.StartedAt
			}
			statusMap[s.AgentSlug] = si
		}

		// Get all agent configs from server config
		configs := g.config.Agents
		if len(configs) == 0 {
			return nil
		}

		var b strings.Builder
		b.WriteString("# Agent Fleet\n\n")
		b.WriteString("Available agents for spawn_agent:\n\n")

		for slug, ac := range configs {
			status := "idle"
			var taskStr string
			if si, ok := statusMap[slug]; ok && si.Status != "" {
				status = si.Status
				if si.Task != "" {
					taskStr = fmt.Sprintf(": %q", truncate(si.Task, 80))
				}
				if si.Status == "working" && si.Since > 0 {
					ago := time.Since(time.Unix(si.Since, 0))
					taskStr += fmt.Sprintf(" (%s)", formatDuration(ago))
				}
			}
			project := ac.Project
			if project == "" && len(ac.Projects) > 0 {
				project = strings.Join(ac.Projects, ", ")
			}
			b.WriteString(fmt.Sprintf("- **%s** (%s) — %s [%s%s]",
				ac.Name, slug, project, status, taskStr))
			b.WriteString("\n")
		}

		return []sessionMod.NamedBlock{
			{ID: "agent-fleet", Text: b.String()},
		}
	}
}

// sessionStatusInjector returns an injector that provides live server session info.
func (g *Server) sessionStatusInjector(ownSessionKey string) engine.ContextInjector {
	return func() []sessionMod.NamedBlock {
		sessions := g.ListSessions()
		if len(sessions) == 0 {
			return nil
		}

		var b strings.Builder
		b.WriteString("# Server Sessions\n\n")

		var hasContent bool
		for _, s := range sessions {
			if s.Key == ownSessionKey {
				continue // skip own session
			}
			hasContent = true
			status := s.Status.String()
			ago := time.Since(s.LastActive)
			agoStr := formatDuration(ago)

			b.WriteString(fmt.Sprintf("- **%s** [%s] — %s, %d msgs, last active %s ago",
				s.Agent, status, s.Key, s.Messages, agoStr))
			if s.SpawnDepth > 0 {
				b.WriteString(fmt.Sprintf(" (spawn depth %d, parent: %s)", s.SpawnDepth, s.ParentKey))
			}
			if len(s.Children) > 0 {
				b.WriteString(fmt.Sprintf(" → children: %s", strings.Join(s.Children, ", ")))
			}
			b.WriteString("\n")
		}

		if !hasContent {
			return nil
		}

		return []sessionMod.NamedBlock{
			{ID: "server-sessions", Text: b.String()},
		}
	}
}