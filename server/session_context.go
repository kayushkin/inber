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
		g.sessionStatusInjector(sessionKey),
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