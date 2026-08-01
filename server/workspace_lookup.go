package server

import (
	"fmt"

	"github.com/kayushkin/forge"
)

// workspaceByID answers which forge workspace an id names, for the three tools
// that act on one.
//
// Server.workspaces is an in-memory map written by Spawn, and it used to be the
// only place merge_workspace, reject_workspace and fix_workspace ever looked. So
// after an inber-server restart all three refused with "workspace not found"
// while the worktrees sat on disk holding the work. Recording a session's
// workspace roots (a216b33) fixed the half where a revived session edited this
// host's live checkout instead of its worktree; this is the other half. The
// changes were no longer written into the wrong repository, and nothing could
// get them out of the right one.
//
// A miss asks forge, which records every workspace beside its own worktrees.
// The answer is kept in the map, and a concurrent caller's answer wins over it,
// so two tools acting on one workspace hold one value — status is a field of it
// and forge moves it as the workspace is committed, merged and reopened.
func (g *Server) workspaceByID(id string) (*forge.Workspace, error) {
	g.mu.RLock()
	ws, inMemory := g.workspaces[id]
	g.mu.RUnlock()
	if inMemory {
		return ws, nil
	}

	if g.forgeDB == nil {
		return nil, fmt.Errorf("workspace not found: %s (there is no forge database on this host, so nothing records workspaces)", id)
	}
	recorded, err := g.forgeDB.GetWorkspace(id)
	if err != nil {
		return nil, fmt.Errorf("workspace not found: %s: %w", id, err)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if raced, alreadyThere := g.workspaces[id]; alreadyThere {
		return raced, nil
	}
	g.workspaces[id] = recorded
	return recorded, nil
}
