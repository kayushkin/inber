package agent

import (
	"encoding/json"
	"fmt"
	"sync"
)

// ReadCache tracks files that have been fully read during one turn. When a file
// is already in context (fully read and not modified since), subsequent reads
// return a short stub instead of re-reading.
//
// The scope is the turn, not the session: engine.buildAgent constructs a fresh
// Agent — and so a fresh ReadCache — before every Agent.Run, so nothing here
// outlives the turn that filled it. That is why the stub says "earlier this
// turn" and why the cache can get away with invalidating only on the file tools
// it can see: a shell `sed -i`, a `git checkout` or a human editing a file
// between turns all happen while the cache is empty. Widening the scope means
// widening the invalidation set first.
type ReadCache struct {
	mu    sync.Mutex
	root  string               // the root tool paths resolve against; "" = the process working directory
	files map[string]readEntry // canonical file path → entry
}

type readEntry struct {
	lines int // line count at time of read
}

func NewReadCache() *ReadCache {
	return &ReadCache{files: make(map[string]readEntry)}
}

// SetRoot gives the cache the root the session's tools resolve relative paths
// against, so that the cache identifies a file the same way the tools open it.
// Without it a relative and an absolute spelling of one file are two entries,
// and a write under one spelling leaves a read under the other answering from
// content the write has already replaced.
func (rc *ReadCache) SetRoot(root string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.root = root
}

// Root returns the root paths are resolved against.
func (rc *ReadCache) Root() string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.root
}

// keyFor returns the entry key for a path as the model wrote it, and the
// lexical identity it was derived from. The key follows symlinks when it can,
// so one file reached by two routes is one entry; when it cannot — the file does
// not exist yet, most often — the lexical identity is the key, which is still
// spelling-independent.
func (rc *ReadCache) keyFor(path string) (key, lexical string) {
	lexical = lexicalPathIdentity(path, rc.root)
	if canonical := canonicalPathIdentity(lexical); canonical != "" {
		return canonical, lexical
	}
	return lexical, lexical
}

// RecordFullRead marks a file as fully read during this turn.
func (rc *ReadCache) RecordFullRead(path string, lines int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	key, _ := rc.keyFor(path)
	rc.files[key] = readEntry{lines: lines}
}

// Invalidate removes a file from the cache (after write/edit).
//
// It drops both identities the file could have been filed under, because the
// two can differ across the write: a path that resolved through a symlink when
// it was read may not resolve at all once the write has replaced or removed it.
// Dropping one identity too many costs a re-read; dropping one too few is the
// model being told stale bytes are current.
func (rc *ReadCache) Invalidate(path string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	key, lexical := rc.keyFor(path)
	delete(rc.files, key)
	delete(rc.files, lexical)
}

// Check returns a stub message if the file is already fully in context.
// Returns ("", false) if the read should proceed normally.
func (rc *ReadCache) Check(path string) (string, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	key, _ := rc.keyFor(path)
	entry, ok := rc.files[key]
	if !ok {
		return "", false
	}
	return fmt.Sprintf("[already in context — %d lines, read earlier this turn. No need to re-read.]", entry.lines), true
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
