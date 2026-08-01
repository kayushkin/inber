package server

import (
	"context"
	"encoding/json"
	"fmt"
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

// sessionKeyForChild generates a child session key.
func sessionKeyForChild(parentKey string) string {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	return parentKey + childKeySeparator + suffix
}