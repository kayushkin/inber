package conversation

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// fileOps are tools that operate on files by path.
var fileOps = map[string]bool{
	"read_file":   true,
	"write_file":  true,
	"edit_file":   true,
	"read_files":  true,
	"write_files": true,
	"edit_files":  true,
}

// toolUseInput decodes a tool_use block's arguments. They arrive as a decoded
// map when the block was built in process and as raw JSON when it came off the
// wire.
func toolUseInput(block anthropic.ContentBlockParamUnion) (map[string]interface{}, bool) {
	if block.OfToolUse == nil || block.OfToolUse.Input == nil {
		return nil, false
	}
	switch v := block.OfToolUse.Input.(type) {
	case map[string]interface{}:
		return v, true
	case json.RawMessage:
		var m map[string]interface{}
		if json.Unmarshal(v, &m) == nil {
			return m, true
		}
	}
	return nil, false
}

// toolUseFilePaths returns every file path a file-operating tool_use touches,
// in the order the call names them. Returns (nil, false) for non-file tools.
//
// Each of the three file tools takes a single path OR a batch — `paths` for
// read_files, `files` for write_files, `edits` for edit_files — and every one
// of their descriptions tells the model the batch form is the one to prefer.
// A reader that understands only the single-path form therefore misses the
// calls that carry the most content.
func toolUseFilePaths(block anthropic.ContentBlockParamUnion) ([]string, bool) {
	if block.OfToolUse == nil || !fileOps[block.OfToolUse.Name] {
		return nil, false
	}
	input, ok := toolUseInput(block)
	if !ok {
		return nil, false
	}

	var paths []string
	if p, ok := input["path"].(string); ok && p != "" {
		paths = append(paths, p)
	}
	// read_files batches bare path strings; write_files and edit_files batch
	// objects that carry their path alongside the content or the replacement.
	if items, ok := input["paths"].([]interface{}); ok {
		for _, item := range items {
			if p, ok := item.(string); ok && p != "" {
				paths = append(paths, p)
			}
		}
	}
	for _, key := range []string{"files", "edits"} {
		items, ok := input[key].([]interface{})
		if !ok {
			continue
		}
		for _, item := range items {
			entry, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if p, ok := entry["path"].(string); ok && p != "" {
				paths = append(paths, p)
			}
		}
	}

	if len(paths) == 0 {
		return nil, false
	}
	return paths, true
}

// toolUseIsPartial reports whether a read call returned part of a file rather
// than all of it. offset and limit only take effect when the call resolves to
// exactly one path (tool-store `tools/fs.go`, ReadFile: a batch of two or more
// reads each file whole), so a batch read is never partial.
func toolUseIsPartial(block anthropic.ContentBlockParamUnion, paths []string) bool {
	if block.OfToolUse == nil {
		return false
	}
	if name := block.OfToolUse.Name; name != "read_file" && name != "read_files" {
		return false
	}
	if len(paths) != 1 {
		return false
	}
	input, ok := toolUseInput(block)
	if !ok {
		return false
	}
	for _, key := range []string{"offset", "limit"} {
		if f, ok := input[key].(float64); ok && f > 0 {
			return true
		}
	}
	return false
}

// distinct returns names in first-seen order with repeats removed, so a stub
// naming several superseding calls does not repeat one tool three times.
func distinct(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// DeduplicateFileRefs scans the conversation and replaces older tool results
// for the same file path with a compact stub. Only the latest read/write/edit
// result is kept in full. Returns the number of results deduplicated.
//
// The approach: scan forward, build a map of path → latest reference.
// When we see a second reference to the same path, replace the OLDER one's
// tool_result content with a stub.
func DeduplicateFileRefs(messages []anthropic.MessageParam) int {
	// Pass 1: collect all file tool_use blocks with the paths they touch. One
	// call can name several, so a ref holds a set, not a path.
	type toolRef struct {
		paths     []string
		toolName  string
		toolID    string
		msgIdx    int
		blockIdx  int
		isPartial bool
	}

	var refs []toolRef
	for i, msg := range messages {
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}
		for j, block := range msg.Content {
			paths, ok := toolUseFilePaths(block)
			if !ok {
				continue
			}
			refs = append(refs, toolRef{
				paths:     paths,
				toolName:  block.OfToolUse.Name,
				toolID:    block.OfToolUse.ID,
				msgIdx:    i,
				blockIdx:  j,
				isPartial: toolUseIsPartial(block, paths),
			})
		}
	}

	if len(refs) < 2 {
		return 0
	}

	// Pass 2: find which paths have duplicates — mark older refs for stubbing.
	// Rules:
	//   - A full read supersedes any earlier read (full or partial) of the same path.
	//   - Partial reads never supersede each other (they cover different ranges).
	//   - Write/edit supersedes earlier reads of the same path.
	//   - A call is only redundant once EVERY path it carries has been read
	//     again in full. Stubbing a batch because one of its files came up
	//     later would delete the only copy of the rest.
	// Map: path → index of latest FULL ref in refs slice
	latestFull := make(map[string]int)
	for i, r := range refs {
		if r.isPartial {
			continue
		}
		for _, p := range r.paths {
			latestFull[p] = i
		}
	}

	// Collect tool IDs to stub (older refs superseded by a later FULL read or write)
	stubIDs := make(map[string]string) // tool_use ID → stub message
	for i, r := range refs {
		supersededBy := make([]string, 0, len(r.paths))
		covered := true
		for _, p := range r.paths {
			latestIdx, hasLatestFull := latestFull[p]
			if !hasLatestFull || latestIdx <= i {
				// Either only partial reads of this path exist, or this ref is
				// itself the latest full one — nothing later replaces it.
				covered = false
				break
			}
			supersededBy = append(supersededBy, refs[latestIdx].toolName)
		}
		if !covered {
			continue
		}
		stub := fmt.Sprintf("[%s superseded — see later %s]",
			strings.Join(r.paths, ", "), strings.Join(distinct(supersededBy), ", "))
		stubIDs[r.toolID] = stub
	}

	if len(stubIDs) == 0 {
		return 0
	}

	// Pass 3: replace tool_result content for stubbed IDs
	deduped := 0
	for i, msg := range messages {
		if msg.Role != anthropic.MessageParamRoleUser {
			continue
		}
		for j, block := range msg.Content {
			if block.OfToolResult == nil {
				continue
			}
			stub, ok := stubIDs[block.OfToolResult.ToolUseID]
			if !ok {
				continue
			}

			// Check if already stubbed (idempotent)
			existing := extractToolResultContent(block.OfToolResult.Content)
			if strings.HasPrefix(existing, "[") && strings.Contains(existing, "superseded") {
				continue
			}

			messages[i].Content[j] = anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: block.OfToolResult.ToolUseID,
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{
							OfText: &anthropic.TextBlockParam{
								Text: stub,
							},
						},
					},
				},
			}
			deduped++
		}
	}

	// Pass 4: also truncate the older tool_use inputs (save ~50 tok per dedup)
	for i, msg := range messages {
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}
		for j, block := range msg.Content {
			if block.OfToolUse == nil {
				continue
			}
			if _, ok := stubIDs[block.OfToolUse.ID]; ok {
				messages[i].Content[j] = anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    block.OfToolUse.ID,
						Name:  block.OfToolUse.Name,
						Input: map[string]interface{}{"_deduped": true},
					},
				}
			}
		}
	}

	return deduped
}
