package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/agent"
)

// fix_workspace spawns, and a spawn is charged to a parent: Spawn checks the
// depth cap and the children quota against it, derives the child's key from it,
// appends the child to its Children, and points the child's event forwarder and
// every progress delivery at it.
//
// The tool used to reconstruct that parent by ranging g.sessions for whichever
// session happened to be Running, with the comment "for now, we need the parent
// key passed or inferred". sync.Map iteration order is unspecified, and
// inber-server runs many sessions at once — so on a host with more than one
// live session the fix agent was charged to, and reported to, an arbitrary one.
//
// The key was never missing. toolsForAgent already receives it and already
// hands it to SpawnAgentTool one line above; FixWorkspaceTool alone never took
// it.

// fixWorkspaceServer builds a server holding one reopenable workspace, and
// returns the workspace id.
func fixWorkspaceServer(t *testing.T) (*Server, string) {
	t.Helper()
	srv := newTestServer(t, &mockWorkspaceManager{})
	ws := &forge.Workspace{
		ID:      "brigid-123",
		Repos:   map[string]string{"kayushkin": "/tmp/ws/kayushkin"},
		Primary: "kayushkin",
		Branch:  "spawn/brigid-123",
		Status:  "done",
	}
	srv.mu.Lock()
	srv.workspaces[ws.ID] = ws
	srv.mu.Unlock()
	return srv, ws.ID
}

// fixWorkspaceTool picks the fix_workspace tool out of the set an orchestrator
// session is given. Going through toolsForAgent rather than calling
// FixWorkspaceTool directly is the point: the wiring is half the defect.
func fixWorkspaceTool(t *testing.T, srv *Server, sessionKey string) agent.Tool {
	t.Helper()
	for _, tool := range srv.toolsForAgent(sessionKey, srv.config.DefaultAgent) {
		if tool.Name == "fix_workspace" {
			return tool
		}
	}
	t.Fatalf("session %s was given no fix_workspace tool", sessionKey)
	return agent.Tool{}
}

func runFixWorkspace(t *testing.T, tool agent.Tool, workspaceID string) error {
	t.Helper()
	in, err := json.Marshal(map[string]any{
		"workspace_id": workspaceID,
		"agent":        "brigid",
		"instructions": "the tests do not pass",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := tool.Run(context.Background(), string(in))
	return runErr
}

// The deterministic half. A session that has gone away must not have its fix
// request handed to a stranger: there is exactly one other session here, it is
// Running, and the old scan would have found it and spawned into it.
//
// Both outcomes are fast and need no engine — the bystander sits at the depth
// cap, so choosing it stops at the depth gate rather than wandering into
// createSession — and the two gates report different errors, so which parent
// was used is readable off the error alone.
func TestFixWorkspaceFromAVanishedSessionDoesNotAttachToAStranger(t *testing.T) {
	srv, workspaceID := fixWorkspaceServer(t)

	srv.sessions.Store("session:bystander", &Session{
		Key:        "session:bystander",
		AgentName:  "claxon",
		Status:     Running,
		SpawnDepth: srv.config.MaxSpawnDepth,
	})

	err := runFixWorkspace(t, fixWorkspaceTool(t, srv, "session:gone"), workspaceID)
	if err == nil {
		t.Fatal("a fix_workspace call from a session that no longer exists was accepted; it was spawned into some other session")
	}
	if strings.Contains(err.Error(), "max spawn depth") {
		t.Fatalf("the call was charged to the bystander session, not the caller: %v", err)
	}
	if !strings.Contains(err.Error(), "session:gone") {
		t.Fatalf("the failure does not name the calling session, so the parent it used is not the caller's: %v", err)
	}
}

// The positive direction: with several other sessions Running, the caller's own
// key is the one that reaches Spawn. The caller sits at the depth cap and every
// bystander sits at the children quota, so each gate reports a different error
// and the error names which parent was charged.
//
// Under the old scan this fails only when the range happens to land on a
// bystander — eight of nine times. That is the safe direction for a flake: it
// can miss the defect, never invent one.
func TestFixWorkspaceIsChargedToTheCallingSessionNotAnyRunningOne(t *testing.T) {
	srv, workspaceID := fixWorkspaceServer(t)

	const caller = "session:caller"
	srv.sessions.Store(caller, &Session{
		Key:        caller,
		AgentName:  "claxon",
		Status:     Running,
		SpawnDepth: srv.config.MaxSpawnDepth,
	})

	quota := make([]string, srv.config.MaxChildrenPerAgent)
	for i := range quota {
		quota[i] = "child"
	}
	for _, key := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		srv.sessions.Store("session:"+key, &Session{
			Key:       "session:" + key,
			AgentName: "claxon",
			Status:    Running,
			Children:  quota,
		})
	}

	err := runFixWorkspace(t, fixWorkspaceTool(t, srv, caller), workspaceID)
	if err == nil {
		t.Fatal("the spawn got past both caps; neither parent's limits were checked")
	}
	if strings.Contains(err.Error(), "max children") {
		t.Fatalf("the fix was charged to a bystander session rather than the caller: %v", err)
	}
	if !strings.Contains(err.Error(), "max spawn depth") {
		t.Fatalf("the caller's depth cap was not the gate that stopped this: %v", err)
	}
}

// A fix_workspace call is refused when its own session is gone, rather than
// falling back to any running session — including when there is no other
// session at all. Without this, "does not attach to a stranger" is satisfied by
// a tool that refuses everything for some unrelated reason.
func TestFixWorkspaceStillWorksForTheOnlyRunningSession(t *testing.T) {
	srv, workspaceID := fixWorkspaceServer(t)

	const caller = "session:only"
	srv.sessions.Store(caller, &Session{
		Key:        caller,
		AgentName:  "claxon",
		Status:     Running,
		SpawnDepth: srv.config.MaxSpawnDepth,
	})

	err := runFixWorkspace(t, fixWorkspaceTool(t, srv, caller), workspaceID)
	if err == nil || !strings.Contains(err.Error(), "max spawn depth") {
		t.Fatalf("the sole running session's own fix request did not reach its own depth cap: %v", err)
	}
}
