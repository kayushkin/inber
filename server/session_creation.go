package server

import (
	"context"
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
	"github.com/kayushkin/inber/guard"
	sessionMod "github.com/kayushkin/inber/session"
)

// getOrCreateSession retrieves an existing session or creates a new one.
func (g *Server) getOrCreateSession(ctx context.Context, key, agentName string, ac AgentConfig, req RunRequest, onEvent func(StreamEvent)) (*Session, error) {
	if val, ok := g.sessions.Load(key); ok {
		sess := val.(*Session)
		sess.setOnEvent(onEvent)
		return sess, nil
	}

	sess, err := g.createSession(ctx, key, agentName, ac, req, onEvent)
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

// applyRequestOverrides copies the per-request overrides a caller sent with a
// RunRequest onto the engine config the session will be built from.
//
// Every field the API advertises has to be copied here, and two were not.
// max_cost was declared on RunRequest, documented as a safety limit, and never
// read by any code in this package, so a caller who asked for a spending cap
// got a session with no cap at all and no error saying so. mode was the same
// omission with a sharper edge: it is documented as "observe, assist,
// autonomous", and a caller who asked to observe got a fully autonomous session
// that could run shell commands. This lives in its own function, apart from
// engine construction, so that the copying can be tested field by field.
func applyRequestOverrides(cfg *engine.EngineConfig, req RunRequest) {
	if req.Model != "" {
		cfg.Model = req.Model
		cfg.ModelExplicitlySet = true
	}
	if req.Thinking != 0 {
		cfg.Thinking = req.Thinking
	}
	if req.Raw {
		cfg.Raw = true
	}
	if req.NoTools {
		cfg.NoTools = true
	}
	if req.NoHooks {
		cfg.NoHooks = true
	}
	if req.System != "" {
		cfg.SystemOverride = req.System
	}
	if req.Detach {
		cfg.Detach = true
	}
	if req.Mode != "" {
		cfg.Mode = req.Mode
	}
	if req.MaxTurns != 0 {
		cfg.MaxTurns = req.MaxTurns
	}
	if req.MaxInputTokens != 0 {
		cfg.MaxInputTokens = req.MaxInputTokens
	}
	if req.MaxCost != 0 {
		cfg.MaxCost = req.MaxCost
	}
}

// createSession creates a new session with a fresh engine.
func (g *Server) createSession(ctx context.Context, key, agentName string, ac AgentConfig, req RunRequest, onEvent func(StreamEvent)) (*Session, error) {
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

	applyRequestOverrides(&cfg, req)

	// Pass shared model store.
	if g.modelStore != nil {
		// Engine will use this instead of opening its own.
		cfg.ModelStore = g.modelStore
	}

	// Display hooks are set dynamically per-request via setOnEvent/updateHooks.
	// Don't set them at creation time — they'd become stale.

	// Try to load existing messages and their turn count from persistence.
	msgs, turnCounter := g.loadPersistedSession(key)
	if len(msgs) > 0 {
		// Repair interrupted sessions.
		msgs = conversation.RepairEmptyContent(msgs)
		msgs, _ = conversation.RepairDanglingToolUse(msgs)
		msgs = conversation.RepairAlternation(msgs)
		msgs = conversation.RepairThinkingSignatures(msgs)
		msgs = agent.SanitizeMessageToolIDs(msgs)
		log.Printf("[server] resumed session %s (%d messages, turn %d)", key, len(msgs), turnCounter)
	}

	// Building the session reads the workspace, so it follows the same rule as
	// a turn: the caller's deadline binds it, the caller's cancellation does
	// not. The session outlives the request that asked for it.
	setupCtx, endSetup := withoutCallerCancellation(ctx)
	defer endSetup()

	eng, err := engine.NewEngine(setupCtx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create engine for %s: %w", agentName, err)
	}

	// If we loaded persisted messages, restore them onto the engine. Restoring
	// (rather than assigning) freezes them, so the first resumed turn does not
	// re-prune and re-pay for the whole transcript, and carries the turn count
	// over so a long-running session is not budgeted as a brand new one.
	if len(msgs) > 0 {
		eng.RestoreSession(msgs, turnCounter)
	}

	g.restoreGuardState(key, eng.Guard)

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

// restoreGuardState puts the safety limits recorded against a session key, and
// the totals counted against them, back onto the guard that a rebuild of that
// session has just constructed.
//
// Every limit this server has — max_turns, max_input_tokens, max_cost — arrives
// on the RunRequest that starts a session and is copied onto the engine by
// applyRequestOverrides. A session is not always started by such a request:
// handleBridgeResume rebuilds one from its persisted messages when a session
// is asked for and is no longer in memory, and it has no request to pass, so it
// passes a zero one. Every cap the session had was a field of that request, so
// before this the rebuilt session had none at all — and a zero cap reads as
// unlimited, so a session capped at $5 came back with no cap, no log line and
// no error.
//
// The totals matter as much as the caps, and are restored in the same call:
// putting a $5 cap back on a guard that has forgotten the $4.80 already spent
// under it hands the budget back whole, and a session rebuilt often enough
// would never reach any cap at all.
func (g *Server) restoreGuardState(key string, sessionGuard *guard.Guard) {
	if sessionGuard == nil {
		return
	}
	dir := filepath.Join(g.config.DataDir, "sessions", key)
	recorded, err := sessionMod.LoadGuardState(dir)
	if err != nil {
		log.Printf("[server] safety limits unreadable for %s, running under whatever this rebuild configured: %v", key, err)
		return
	}
	// Nothing was recorded — a session being created rather than rebuilt, or one
	// last persisted before this sidecar existed. ResumeState would return the
	// configured state unchanged, so say nothing rather than log a restore that
	// restored nothing.
	if recorded == (guard.State{}) {
		return
	}

	if err := sessionGuard.RestoreState(guard.ResumeState(recorded, sessionGuard.State())); err != nil {
		log.Printf("[server] %s: %v", key, err)
	}

	restored := sessionGuard.State()
	log.Printf("[server] restored safety limits for %s (mode %q, max turns %d, max input tokens %d, max cost $%.2f; already spent %d turns, %d input tokens, $%.4f)",
		key, restored.Mode, restored.MaxTurns, restored.MaxInputTokens, restored.MaxCost,
		restored.Turns, restored.InputTokens, restored.Cost)
}

// loadPersistedSession loads a session's messages and the turn count recorded
// against them from the server data dir. They are loaded together because a
// turn count without its transcript describes messages that are not there.
func (g *Server) loadPersistedSession(key string) ([]anthropic.MessageParam, int) {
	dir := filepath.Join(g.config.DataDir, "sessions", key)
	data, err := os.ReadFile(filepath.Join(dir, "messages.json"))
	if err != nil {
		return nil, 0
	}
	var msgs []anthropic.MessageParam
	if err := json.Unmarshal(data, &msgs); err != nil {
		log.Printf("[server] failed to load messages for %s: %v", key, err)
		return nil, 0
	}
	turnCounter, err := sessionMod.LoadTurnCounter(dir)
	if err != nil {
		log.Printf("[server] turn counter unreadable for %s, resuming as if from turn 0: %v", key, err)
	}
	return msgs, turnCounter
}