package conversation

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// fileOps are tools that operate on files by path.
var fileOps = map[string]bool{
	"read_file":  true,
	"write_file": true,
	"edit_file":  true,
	"read_files":  true,
	"write_files": true,
	"edit_files":  true,
}

// toolUseFilePath extracts the file path from a tool_use block if it's a file operation.
// Returns ("", false) for non-file tools.
func toolUseFilePath(block anthropic.ContentBlockParamUnion) (string, bool) {
	if block.OfToolUse == nil {
		return "", false
	}
	if !fileOps[block.OfToolUse.Name] {
		return "", false
	}

	// Input is interface{} — could be map or raw JSON
	input := block.OfToolUse.Input
	if input == nil {
		return "", false
	}

	switch v := input.(type) {
	case map[string]interface{}:
		if p, ok := v["path"].(string); ok {
			return p, true
		}
	case json.RawMessage:
		var m map[string]interface{}
		if json.Unmarshal(v, &m) == nil {
			if p, ok := m["path"].(string); ok {
				return p, true
			}
		}
	}
	return "", false
}

// fileRef tracks where a file was last referenced in the conversation.
type fileRef struct {
	msgIdx   int    // message index containing the tool_use
	blockIdx int    // block index within that message
	resultMsgIdx   int    // message index containing the tool_result
	resultBlockIdx int   // block index of the result
	toolName string // read_file, write_file, edit_file
	toolID   string // tool_use ID for matching with tool_result
}

// DeduplicateFileRefs scans the conversation and replaces older tool results
// for the same file path with a compact stub. Only the latest read/write/edit
// result is kept in full. Returns the number of results deduplicated.
//
// The approach: scan forward, build a map of path → latest reference.
// When we see a second reference to the same path, replace the OLDER one's
// tool_result content with a stub.
func DeduplicateFileRefs(messages []anthropic.MessageParam) int {
	// Pass 1: collect all file tool_use blocks with their paths
	type toolRef struct {
		path     string
		toolName string
		toolID   string
		msgIdx   int
		blockIdx int
	}

	var refs []toolRef
	for i, msg := range messages {
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}
		for j, block := range msg.Content {
			path, ok := toolUseFilePath(block)
			if !ok {
				continue
			}
			refs = append(refs, toolRef{
				path:     path,
				toolName: block.OfToolUse.Name,
				toolID:   block.OfToolUse.ID,
				msgIdx:   i,
				blockIdx: j,
			})
		}
	}

	if len(refs) < 2 {
		return 0
	}

	// Pass 2: find which paths have duplicates — mark all but the latest for stubbing
	// Map: path → index of latest ref in refs slice
	latest := make(map[string]int)
	for i, r := range refs {
		latest[r.path] = i
	}

	// Collect tool IDs to stub (older refs for same path)
	stubIDs := make(map[string]string) // tool_use ID → stub message
	for i, r := range refs {
		if latest[r.path] != i {
			// This is an older reference — mark its tool_result for stubbing
			stub := fmt.Sprintf("[%s superseded — see later %s]", r.path, refs[latest[r.path]].toolName)
			stubIDs[r.toolID] = stub
		}
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
