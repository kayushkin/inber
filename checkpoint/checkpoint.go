// Package checkpoint manages workspace state snapshots for agent sessions.
//
// Before an agent modifies files, a checkpoint captures the workspace state.
// After each turn, if files changed, a new checkpoint is created. Users can
// diff any two checkpoints or restore to a prior state.
//
// Implementation uses git commits on a detached checkpoint branch:
//
//	.inber/checkpoints/{session_id}/
//	    checkpoint_001  → git commit hash
//	    checkpoint_002  → git commit hash
//	    ...
//
// Checkpoints are lightweight (git objects, not file copies) and enable:
//   - Comparing workspace state before/after any turn
//   - Restoring to any prior checkpoint
//   - Viewing the diff of what an agent changed per turn
//
// Usage:
//
//	mgr, _ := checkpoint.New(repoRoot, sessionID)
//	mgr.Take("before turn 3")  // snapshot current state
//	// ... agent modifies files ...
//	mgr.Take("after turn 3")   // snapshot modified state
//	diff := mgr.Diff(1, 2)     // compare checkpoints
//	mgr.Restore(1)             // roll back to checkpoint 1
package checkpoint

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

// New creates a checkpoint manager for the given repo and session.
// Returns nil if the directory is not a git repository.
func New(repoRoot, sessionID string) (*Manager, error) {
	// TODO: verify git repo, create checkpoint branch if needed
	return &Manager{
		repoRoot:  repoRoot,
		sessionID: sessionID,
	}, nil
}

// Take creates a checkpoint of the current workspace state.
// Label is a human-readable description (e.g., "before turn 3").
func (m *Manager) Take(label string, turnNum int) (*Checkpoint, error) {
	if m == nil {
		return nil, nil
	}
	// TODO: git add -A && git commit on checkpoint branch
	return nil, nil
}

// List returns all checkpoints for this session.
func (m *Manager) List() []Checkpoint {
	if m == nil {
		return nil
	}
	return m.points
}

// Diff returns the file changes between two checkpoints.
func (m *Manager) Diff(from, to int) (string, error) {
	if m == nil {
		return "", nil
	}
	// TODO: git diff between two checkpoint commits
	return "", nil
}

// DiffFromPrevious returns changes since the previous checkpoint.
func (m *Manager) DiffFromPrevious(num int) (string, error) {
	if num <= 1 {
		return "", nil
	}
	return m.Diff(num-1, num)
}

// Restore rolls the workspace back to a specific checkpoint.
func (m *Manager) Restore(num int) error {
	if m == nil {
		return nil
	}
	// TODO: git checkout checkpoint commit, apply to working tree
	return nil
}
