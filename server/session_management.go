// Package server provides session management functionality for controlling
// active agent sessions, listing session status, and managing session lifecycle.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	sessionMod "github.com/kayushkin/inber/session"
)

// SessionInfo is a summary of a session for listing.
type SessionInfo struct {
	Key        string        `json:"key"`
	Agent      string        `json:"agent"`
	Status     SessionStatus `json:"status"`
	SpawnDepth int           `json:"spawn_depth"`
	ParentKey  string        `json:"parent_key,omitempty"`
	Children   []string      `json:"children,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	LastActive time.Time     `json:"last_active"`
	Messages   int           `json:"messages"`
}

// ---------------------------------------------------------------------------
// Session listing and management
// ---------------------------------------------------------------------------

// ListSessions returns info about all sessions.
func (g *Server) ListSessions() []*SessionInfo {
	var result []*SessionInfo
	g.sessions.Range(func(key, val any) bool {
		s := val.(*Session)
		s.mu.Lock()
		info := &SessionInfo{
			Key:        s.Key,
			Agent:      s.AgentName,
			Status:     s.Status,
			SpawnDepth: s.SpawnDepth,
			ParentKey:  s.ParentKey,
			Children:   s.Children,
			CreatedAt:  s.CreatedAt,
			LastActive: s.LastActive,
			Messages:   len(s.Engine.Messages),
		}
		s.mu.Unlock()
		result = append(result, info)
		return true
	})
	return result
}

// InterruptSession pauses a running session's current turn but keeps
// the session alive for future messages. Cascades to children.
func (g *Server) InterruptSession(key string) error {
	val, ok := g.sessions.Load(key)
	if !ok {
		return fmt.Errorf("session not found: %s", key)
	}
	s := val.(*Session)

	// Cascade to children first.
	s.mu.Lock()
	children := append([]string{}, s.Children...)
	s.mu.Unlock()

	for _, childKey := range children {
		g.InterruptSession(childKey)
	}

	s.interrupt()
	g.persistSessionState(s)
	return nil
}

// StopSession aborts a running session and cascades to children.
func (g *Server) StopSession(key string) error {
	val, ok := g.sessions.Load(key)
	if !ok {
		return fmt.Errorf("session not found: %s", key)
	}
	s := val.(*Session)

	// Cascade to children first.
	s.mu.Lock()
	children := append([]string{}, s.Children...)
	s.mu.Unlock()

	for _, childKey := range children {
		g.StopSession(childKey)
	}

	s.stop()
	return nil
}

// Inject sends a message into a session.
// If the session is running, injects mid-turn (agent sees it between tool calls).
// If idle, queues as pending (delivered as prefix on next turn).
func (g *Server) Inject(sessionKey, message string) error {
	val, ok := g.sessions.Load(sessionKey)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionKey)
	}
	s := val.(*Session)

	s.mu.Lock()
	isRunning := s.Status == Running
	s.mu.Unlock()

	if isRunning {
		s.inject(message)
	} else {
		s.queuePending(message)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Session persistence
// ---------------------------------------------------------------------------

// persistSessionState saves a session's messages and turn count to disk — both
// halves of what loadPersistedSession reads back on resume.
func (g *Server) persistSessionState(s *Session) {
	s.mu.Lock()
	msgs := s.Engine.Messages
	turnCounter := s.Engine.Turn.Counter
	s.mu.Unlock()

	dir := filepath.Join(g.config.DataDir, "sessions", s.Key)
	os.MkdirAll(dir, 0755)

	data, err := json.Marshal(msgs)
	if err != nil {
		log.Printf("[server] persist %s: %v", s.Key, err)
		return
	}
	os.WriteFile(filepath.Join(dir, "messages.json"), data, 0644)

	if err := sessionMod.SaveTurnCounter(dir, turnCounter); err != nil {
		log.Printf("[server] turn counter not persisted for %s, next resume will start from turn 0: %v", s.Key, err)
	}
}
