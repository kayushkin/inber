package server

import "github.com/kayushkin/forge"

// WorkspaceManager defines the forge operations that the server depends on.
// This interface is the contract between server and forge — if forge changes
// a method signature, this won't compile. Tests can mock this interface
// instead of the concrete *forge.Forge type.
type WorkspaceManager interface {
	CreateWorkspace(agent string, projects []string) (*forge.Workspace, error)
	CommitAll(ws *forge.Workspace, message string) (map[string]forge.CommitResult, error)
	MergeToMain(ws *forge.Workspace) map[string]forge.MergeResult
	PushAll(ws *forge.Workspace) map[string]error
	Cleanup(ws *forge.Workspace) error
	ReopenWorkspace(ws *forge.Workspace) error
	Close() error
}

// Compile-time check: *forge.Forge must satisfy WorkspaceManager.
var _ WorkspaceManager = (*forge.Forge)(nil)
