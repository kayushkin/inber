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
// turn".
//
// The scope does NOT make the invalidation set safe to narrow to the file
// tools, and this comment used to say it did — the claim was that "a shell
// `sed -i`, a `git checkout` or a human editing a file between turns all happen
// while the cache is empty". Two of those three are `shell_commands`, which is
// a tool, so it runs *inside* a turn with the cache full: read a file, run
// `sed -i` on it, read it again, and the second read was answered with
// "already in context ... No need to re-read." over content the shell had
// already replaced. Only the human-between-turns case was ever covered by the
// turn scope.
//
// So invalidation splits by what a call's own input can name, not by whether
// the tool is a "file tool" — see invalidatesEverything.
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

// keyFor returns the entry key for a path as the model wrote it. The key
// follows symlinks when it can, so one file reached by two routes is one entry;
// when neither the file nor its directory can be resolved, the lexical identity
// stands in, which is still spelling-independent.
func (rc *ReadCache) keyFor(path string) string {
	lexical := lexicalPathIdentity(path, rc.root)
	if canonical := canonicalPathIdentity(lexical); canonical != "" {
		return canonical
	}
	return lexical
}

// RecordFullRead marks a file as fully read during this turn.
func (rc *ReadCache) RecordFullRead(path string, lines int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.files[rc.keyFor(path)] = readEntry{lines: lines}
}

// Invalidate removes a file from the cache (after write/edit).
func (rc *ReadCache) Invalidate(path string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	delete(rc.files, rc.keyFor(path))
}

// InvalidateAll forgets every entry. It is what a call whose input cannot name
// the files it writes leaves behind: the cache's promise is "read earlier this
// turn and not modified since", and after `shell_commands` ran there is no file
// that promise still holds for.
//
// Forgetting too much costs a re-read. Forgetting too little hands the model
// stale bytes under a stub that tells it not to look again. Those are not
// comparable, so this is deliberately the blunt instrument.
func (rc *ReadCache) InvalidateAll() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	clear(rc.files)
}

// Check returns a stub message if the file is already fully in context.
// Returns ("", false) if the read should proceed normally.
func (rc *ReadCache) Check(path string) (string, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	entry, ok := rc.files[rc.keyFor(path)]
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

// invalidatesEverything reports whether a call can modify files that its own
// input does not name. Such a call tells us that something changed and refuses
// to say what, so the only sound answer is to stop trusting the whole cache.
//
// The split is by what the input can name, NOT by whether the tool is a "file
// tool" — that was the mistake this function exists to correct. Measured
// against the real implementations, 2026-08-03:
//
//   - shell_commands (tool-store tools/shell.go) runs an arbitrary command.
//     `sed -i`, `git checkout`, `gofmt -w`, `make` and `patch` are all one call
//     and none of them appears in the input as a path this cache could parse.
//   - deploy (tools/deploy.go) posts to /api/forge/deploy, whose own
//     description says it "Commits any uncommitted changes first" and which
//     builds the slot. That work happens in another process against this
//     working tree, so what it rewrites is not knowable from here.
//   - task_plan and scratchpad (tool-store tools/task_plan.go:205,
//     tools/scratchpad.go:129) each os.WriteFile a path derived from repoRoot
//     rather than from the call, and task_plan additionally reaches
//     RunBuildCheck -> a subprocess when the plan empties.
//
// Every other registered tool either names its paths (write_files, edit_files
// — handled by isFileWrite) or writes nothing at all.
// TestEveryToolIsNamedByTheReadCacheInvalidationRules pins that partition, so
// adding a tool to the registry without classifying it reddens the suite.
func invalidatesEverything(toolName string) bool {
	switch toolName {
	case "shell_commands", "deploy", "task_plan", "scratchpad":
		return true
	}
	return false
}

// writesNamedPaths reports whether a call writes exactly the paths its own
// input names, which is what lets isFileWrite invalidate precisely.
//
// write_file and edit_file are the pre-tool-store spellings. They are kept
// because a stale entry is the failure this file is about, and the cost of
// carrying two dead names is nothing; see guard.isDangerous, which lost the
// live ones to that same rename and failed open.
func writesNamedPaths(toolName string) bool {
	switch toolName {
	case "write_file", "write_files", "edit_file", "edit_files":
		return true
	}
	return false
}

// Read-cache effects, as returned by ReadCacheEffect.
const (
	// ReadCacheUnaffected — the call writes no file this cache could hold.
	ReadCacheUnaffected = "none"
	// ReadCacheNamedPaths — the call writes the paths its input names, and
	// only those.
	ReadCacheNamedPaths = "named-paths"
	// ReadCacheEverything — the call can write files its input does not name,
	// so no entry survives it.
	ReadCacheEverything = "everything"
)

// ReadCacheEffect names what a tool call does to the read cache, from the tool
// name alone.
//
// It is exported so the partition can be pinned against the real tool set. The
// agent package cannot import inber's tools package — tools imports agent — so
// the completeness test lives over there and reaches back through this, the
// same shape as server/tool_classification_test.go, which exists because guard
// cannot import server.
func ReadCacheEffect(toolName string) string {
	switch {
	case invalidatesEverything(toolName):
		return ReadCacheEverything
	case writesNamedPaths(toolName):
		return ReadCacheNamedPaths
	default:
		return ReadCacheUnaffected
	}
}

// isFileWrite checks if a tool call is a write/edit that should invalidate the cache.
func isFileWrite(toolName, rawInput string) []string {
	if !writesNamedPaths(toolName) {
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
