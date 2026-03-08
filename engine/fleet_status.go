package engine

import (
	"fmt"
	"strings"
	"time"

	sessionMod "github.com/kayushkin/inber/session"
)

// buildFleetStatus generates a system prompt block showing all active agent sessions.
// Returns empty string if no other agents are running or DB is unavailable.
func (e *Engine) buildFleetStatus() string {
	if e.Session == nil {
		return ""
	}
	db := e.Session.DB()
	if db == nil {
		return ""
	}

	active, err := db.ListActiveStatus()
	if err != nil {
		Log.Warn("fleet status: %v", err)
		return ""
	}

	// Filter out our own session
	var others []sessionMod.ActiveAgentStatus
	for _, a := range active {
		if a.SessionID != e.Session.SessionID() {
			others = append(others, a)
		}
	}

	if len(others) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Active Agents\n")
	for _, a := range others {
		elapsed := formatDuration(a.Duration)
		lastAgo := formatDuration(time.Since(a.LastTurn))

		b.WriteString(fmt.Sprintf("- **%s** (%s) — %d turns, running %s, last turn %s ago",
			a.Agent, shortModel(a.Model), a.Turns, elapsed, lastAgo))
		if a.Task != "" {
			b.WriteString(fmt.Sprintf("\n  Task: %s", a.Task))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// shortModel strips the provider prefix from model names.
func shortModel(model string) string {
	if idx := strings.LastIndex(model, "/"); idx >= 0 {
		return model[idx+1:]
	}
	return model
}

// formatDuration formats a duration as a human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, mins)
}
