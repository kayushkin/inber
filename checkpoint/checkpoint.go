// Package checkpoint is a design sketch for workspace state snapshots. Nothing
// in it is implemented.
//
// Every method returns ErrNotImplemented. That is the point of this file in its
// current state: before it, each one returned a zero value and a nil error, so
// Take reported a snapshot it had not taken and Restore reported a workspace
// rollback without touching a file. A safety feature that reports success while
// doing nothing is worse than an absent one, because the absent one cannot be
// planned on.
//
// The design below is intent, not behaviour. Building it needs three answers
// that are not mechanical, and they are the reason this is a sketch rather than
// an implementation:
//
//   - Does "checkpoint" mean rewinding the conversation, rewinding the
//     workspace, or the atomic pair? session/checkpoint.go already writes the
//     conversation half, and the two do not know about each other.
//   - Per turn, or per user turn? A turn that runs fifty tool round-trips would
//     otherwise produce fifty commits.
//   - Are untracked files captured? git stash semantics miss exactly the files
//     an agent most often creates, so a restore that rewinds tracked files only
//     leaves a workspace that never existed at any point in the session.
//
// The intended implementation is git commits on a detached checkpoint branch:
//
//	.inber/checkpoints/{session_id}/
//	    checkpoint_001  → git commit hash
//	    checkpoint_002  → git commit hash
//	    ...
//
// which would be lightweight (git objects, not file copies) and would enable
// comparing workspace state before and after any turn, restoring to any prior
// checkpoint, and viewing the diff of what an agent changed per turn.
//
// The intended shape of a session, once it exists:
//
//	mgr, _ := checkpoint.New(repoRoot, sessionID)
//	mgr.Take("before turn 3")  // snapshot current state
//	// ... agent modifies files ...
//	mgr.Take("after turn 3")   // snapshot modified state
//	diff := mgr.Diff(1, 2)     // compare checkpoints
//	mgr.Restore(1)             // roll back to checkpoint 1
package checkpoint

import "errors"

// ErrNotImplemented is returned by every function in this package. See the
// package doc: this is a design sketch, and a caller must be able to tell that
// from the return value rather than from reading the source.
var ErrNotImplemented = errors.New("checkpoint: workspace snapshots are not implemented")

// Checkpoint represents a single workspace state snapshot.
type Checkpoint struct {
	Number    int    `json:"number"`
	Label     string `json:"label"`
	CommitSHA string `json:"commit_sha"`
	TurnNum   int    `json:"turn_number"`
	FileCount int    `json:"file_count"` // files changed since previous checkpoint
}

// Manager handles checkpoint lifecycle for a session.
type Manager struct {
	repoRoot  string
	sessionID string
	points    []Checkpoint
}

// New would create a checkpoint manager for the given repo and session, and
// verify that repoRoot is a git repository. It returns ErrNotImplemented.
func New(repoRoot, sessionID string) (*Manager, error) {
	return nil, ErrNotImplemented
}

// Take would create a checkpoint of the current workspace state, labelled with
// a human-readable description (e.g., "before turn 3"). It returns
// ErrNotImplemented.
func (m *Manager) Take(label string, turnNum int) (*Checkpoint, error) {
	return nil, ErrNotImplemented
}

// List would return all checkpoints for this session. It returns
// ErrNotImplemented.
//
// The error is new: this used to return a nil slice, which a caller reads as
// "this session has no checkpoints" rather than "checkpoints do not exist".
func (m *Manager) List() ([]Checkpoint, error) {
	return nil, ErrNotImplemented
}

// Diff would return the file changes between two checkpoints. It returns
// ErrNotImplemented.
func (m *Manager) Diff(from, to int) (string, error) {
	return "", ErrNotImplemented
}

// DiffFromPrevious would return changes since the previous checkpoint. It
// returns ErrNotImplemented.
func (m *Manager) DiffFromPrevious(num int) (string, error) {
	return "", ErrNotImplemented
}

// Restore would roll the workspace back to a specific checkpoint. It returns
// ErrNotImplemented.
//
// This is the method the rest of the package exists for, and the one whose old
// nil return was most dangerous: a caller asking to undo an agent's file
// changes was told the rollback had happened.
func (m *Manager) Restore(num int) error {
	return ErrNotImplemented
}
