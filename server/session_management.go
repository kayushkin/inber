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

	"github.com/kayushkin/inber/guard"
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

// Inject sends a message into a session and reports which route delivered it:
// mid-turn, where the agent sees it between tool calls, or the pending queue,
// where it is prepended to the input of the session's next turn.
//
// The route is returned rather than left for the caller to infer, because the
// two callers here both describe the delivery to somebody — one to a model, one
// over HTTP — and both used to describe it by re-reading Status afterwards,
// which is a second race on top of the one inside the delivery.
func (g *Server) Inject(sessionKey, message string) (DeliveryRoute, error) {
	val, ok := g.sessions.Load(sessionKey)
	if !ok {
		return "", fmt.Errorf("session not found: %s", sessionKey)
	}
	s := val.(*Session)
	return s.deliver(message), nil
}

// ---------------------------------------------------------------------------
// Session persistence
// ---------------------------------------------------------------------------

// persistSessionState saves a session's messages, turn count and safety limits
// to disk — everything createSession reads back when it rebuilds the session
// under that key.
//
// Taking s.mu here does NOT make the read safe against a turn: Session.turn
// releases s.mu before calling Engine.RunTurn and holds nothing for the length
// of the turn, so the engine's conversation is written by a goroutine this lock
// does not reach. Callers that must not race a turn have to establish that
// themselves and use persistSessionStateLocked. See the note there.
func (g *Server) persistSessionState(s *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g.persistSessionStateLocked(s)
}

// persistSessionStateLocked is persistSessionState for a caller that already
// holds s.mu and needs to keep holding it across the write.
//
// The reason that matters is not the lock's protection of the engine — it has
// none — but what holding it excludes: Session.turn takes s.mu to move the
// session to Running before it starts. So a caller that has checked, under this
// same hold, that no turn is running knows that none can begin before it lets
// go, and its read of the conversation is the only one there is. That is the
// only way a caller in this package gets a race-free read of Engine.Messages
// today, and it is why handleBridgeCompact does its whole job inside one hold
// rather than compacting and then persisting.
//
// s.mu must be held.
func (g *Server) persistSessionStateLocked(s *Session) {
	msgs := s.Engine.Messages
	turnCounter := s.Engine.Turn.Counter
	var guardState guard.State
	haveGuardState := s.Engine.Guard != nil
	if haveGuardState {
		guardState = s.Engine.Guard.State()
	}

	dir := filepath.Join(g.config.DataDir, "sessions", s.Key)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[server] session dir not created for %s, none of the three writes below can land: %v", s.Key, err)
	}

	data, err := json.Marshal(msgs)
	if err != nil {
		log.Printf("[server] persist %s: %v", s.Key, err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "messages.json"), data, 0644); err != nil {
		// The turn counter and guard state below are written regardless, so
		// after this line the three records disagree: the counter and the
		// spend describe messages the transcript does not hold. Whether they
		// should be skipped, or written first so a torn persist over-counts
		// rather than under-counts, is an open question — noteboard card
		// d347a2fe. Saying so is the floor; the ordering is not settled here.
		log.Printf("[server] transcript not persisted for %s, the turn counter and safety limits below still advance and will describe messages that are not there: %v", s.Key, err)
	}

	if err := sessionMod.SaveTurnCounter(dir, turnCounter); err != nil {
		log.Printf("[server] turn counter not persisted for %s, next resume will start from turn 0: %v", s.Key, err)
	}

	if haveGuardState {
		if err := sessionMod.SaveGuardState(dir, guardState); err != nil {
			log.Printf("[server] safety limits not persisted for %s, next resume will rebuild it uncapped and unspent: %v", s.Key, err)
		}
	}
}
