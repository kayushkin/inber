package server

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/engine"
)

// workspaceRoots turns a forge workspace into the set of repositories the
// engine is told about, with the primary one marked.
//
// Both callers used to read ws.Repos[ws.Primary] straight out of the map and
// keep only that one path. Everything else forge had checked out stayed
// invisible to the model while CommitAll, MergeToMain and PushAll went on
// operating on it.
//
// A workspace whose Primary names no repository is an error here rather than a
// warning, because indexing a map with a missing key yields "" — and "" reaches
// the engine as "no root is known", which sends every relative path in every
// tool call back to the inber-server process's own working directory. That is
// the leak tools.ScopeToRoot was written to close, arriving through a second
// door and just as silently.
func workspaceRoots(ws *forge.Workspace) ([]engine.WorkspaceRoot, error) {
	if ws == nil {
		return nil, fmt.Errorf("no workspace")
	}
	names := make([]string, 0, len(ws.Repos))
	for name := range ws.Repos {
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		return nil, fmt.Errorf("workspace %s has no repositories", ws.ID)
	}
	primaryPath, isARepository := ws.Repos[ws.Primary]
	if !isARepository {
		return nil, fmt.Errorf("workspace %s names %q as its primary repository, and holds no repository by that name (it holds: %s)",
			ws.ID, ws.Primary, strings.Join(names, ", "))
	}
	if primaryPath == "" {
		return nil, fmt.Errorf("workspace %s has no worktree path for its primary repository %q", ws.ID, ws.Primary)
	}

	roots := make([]engine.WorkspaceRoot, 0, len(names))
	roots = append(roots, engine.WorkspaceRoot{Name: ws.Primary, Path: primaryPath, Primary: true})
	for _, name := range names {
		if name == ws.Primary {
			continue
		}
		path := ws.Repos[name]
		if path == "" {
			return nil, fmt.Errorf("workspace %s has no worktree path for its repository %q", ws.ID, name)
		}
		roots = append(roots, engine.WorkspaceRoot{Name: name, Path: path})
	}
	return roots, nil
}

// useWorkspace points an agent config at a forge workspace: every repository it
// holds, and the primary one as the working directory.
//
// The two facts are set together and from one reading of the workspace so they
// cannot disagree. The engine checks that they still agree when it starts, and
// refuses to run a session where the model is told it works somewhere its tools
// do not write.
func useWorkspace(ac *AgentConfig, ws *forge.Workspace) error {
	roots, err := workspaceRoots(ws)
	if err != nil {
		return err
	}
	ac.WorkspaceRoots = roots
	ac.Workspace = engine.PrimaryWorkspaceRoot(roots)
	return nil
}
