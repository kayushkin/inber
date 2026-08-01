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

// childKeyMintAttempts bounds how many keys are proposed before a spawn is
// refused. Each attempt reads the clock afresh, and on this host time.Now
// advances every ~70ns, so consecutive proposals differ; the bound is there for
// the case the loop cannot solve — a suffix space so full that drawing from it
// keeps landing on a taken key — where the honest answer is an error, not a
// loop that never returns.
const childKeyMintAttempts = 100

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
// then releases it with releaseChildSessionKey — the reservation is what keeps
// two concurrent spawns from passing the same check before either has stored
// anything.
func (g *Server) mintChildSessionKey(parentKey string) (string, error) {
	return g.mintChildSessionKeyFrom(parentKey, sessionKeyForChild)
}

// mintChildSessionKeyFrom is mintChildSessionKey with the proposal separated
// from the checking, so a test can script the collision the clock only produces
// once in tens of thousands of spawns.
func (g *Server) mintChildSessionKeyFrom(parentKey string, propose func(parentKey string) string) (string, error) {
	for attempt := 0; attempt < childKeyMintAttempts; attempt++ {
		candidate := propose(parentKey)
		if _, reserved := g.pendingChildKeys.LoadOrStore(candidate, struct{}{}); reserved {
			continue
		}
		inUse, err := g.sessionKeyInUse(candidate)
		if err != nil {
			g.pendingChildKeys.Delete(candidate)
			return "", fmt.Errorf("mint a session key for a child of %s: %w", parentKey, err)
		}
		if inUse {
			g.pendingChildKeys.Delete(candidate)
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("mint a session key for a child of %s: %d proposed keys were all taken",
		parentKey, childKeyMintAttempts)
}

// releaseChildSessionKey drops a reservation taken by mintChildSessionKey. It is
// safe to call once the session is in g.sessions — which is itself one of the
// things sessionKeyInUse reads — and it must be called when the session could
// not be built, or the key is held out of circulation until the process exits.
func (g *Server) releaseChildSessionKey(key string) {
	g.pendingChildKeys.Delete(key)
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