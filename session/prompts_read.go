package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ListPromptBreakdowns lists prompt breakdown files for a session.
func ListPromptBreakdowns(logsDir, sessionID string) ([]string, error) {
	var files []string

	// New format: look in {logsDir}/*/{sessionID}/prompts/
	// Legacy: look in {logsDir}/*/prompts/{sessionID}-turn-*.md
	err := filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		// New format: turn-N.md inside a session dir
		if strings.HasPrefix(info.Name(), "turn-") && strings.Contains(path, sessionID) {
			files = append(files, path)
		}
		// Legacy format: sessionID-turn-N.md
		if strings.HasPrefix(info.Name(), sessionID) {
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
func ReadPromptBreakdown(logsDir, sessionID string, turn int) (string, error) {
	newFilename := fmt.Sprintf("turn-%d.md", turn)
	legacyFilename := fmt.Sprintf("%s-turn-%d.md", sessionID, turn)

	var found string
	filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if (info.Name() == newFilename && strings.Contains(path, sessionID)) ||
			info.Name() == legacyFilename {
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