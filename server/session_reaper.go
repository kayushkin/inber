package server

// session_reaper.go — periodic cleanup of idle bridge sessions.
//
// Bridge sessions (keys starting with "bridge-") are created per request by
// the llm-bridge-compatible API and are never explicitly closed by the client.
// Without cleanup they accumulate indefinitely. The reaper runs every
// reaperInterval and removes sessions that haven't been active for longer than
// BridgeSessionTTL (default 1 hour).
//
// Only bridge- prefixed sessions are touched; main and named sessions are safe.

import (
	"context"
	"log"
	"strings"
	"time"
)

const (
	reaperInterval = 10 * time.Minute
)

// startBridgeSessionReaper launches the background reaper goroutine.
// It returns immediately; the goroutine shuts down when ctx is cancelled.
func (g *Server) startBridgeSessionReaper(ctx context.Context) {
	ttl := g.config.BridgeSessionTTL
	if ttl == 0 {
		ttl = defaultBridgeSessionTTL
	}

	go func() {
		ticker := time.NewTicker(reaperInterval)
		defer ticker.Stop()

		// Run once immediately so stale sessions from a previous server run
		// are cleaned up without waiting 10 minutes.
		g.reapBridgeSessions(ttl)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				g.reapBridgeSessions(ttl)
			}
		}
	}()
}

// reapBridgeSessions removes bridge sessions that have been idle longer than ttl.
func (g *Server) reapBridgeSessions(ttl time.Duration) {
	cutoff := time.Now().Add(-ttl)
	var reaped []string

	g.sessions.Range(func(key, val any) bool {
		sessionKey := key.(string)
		if !strings.HasPrefix(sessionKey, "bridge-") {
			return true
		}

		s := val.(*Session)
		s.mu.Lock()
		lastActive := s.LastActive
		isRunning := s.Status == Running
		s.mu.Unlock()

		if isRunning {
			// Never evict an active session.
			return true
		}
		if lastActive.Before(cutoff) {
			reaped = append(reaped, sessionKey)
		}
		return true
	})

	for _, sessionKey := range reaped {
		val, ok := g.sessions.LoadAndDelete(sessionKey)
		if !ok {
			continue // already gone
		}
		s := val.(*Session)
		s.close()

		if err := g.store.DeleteSession(sessionKey); err != nil {
			log.Printf("[reaper] failed to delete bridge session %s from DB: %v", sessionKey, err)
		}
	}

	if len(reaped) > 0 {
		log.Printf("[reaper] reaped %d idle bridge session(s) (ttl=%s)", len(reaped), ttl)
	}
}
