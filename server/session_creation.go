package server

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/engine"
)

// getOrCreateSession retrieves an existing session or creates a new one.
func (g *Server) getOrCreateSession(key, agentName string, ac AgentConfig, onEvent func(StreamEvent)) (*Session, error) {
	if val, ok := g.sessions.Load(key); ok {
		sess := val.(*Session)
		sess.setOnEvent(onEvent)
		return sess, nil
	}

	sess, err := g.createSession(key, agentName, ac, onEvent)
	if err != nil {
		return nil, err
	}
	sess.setOnEvent(onEvent)

	// Store, but handle race (another goroutine may have created it).
	actual, loaded := g.sessions.LoadOrStore(key, sess)
	if loaded {
		// Someone else created it first. Close ours and use theirs.
		sess.close()
		return actual.(*Session), nil
	}
	return sess, nil
}

// createSession creates a new session with a fresh engine.
func (g *Server) createSession(key, agentName string, ac AgentConfig, onEvent func(StreamEvent)) (*Session, error) {
	injections := make(chan string, 10)

	cfg := engine.EngineConfig{
		AgentName:        agentName,
		RepoRoot:         ac.Workspace,
		Model:            ac.Model,
		Thinking:         ac.Thinking,
		CommandName:      "serve",
		Injections:       injections,
		ExtraTools:       g.toolsForAgent(key, agentName),
		ContextInjectors: g.contextInjectorsFor(key, agentName),
	}

	// Pass shared model store.
	if g.modelStore != nil {
		// Engine will use this instead of opening its own.
		cfg.ModelStore = g.modelStore
	}

	// Display hooks are set dynamically per-request via setOnEvent/updateHooks.
	// Don't set them at creation time — they'd become stale.

	// Try to load existing messages from persistence.
	msgs := g.loadPersistedMessages(key)
	if len(msgs) > 0 {
		// Repair interrupted sessions.
		msgs = conversation.RepairEmptyContent(msgs)
		msgs, _ = conversation.RepairDanglingToolUse(msgs)
		msgs = conversation.RepairAlternation(msgs)
		msgs = conversation.RepairThinkingSignatures(msgs)
		msgs = agent.SanitizeMessageToolIDs(msgs)
		log.Printf("[server] resumed session %s (%d messages)", key, len(msgs))
	}

	eng, err := engine.NewEngine(cfg)
	if err != nil {
		return nil, fmt.Errorf("create engine for %s: %w", agentName, err)
	}

	// If we loaded persisted messages, set them on the engine.
	if len(msgs) > 0 {
		eng.Messages = msgs
	}

	return &Session{
		Key:        key,
		AgentName:  agentName,
		Engine:     eng,
		Status:     Idle,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
		injections: injections,
	}, nil
}

// loadPersistedMessages loads messages from the server data dir.
func (g *Server) loadPersistedMessages(key string) []anthropic.MessageParam {
	path := filepath.Join(g.config.DataDir, "sessions", key, "messages.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var msgs []anthropic.MessageParam
	if err := json.Unmarshal(data, &msgs); err != nil {
		log.Printf("[server] failed to load messages for %s: %v", key, err)
		return nil
	}
	return msgs
}