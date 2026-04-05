package server

import (
	"net/http"
	"time"
)

// handleHealth returns server health status.
func (g *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	g.mu.RLock()
	lastErr := g.lastError
	lastErrAt := g.lastErrorAt
	g.mu.RUnlock()

	// Count active sessions.
	activeSessions := 0
	g.sessions.Range(func(_, _ any) bool {
		activeSessions++
		return true
	})

	// Count loaded agents.
	agentsLoaded := len(g.config.Agents)

	// Determine NATS status.
	natsConnected := g.bus != nil

	// Determine overall status.
	status := "ok"
	if !lastErrAt.IsZero() && time.Since(lastErrAt) < 5*time.Minute {
		status = "degraded"
	}
	if !natsConnected {
		status = "error"
	}

	resp := map[string]any{
		"status":          status,
		"uptime_seconds":  int(time.Since(g.startedAt).Seconds()),
		"started_at":      g.startedAt.UTC().Format(time.RFC3339),
		"nats_connected":  natsConnected,
		"active_sessions": activeSessions,
		"agents_loaded":   agentsLoaded,
	}

	if lastErr != "" {
		resp["last_error"] = lastErr
		resp["last_error_at"] = lastErrAt.UTC().Format(time.RFC3339)
	}

	httpCode := http.StatusOK
	if status == "error" {
		httpCode = http.StatusServiceUnavailable
	}

	w.WriteHeader(httpCode)
	jsonResponse(w, resp)
}
