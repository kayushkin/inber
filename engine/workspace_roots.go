package engine

import (
	"fmt"
	"sort"
	"strings"
)

// WorkspaceRoot is one repository a session has checked out to work in.
//
// A forge workspace holds one worktree per repository, and until this type
// existed only one of them was ever named. The server collapsed the whole
// workspace to ws.Repos[ws.Primary] and handed that single path over as the
// session's repo root, so the other worktrees sat on disk unmentioned — while
// forge's CommitAll committed them, MergeToMain merged them and PushAll pushed
// them. An agent asked to change two repositories could see one, and the commit
// it never made was merged in its name.
type WorkspaceRoot struct {
	// Name is the repository name forge knows it by.
	Name string
	// Path is the absolute path to this repository's worktree. It is absolute
	// because that is what the model has to write to reach a repository that is
	// not the primary one: tools.ScopeToRoot resolves a relative path against
	// the primary root and passes an absolute path through as written.
	Path string
	// Primary marks the root that relative paths resolve against. Exactly one
	// root in a set carries it.
	Primary bool
}

// PrimaryWorkspaceRoot returns the path of the root marked primary, or "" when
// there is no such root. Callers use it instead of indexing the slice so that
// the meaning of "primary" stays in one place rather than becoming "whichever
// one is first".
func PrimaryWorkspaceRoot(roots []WorkspaceRoot) string {
	for _, root := range roots {
		if root.Primary {
			return root.Path
		}
	}
	return ""
}

// RepoRoot returns the directory this session's filesystem tools resolve
// relative paths against.
//
// It is exported because where a session actually works is the fact callers
// have twice got wrong by reading it off something else — an agent's stored
// config, a path-shaped display name — and a fact nobody can read is a fact
// every test has to take on trust from the value it passed in.
func (e *Engine) RepoRoot() string {
	return e.repoRoot
}

// validateWorkspaceRoots checks that a set of roots describes one workspace the
// engine can actually work in, and that its primary is the root the rest of the
// engine was already built around.
//
// The primary path and EngineConfig.RepoRoot are the same fact reaching the
// engine by two routes — the prompt is built from the first and every
// filesystem tool is rooted at the second. If they ever disagree, the model is
// told it is working somewhere its tools do not write, which is the defect this
// whole file exists to close, arriving inverted. So they are compared once,
// here, and a mismatch fails the session rather than running it.
//
// No roots at all is not an error: a session outside a forge workspace has one
// repository, has always had one, and needs nothing said about it.
func validateWorkspaceRoots(roots []WorkspaceRoot, repoRoot string) error {
	if len(roots) == 0 {
		return nil
	}
	primaries := 0
	for _, root := range roots {
		if root.Name == "" {
			return fmt.Errorf("a workspace root has no repository name (path %q)", root.Path)
		}
		if root.Path == "" {
			return fmt.Errorf("workspace root %q has no worktree path", root.Name)
		}
		if root.Primary {
			primaries++
		}
	}
	if primaries != 1 {
		return fmt.Errorf("a workspace must have exactly one primary repository, this one has %d", primaries)
	}
	if primary := PrimaryWorkspaceRoot(roots); primary != repoRoot {
		return fmt.Errorf("the primary repository is %s but the session is rooted at %q", primary, repoRoot)
	}
	return nil
}

// renderWorkspaceRoots names every repository the session works in, marking the
// primary one, and returns "" when there is nothing worth saying.
//
// Below two roots it renders nothing at all. A session with a single repository
// has never been told about it and does not need to be — the tools already
// resolve relative paths there — and saying so anyway would add bytes to the
// last user message of every existing single-repo session. Those bytes persist
// into the conversation and become part of the prefix a later turn caches, so
// the cheapest way to keep them from moving is not to write them.
//
// The order is fixed rather than the map order the caller read the workspace
// from: primary first, the rest by name. An unordered walk would hand the model
// a different rendering on every session with no change in meaning.
func renderWorkspaceRoots(roots []WorkspaceRoot) string {
	if len(roots) < 2 {
		return ""
	}

	ordered := make([]WorkspaceRoot, len(roots))
	copy(ordered, roots)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Primary != ordered[j].Primary {
			return ordered[i].Primary
		}
		return ordered[i].Name < ordered[j].Name
	})

	var b strings.Builder
	fmt.Fprintf(&b, "## Workspace repositories\nThis session has %d repositories checked out. "+
		"A relative path resolves inside the primary one; name a file in any other by its absolute path.\n",
		len(ordered))
	for _, root := range ordered {
		if root.Primary {
			fmt.Fprintf(&b, "- **%s** (primary) — %s\n", root.Name, root.Path)
			continue
		}
		fmt.Fprintf(&b, "- **%s** — %s\n", root.Name, root.Path)
	}
	return strings.TrimRight(b.String(), "\n")
}
