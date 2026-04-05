package agent

import (
	"encoding/json"
	"fmt"
	"sync"
)

// ReadCache tracks files that have been fully read in the current session.
// When a file is already in context (fully read and not modified since),
// subsequent reads return a short stub instead of re-reading.
type ReadCache struct {
	mu    sync.Mutex
	files map[string]readEntry // path → entry
}

type readEntry struct {
	turn  int // turn when last fully read
	lines int // line count at time of read
}

func NewReadCache() *ReadCache {
	return &ReadCache{files: make(map[string]readEntry)}
}

// RecordFullRead marks a file as fully read on this turn.
func (rc *ReadCache) RecordFullRead(path string, turn, lines int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.files[path] = readEntry{turn: turn, lines: lines}
}

// Invalidate removes a file from the cache (after write/edit).
func (rc *ReadCache) Invalidate(path string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	delete(rc.files, path)
}

// Check returns a stub message if the file is already fully in context.
// Returns ("", false) if the read should proceed normally.
func (rc *ReadCache) Check(path string) (string, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	entry, ok := rc.files[path]
	if !ok {
		return "", false
	}
	return fmt.Sprintf("[already in context — %d lines, read on turn %d. No need to re-read.]", entry.lines, entry.turn), true
}

// isFullRead checks if a read_files tool input represents a full read (no offset/limit).
func isFullRead(toolName, rawInput string) (path string, isFull bool) {
	if toolName != "read_files" && toolName != "read_file" {
		return "", false
	}
	var inp map[string]interface{}
	if json.Unmarshal([]byte(rawInput), &inp) != nil {
		return "", false
	}
	
	// Multi-file reads aren't cached individually
	if _, ok := inp["paths"]; ok {
		return "", false
	}
	
	p, ok := inp["path"].(string)
	if !ok || p == "" {
		return "", false
	}

	// Check for offset/limit
	if offset, ok := inp["offset"].(float64); ok && offset > 0 {
		return p, false
	}
	if limit, ok := inp["limit"].(float64); ok && limit > 0 {
		return p, false
	}
	return p, true
}

// isPartialRead checks if a read_files call uses offset/limit on a single file.
func isPartialRead(toolName, rawInput string) (path string, isPartial bool) {
	if toolName != "read_files" && toolName != "read_file" {
		return "", false
	}
	var inp map[string]interface{}
	if json.Unmarshal([]byte(rawInput), &inp) != nil {
		return "", false
	}
	if _, ok := inp["paths"]; ok {
		return "", false
	}
	p, ok := inp["path"].(string)
	if !ok || p == "" {
		return "", false
	}
	hasOffset := false
	hasLimit := false
	if offset, ok := inp["offset"].(float64); ok && offset > 0 {
		hasOffset = true
	}
	if limit, ok := inp["limit"].(float64); ok && limit > 0 {
		hasLimit = true
	}
	if hasOffset || hasLimit {
		return p, true
	}
	return p, false
}

// extractCompleteFileLines parses the "[complete file — N lines]" footer.
// Returns the line count, or 0 if the output doesn't indicate a complete read.
func extractCompleteFileLines(output string) int {
	// Look for the footer pattern at the end of output
	idx := len(output) - 1
	for idx >= 0 && output[idx] == '\n' {
		idx--
	}
	// Find the last line
	lineStart := idx
	for lineStart > 0 && output[lineStart-1] != '\n' {
		lineStart--
	}
	lastLine := output[lineStart : idx+1]

	// Parse "[complete file — N lines]"
	if len(lastLine) < 20 {
		return 0
	}
	var lines int
	// Try both dash types
	if n, _ := fmt.Sscanf(lastLine, "[complete file — %d lines]", &lines); n == 1 {
		return lines
	}
	if n, _ := fmt.Sscanf(lastLine, "[complete file - %d lines]", &lines); n == 1 {
		return lines
	}
	return 0
}

// isFileWrite checks if a tool call is a write/edit that should invalidate the cache.
func isFileWrite(toolName, rawInput string) []string {
	switch toolName {
	case "write_file", "write_files", "edit_file", "edit_files":
	default:
		return nil
	}
	var inp map[string]interface{}
	if json.Unmarshal([]byte(rawInput), &inp) != nil {
		return nil
	}
	var paths []string
	if p, ok := inp["path"].(string); ok && p != "" {
		paths = append(paths, p)
	}
	// Handle batch writes/edits
	for _, key := range []string{"files", "edits"} {
		if items, ok := inp[key].([]interface{}); ok {
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					if p, ok := m["path"].(string); ok && p != "" {
						paths = append(paths, p)
					}
				}
			}
		}
	}
	return paths
}
