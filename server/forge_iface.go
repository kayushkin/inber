package server

import (
	"os"

	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/logger"
)

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

// openWorkspaceManager opens the forge database that backs ephemeral workspaces,
// and returns nil when there is no usable one.
//
// The return type is the interface on purpose, and it is the whole point of this
// function. A `*forge.Forge` variable that is nil still carries a type, so assigning
// it into a WorkspaceManager field produces an interface that is NOT nil — every
// `forgeDB != nil` guard in this package then passes and the very next call
// dereferences a nil receiver. Returning the interface directly means the only value
// a caller can store for "no forge" is an untyped nil, so those guards tell the truth.
//
// A missing or unopenable database is not an error: workspaces are optional, and the
// server runs without them. The caller gets nil and every workspace tool then refuses
// with "forge not available" instead of crashing the process.
func openWorkspaceManager(forgeDatabasePath string) WorkspaceManager {
	if _, err := os.Stat(forgeDatabasePath); err != nil {
		return nil
	}
	openedForge, err := forge.Open(forgeDatabasePath)
	if err != nil {
		logger.WithComponent("server").Warn("forge unavailable", map[string]interface{}{
			"error": err,
			"path":  forgeDatabasePath,
		})
		return nil
	}
	if openedForge == nil {
		// forge.Open reported success with no value; treat it as unavailable rather
		// than boxing a typed nil back into the interface this function exists to keep honest.
		logger.WithComponent("server").Warn("forge opened but returned no handle", map[string]interface{}{
			"path": forgeDatabasePath,
		})
		return nil
	}
	logger.WithComponent("server").Info("forge DB opened", map[string]interface{}{
		"path": forgeDatabasePath,
	})
	return openedForge
}
