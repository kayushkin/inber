package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/engine"
)

// forkSession creates a child session with a deep copy of the parent's messages.
func (g *Server) forkSession(ctx context.Context, parent *Session, childKey, agentName string, ac AgentConfig, onEvent func(StreamEvent)) (*Session, error) {
	// Deep copy parent's messages, and read its turn count under the same lock
	// so the pair describes one moment in the parent's life.
	parent.mu.Lock()
	msgData, err := json.Marshal(parent.Engine.Messages)
	parentTurnCounter := parent.Engine.Turn.Counter
	parent.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("copy messages: %w", err)
	}

	var parentMessages []anthropic.MessageParam
	if err := json.Unmarshal(msgData, &parentMessages); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}

	// A fork inherits its parent's workspace along with its parent's messages,
	// and for the same reason: the conversation being copied is about the
	// worktree the parent works in. Both callers hand over the agent's *stored*
	// config, which names this host's live checkout — so without this a fork of
	// a spawned session came back rooted at ~/repos/<repo> holding a transcript
	// entirely about ~/forge/work/<id>/<repo>, immediately, with no restart
	// involved. It is set here rather than at the call sites because there are
	// two of them and neither has any reason to think about it.
	if len(parent.WorkspaceRoots) > 0 {
		ac.WorkspaceRoots = parent.WorkspaceRoots
		ac.Workspace = engine.PrimaryWorkspaceRoot(parent.WorkspaceRoots)
	}

	child, err := g.createSession(ctx, childKey, agentName, ac, RunRequest{}, onEvent)
	if err != nil {
		return nil, err
	}

	// Replace the empty messages with parent's history. Restoring freezes that
	// history, so the child's BP3 breakpoint lands on the same boundary the
	// parent already cached instead of re-staging the inherited transcript. The
	// child inherits the parent's turn count for the same reason it inherits the
	// frozen boundary: its first turn is not a first turn, it opens on a
	// conversation that is already however many turns deep.
	child.Engine.RestoreSession(parentMessages, parentTurnCounter)
	child.SpawnDepth = parent.SpawnDepth + 1
	child.ParentKey = parent.Key

	// Inject sub-agent context.
	taskContext := fmt.Sprintf("[System] You are a forked sub-agent. "+
		"You inherited your parent's conversation context. "+
		"Complete your assigned task and respond with your results. "+
		"Do not repeat context you already have.")
	child.Engine.Messages = append(child.Engine.Messages,
		anthropic.NewUserMessage(anthropic.NewTextBlock(taskContext)))

	return child, nil
}

// recordChildSession writes a child's row: where it came from, and where it
// works.
//
// It takes the child rather than a lineage so that neither caller can record a
// child while leaving out either fact: the parent key, the spawn depth and the
// workspace roots are all read off the session that has just been built, which
// is the only place they are set. Spawn and handleBridgeFork both go through
// here.
func (g *Server) recordChildSession(child *Session, agentName, kind string) {
	g.store.UpsertSession(child.Key, agentName, kind,
		SessionLineage{ParentKey: child.ParentKey, SpawnDepth: child.SpawnDepth},
		child.WorkspaceRoots)
}

// childKeySeparator joins a parent's session key to the suffix that makes a
// child's. One per level of the tree, so counting them counts spawn depth —
// which is how backfillSessionLineageFromChildKeys repairs the children that
// were recorded before there was a column to record their depth in.
const childKeySeparator = ":sub:"

// sessionKeyForChild proposes a child session key. It is a proposal and not an
// answer: the suffix is a clock remainder over a space of 100,000, so two
// children of one parent can be handed the same key. Everything that mints a
// key goes through mintChildSessionKey, which is what checks.
func sessionKeyForChild(parentKey string) string {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	return parentKey + childKeySeparator + suffix
}

// sessionKeyMintAttempts bounds how many keys are proposed before minting is
// refused. Each attempt reads the clock afresh, and every proposal is also
// handed its attempt number so a proposal whose only variation is a coarse
// clock can disambiguate itself rather than offering the same taken key a
// hundred times. The bound is there for the case the loop cannot solve — a
// suffix space so full that drawing from it keeps landing on a taken key —
// where the honest answer is an error, not a loop that never returns.
const sessionKeyMintAttempts = 100

// mintChildSessionKey returns a child session key that nothing else is using.
//
// The key is this server's identity for a session, and it is used as one: it is
// the directory a transcript and a guard sidecar are stored under, the row a
// child's agent and lineage are recorded against, and the entry a live session
// occupies in g.sessions. Because the suffix was a clock remainder taken with no
// check, a second child could be handed a first child's key and inherit all
// three, silently:
//
//   - restoreGuardState reads <DataDir>/sessions/<key> and puts the recorded
//     caps AND the totals spent against them onto the new session's guard, so a
//     fresh child starts having already spent a sibling's turns, tokens and
//     dollars — and on the fork path RestoreSession overwrites the inherited
//     messages a moment later, so the transcript half is masked and the budget
//     half is not.
//   - UpsertSession leaves agent, lineage and workspace alone on conflict, so
//     the new child's own agent name is dropped and agentForSession later
//     rebuilds it as the sibling's agent. That is exactly the defect
//     session_agent_resolution_test.go was written to close, arriving by a
//     different door: on the live store the 27 children of agent:claxon:main are
//     brigid's, fionn's and manannan's, so a collision there crosses agents.
//   - g.sessions.Store would replace a still-running sibling in the map, so the
//     sibling's results are delivered against a session nobody can look up.
//
// A caller holds the returned key until the session it built is in g.sessions,
// then releases it with releaseSessionKeyReservation — the reservation is what
// keeps two concurrent spawns from passing the same check before either has
// stored anything.
func (g *Server) mintChildSessionKey(parentKey string) (string, error) {
	return g.mintChildSessionKeyFrom(parentKey, sessionKeyForChild)
}

// mintChildSessionKeyFrom is mintChildSessionKey with the proposal separated
// from the checking, so a test can script the collision the clock only produces
// once in tens of thousands of spawns.
func (g *Server) mintChildSessionKeyFrom(parentKey string, propose func(parentKey string) string) (string, error) {
	return g.mintUnusedSessionKey("a child of "+parentKey, func(int) string {
		return propose(parentKey)
	})
}

// detachedSessionKey proposes the key for a one-off `detach` run. It is a
// proposal and not an answer, for the same reason sessionKeyForChild is: the
// suffix is a clock reading, so two detached runs of one agent inside the same
// millisecond are handed the same key. Everything that mints a key goes
// through mintUnusedSessionKey, which is what checks.
//
// The first attempt is the bare millisecond, which is the key shape this has
// always produced and the shape anyone reading a session list recognises. Only
// a retry — i.e. only a genuine collision — adds the attempt number, so the
// disambiguator appears in the rare case that needs it and nowhere else.
//
// A detached key deliberately does NOT use childKeySeparator. Counting those
// separators is how backfillSessionLineageFromChildKeys derives spawn depth, so
// a one-off run wearing one would be repaired into a child of an agent it has
// no parent in.
func detachedSessionKey(agentName string, attempt int) string {
	key := fmt.Sprintf("agent:%s:detach-%d", agentName, time.Now().UnixMilli())
	if attempt > 0 {
		key = fmt.Sprintf("%s-%d", key, attempt)
	}
	return key
}

// mintDetachedSessionKey returns a key for a one-off run that nothing else is
// using.
//
// `detach` mints a key of its own precisely so the run cannot touch the agent's
// main session — and it used to mint it from a bare millisecond with no check,
// which is the one way that promise breaks. Two detached runs inside one
// millisecond, or a detached run landing on a key some earlier run left on
// disk, inherit each other exactly as two children did before there was a
// checked mint: restoreGuardState puts the recorded caps AND the spend against
// them onto the new session's guard, UpsertSession keeps the first run's agent
// and lineage on conflict, and g.sessions.Store replaces a still-running
// sibling in the map. See mintChildSessionKey for the measured version of each.
func (g *Server) mintDetachedSessionKey(agentName string) (string, error) {
	return g.mintDetachedSessionKeyFrom(agentName, detachedSessionKey)
}

// mintDetachedSessionKeyFrom is mintDetachedSessionKey with the proposal
// separated from the checking, so a test can hold the millisecond still. The
// collision this guards against needs two runs inside one clock tick, which is
// not something a test can wait for.
func (g *Server) mintDetachedSessionKeyFrom(agentName string, propose func(agentName string, attempt int) string) (string, error) {
	return g.mintUnusedSessionKey("a detached run of "+agentName, func(attempt int) string {
		return propose(agentName, attempt)
	})
}

// resolveSessionKey answers which session key a run belongs to, and returns
// the release for any reservation it had to take. The release is always
// callable, so a caller defers it once rather than branching on which arm ran.
//
// This lives out of run() because the detach arm is the only one that mints,
// and a mint that is not reached is a mint that is not protecting anything:
// keeping the choice in one named function is what lets a test drive it
// without a live agent behind it.
func (g *Server) resolveSessionKey(agentName string, req RunRequest) (string, func(), error) {
	noReservation := func() {}

	if req.Detach {
		// One-off run: a key of its own, so it cannot touch the agent's main
		// session. That promise is only kept if the key is genuinely unused, so
		// it is minted and held rather than stamped from the clock — a bare
		// millisecond collides with a second detached run in the same
		// millisecond, and with any earlier run that left its key on disk. See
		// mintDetachedSessionKey for what a collision inherits.
		minted, err := g.mintDetachedSessionKey(agentName)
		if err != nil {
			return "", noReservation, err
		}
		// Held for the whole request rather than until the session is stored:
		// run() blocks on the turn anyway, and a reservation dropped early is a
		// reservation that stopped protecting anything.
		return minted, func() { g.releaseSessionKeyReservation(minted) }, nil
	}

	if req.NewSession {
		// Fresh session: use the main key. The session already sitting under
		// that key is released inside the queue, not here — the queue's
		// per-session lock has by then serialized this request behind any turn
		// still running on that key, which is what makes closing it safe.
		return mainSessionKey(agentName), noReservation, nil
	}

	if req.SessionKey == "" {
		return mainSessionKey(agentName), noReservation, nil
	}
	return req.SessionKey, noReservation, nil
}

// mintUnusedSessionKey returns a session key that nothing else is using. It is
// the checking half shared by every mint; propose supplies the shape.
//
// subject names what the key is for and appears in the refusal, so a caller
// reading a failure learns which spawn or run could not be given a key.
//
// A caller holds the returned key until the session it built is in g.sessions,
// then releases it with releaseSessionKeyReservation — the reservation is what
// keeps two concurrent callers from passing the same check before either has
// stored anything.
func (g *Server) mintUnusedSessionKey(subject string, propose func(attempt int) string) (string, error) {
	for attempt := 0; attempt < sessionKeyMintAttempts; attempt++ {
		candidate := propose(attempt)
		if _, reserved := g.pendingSessionKeys.LoadOrStore(candidate, struct{}{}); reserved {
			continue
		}
		inUse, err := g.sessionKeyInUse(candidate)
		if err != nil {
			g.pendingSessionKeys.Delete(candidate)
			return "", fmt.Errorf("mint a session key for %s: %w", subject, err)
		}
		if inUse {
			g.pendingSessionKeys.Delete(candidate)
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("mint a session key for %s: %d proposed keys were all taken",
		subject, sessionKeyMintAttempts)
}

// releaseSessionKeyReservation drops a reservation taken by one of the mints.
// It is safe to call once the session is in g.sessions — which is itself one of
// the things sessionKeyInUse reads — and it must be called when the session
// could not be built, or the key is held out of circulation until the process
// exits.
func (g *Server) releaseSessionKeyReservation(key string) {
	g.pendingSessionKeys.Delete(key)
}

// sessionKeyInUse reports whether a key already names a session somewhere that
// matters: a live one in memory, a recorded one in the store, or a persisted one
// on disk. All three are asked, because they can disagree — a session that ran
// before the last restart is gone from memory and present in both of the others,
// and one whose row was deleted may still have its transcript directory.
//
// An unreadable store or an unreadable data directory is an error, not a "no".
// Answering "free" there is how a key that IS taken gets handed out, which is
// the thing this exists to prevent.
func (g *Server) sessionKeyInUse(key string) (bool, error) {
	if _, live := g.sessions.Load(key); live {
		return true, nil
	}
	if g.store != nil {
		recorded, err := g.store.SessionExists(key)
		if err != nil {
			return false, err
		}
		if recorded {
			return true, nil
		}
	}
	dir := filepath.Join(g.config.DataDir, "sessions", key)
	switch _, err := os.Stat(dir); {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("check whether session %s is persisted: %w", key, err)
	}
}