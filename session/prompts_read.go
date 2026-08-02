package session

import (
	"fmt"
	"os"
	"path/filepath"
)

// ListPromptBreakdowns lists prompt breakdown files for a session.
//
// A file belongs to the session its own path names — see PromptBreakdown, which
// owns both layouts. Asking instead whether the id appeared somewhere in the
// path listed another session's turns whenever this id was a prefix of that
// one's, and listed every turn under a root whose own directory name held an
// id. The legacy half was looser still: it claimed any .md file whose name
// merely started with the id, which is every system block file a run of that
// session wrote.
func ListPromptBreakdowns(logsDir, sessionID string) ([]string, error) {
	var files []string

	// Current: {logsDir}/*/{sessionID}/prompts/turn-N.md
	// Legacy:  {logsDir}/*/prompts/{sessionID}-turn-N.md
	err := filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if owner, _, ok := PromptBreakdown(path); ok && owner == sessionID {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk logs directory %s looking for session %s prompts: %w", logsDir, sessionID, err)
	}
	return files, nil
}

// ReadPromptBreakdown reads a specific prompt breakdown.
//
// It asks the same question ListPromptBreakdowns asks, so a turn that is listed
// is a turn that reads back. The two used to spell the match differently, and
// the current-layout half was the substring test described there.
func ReadPromptBreakdown(logsDir, sessionID string, turn int) (string, error) {
	var found string
	filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if owner, n, ok := PromptBreakdown(path); ok && owner == sessionID && n == turn {
			found = path
			return filepath.SkipAll
		}
		return nil
	})

	if found == "" {
		return "", fmt.Errorf("prompt breakdown not found: %s turn %d", sessionID, turn)
	}

	data, err := os.ReadFile(found)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt breakdown file %s: %w", found, err)
	}
	return string(data), nil
}