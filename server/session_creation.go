package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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
	if req.MaxDuration != 0 {
		cfg.MaxDuration = req.MaxDuration
	}
}

// createSession creates a new session with a fresh engine.
func (g *Server) createSession(ctx context.Context, key, agentName string, ac AgentConfig, req RunRequest, onEvent func(StreamEvent)) (*Session, error) {
	injections := make(chan string, 10)

	// Where this session works is resolved here, once, for every way a session
	// is born. Doing it at the call sites is what let a rebuild come back in
	// the wrong repository: two of them — resume and fork — reach for the
	// agent's stored config, which names this host's live checkout.
	repoRoot, workspaceRoots, err := g.workspaceRootsForSession(key, ac)
	if err != nil {
		return nil, err
	}

	cfg := engine.EngineConfig{
		AgentName:        agentName,
		RepoRoot:         repoRoot,
		WorkspaceRoots:   workspaceRoots,
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
	msgs, turnCounter, err := g.loadPersistedSession(key)
	if err != nil {
		return nil, err
	}
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

	lineage := g.lineageForSession(key)

	return &Session{
		Key:            key,
		AgentName:      agentName,
		Engine:         eng,
		Status:         Idle,
		SpawnDepth:     lineage.SpawnDepth,
		ParentKey:      lineage.ParentKey,
		WorkspaceRoots: workspaceRoots,
		CreatedAt:      time.Now(),
		LastActive:     time.Now(),
		injections:     injections,
	}, nil
}

// workspaceRootsForSession answers which repositories a session works in, and
// which of them relative paths resolve against.
//
// A spawn is the only moment a forge workspace is created, and at that moment
// the answer is on the config the spawn just overwrote — the store has no row
// for the child yet. Every later rebuild of the same session has the opposite
// problem: the config it is handed is the agent's stored one, naming this
// host's live checkout, and the workspace exists only in Server.workspaces,
// which does not survive a restart. So the config wins while it has an answer,
// and the record answers afterwards.
//
// A recorded workspace whose worktrees are gone from disk REFUSES the rebuild.
// Falling back to the agent's repository would be the original defect with an
// extra step: a session whose entire conversation is about a worktree, resumed
// into the live checkout, with every filesystem tool rooted there and nothing
// in the transcript to suggest it. A caller that gets an error can handle it;
// nobody can handle a turn that has already edited the wrong repository.
func (g *Server) workspaceRootsForSession(key string, ac AgentConfig) (string, []engine.WorkspaceRoot, error) {
	if len(ac.WorkspaceRoots) > 0 {
		return ac.Workspace, ac.WorkspaceRoots, nil
	}
	if g.store == nil {
		return ac.Workspace, ac.WorkspaceRoots, nil
	}
	recorded, err := g.store.SessionWorkspaceRoots(key)
	if err != nil {
		return "", nil, fmt.Errorf("session %s: %w", key, err)
	}
	if len(recorded) == 0 {
		return ac.Workspace, ac.WorkspaceRoots, nil
	}
	if err := workspaceRootsStillOnDisk(recorded); err != nil {
		return "", nil, fmt.Errorf("%w: session %s: %w", ErrWorkspaceGone, key, err)
	}
	return engine.PrimaryWorkspaceRoot(recorded), recorded, nil
}

// ErrWorkspaceGone reports that a session recorded a forge workspace that is no
// longer on disk, so the session cannot be rebuilt where it used to work.
//
// It is a sentinel because the HTTP layer has to tell this apart from a failure
// to build a session: nothing is broken here, the worktree was cleaned up, and
// the caller's request is the thing that cannot be satisfied. Answering 500
// would invite a retry that can never succeed.
var ErrWorkspaceGone = errors.New("workspace no longer exists")

// workspaceRootsStillOnDisk checks that every worktree a session was recorded
// in is still there, and still a directory.
//
// forge.Cleanup removes a workspace, and a merged or expired one is meant to be
// gone. This is the check that turns "its worktree was deleted" into a refusal
// at rebuild time rather than a session rooted at a path that no longer exists —
// which the engine would accept, since it validates that the roots agree with
// each other and not that they are real.
func workspaceRootsStillOnDisk(roots []engine.WorkspaceRoot) error {
	for _, root := range roots {
		info, err := os.Stat(root.Path)
		if err != nil {
			return fmt.Errorf("its worktree for %s is gone (%s): %w", root.Name, root.Path, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("its worktree for %s is not a directory (%s)", root.Name, root.Path)
		}
	}
	return nil
}

// lineageForSession answers where a session came from by asking the record that
// owns that fact.
//
// Session.SpawnDepth and Session.ParentKey are written by Spawn and
// forkSession and live in memory, so a rebuild used to come back as a root and
// three things read the lie. The cap in Spawn is checked against the parent's
// depth, so a revived depth-2 child could spawn MaxSpawnDepth more levels, and
// each of those could do the same after the next restart — the cap bounded a
// tree only for as long as the process stayed up. sessionStatusInjector tells
// the model its depth and parent only when the depth is above zero, so a
// revived child was told nothing about who asked for the work. And a child's
// results are delivered to its parent key, which came back empty.
//
// Nothing is inferred from the key here. A session with no lineage recorded is
// a root, which is what the zero value already says; reading it out of the key
// at run time would be the defect that handed a child's transcript to its
// parent's agent, arriving a second way.
func (g *Server) lineageForSession(key string) SessionLineage {
	if g.store == nil {
		return SessionLineage{}
	}
	lineage, err := g.store.SessionLineage(key)
	if err != nil {
		log.Printf("[server] lineage unreadable for %s, rebuilding it as a root: %v", key, err)
		return SessionLineage{}
	}
	return lineage
}

// restoreGuardState puts the safety limits recorded against a session key, and
// the totals counted against them, back onto the guard that a rebuild of that
// session has just constructed.
//
// Every limit this server has — max_turns, max_input_tokens, max_cost,
// max_duration — arrives
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
	log.Printf("[server] restored safety limits for %s (mode %q, max turns %d, max input tokens %d, max cost $%.2f, max duration %ds; already spent %d turns, %d input tokens, $%.4f, %ds)",
		key, restored.Mode, restored.MaxTurns, restored.MaxInputTokens, restored.MaxCost, restored.MaxDuration,
		restored.Turns, restored.InputTokens, restored.Cost, restored.ElapsedSeconds)
}

// loadPersistedSession loads a session's messages and the turn count recorded
// against them from the server data dir. They are loaded together because a
// turn count without its transcript describes messages that are not there.
//
// A session with nothing persisted yet is the ordinary case — every session is
// that on its first turn — and returns no messages and no error. A transcript
// that exists and cannot be read is not: the caller must not build a session
// over it, because persistSessionState overwrites messages.json at the end of
// every turn, so continuing here would replace a transcript we failed to read
// with one that starts empty. Report it instead.
func (g *Server) loadPersistedSession(key string) ([]anthropic.MessageParam, int, error) {
	dir := filepath.Join(g.config.DataDir, "sessions", key)
	path := filepath.Join(dir, "messages.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("read transcript for %s: %w", key, err)
	}
	var msgs []anthropic.MessageParam
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, 0, fmt.Errorf("parse transcript %s for %s: %w", path, key, err)
	}
	turnCounter, err := sessionMod.LoadTurnCounter(dir)
	if err != nil {
		log.Printf("[server] turn counter unreadable for %s, resuming as if from turn 0: %v", key, err)
	}
	return msgs, turnCounter, nil
}