package server

import "github.com/kayushkin/forge"

// WorkspaceManager defines the forge operations that the server depends on.
// This interface abstracts workspace management operations, making the server
// testable and enabling future forge backend swapping (e.g., different VCS systems).
//
// The interface encompasses the full lifecycle of ephemeral workspaces:
//   - CreateWorkspace: Set up isolated project copies for sub-agents
//   - CommitAll: Save sub-agent changes to the spawn branch
//   - MergeToMain: Integrate approved changes into main branches
//   - PushAll: Sync changes to remote repositories  
//   - Cleanup: Remove workspace when no longer needed
//   - ReopenWorkspace: Restore previously closed workspace for further work
//   - Close: Shut down the workspace manager
//
// Tests can mock this interface instead of depending on the concrete *forge.Forge.
// Future implementations could support different backends (Git, Mercurial, Perforce).
type WorkspaceManager interface {
	// CreateWorkspace creates an ephemeral workspace with isolated copies of the specified projects.
	// The agent parameter identifies who is creating the workspace (for naming/tracking).
	// The projects parameter lists the project IDs to include in the workspace.
	CreateWorkspace(agent string, projects []string) (*forge.Workspace, error)
	
	// CommitAll commits all pending changes in the workspace with the given message.
	// Returns per-repository commit results (hash, dirty status, errors).
	CommitAll(ws *forge.Workspace, message string) (map[string]forge.CommitResult, error)
	
	// MergeToMain merges the workspace's spawn branch into the main branch for each repo.
	// Returns per-repository merge results (ok, conflict, error).
	MergeToMain(ws *forge.Workspace) map[string]forge.MergeResult
	
	// PushAll pushes changes to remote repositories.
	// Returns per-repository push errors (nil = success).
	PushAll(ws *forge.Workspace) map[string]error
	
	// Cleanup removes the workspace and its files.
	// Should be called when the workspace is no longer needed.
	Cleanup(ws *forge.Workspace) error
	
	// ReopenWorkspace restores a previously closed workspace for additional work.
	// Useful for "fix this workspace" operations.
	ReopenWorkspace(ws *forge.Workspace) error
	
	// Close shuts down the workspace manager and releases any resources.
	Close() error
}

// Compile-time check: *forge.Forge must satisfy WorkspaceManager.
var _ WorkspaceManager = (*forge.Forge)(nil)
