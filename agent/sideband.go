package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Sideband fields are injected into every tool call's schema.
// They let the agent do bookkeeping (complete tasks, take notes)
// without wasting an API turn on dedicated tool calls.

const (
	sbDone  = "done"  // array of task indices to complete
	sbNote  = "note"  // {key, value} to upsert scratchpad note
	sbSplit = "split" // {index, into: ["subtask1", ...]} to break down a task
)

// SidebandCallbacks are called when sideband fields are present.
type SidebandCallbacks struct {
	// CompleteTasks marks task indices as done and removes them.
	CompleteTasks func(indices []int) error
	// SaveNote upserts a scratchpad note.
	SaveNote func(key, value string) error
	// SplitTask replaces a task at index with subtasks.
	SplitTask func(index int, subtasks []string) error
}

// sidebandData parsed from tool input.
type sidebandData struct {
	Done  []int         `json:"done,omitempty"`
	Note  *noteData     `json:"note,omitempty"`
	Split *splitData    `json:"split,omitempty"`
}

type noteData struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type splitData struct {
	Index int      `json:"index"`
	Into  []string `json:"into"`
}

// sidebandSchema returns the schema properties for sideband fields.
func sidebandSchema() map[string]any {
	return map[string]any{
		sbDone: map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "integer"},
			"description": "Task indices (0-based) to mark complete. Use on any tool call — no separate turn needed.",
		},
		sbNote: map[string]any{
			"type": "object",
			"description": "Save a working note (e.g. file structure, API endpoint). Always visible, never pruned.",
			"properties": map[string]any{
				"key":   map[string]any{"type": "string", "description": "Note key"},
				"value": map[string]any{"type": "string", "description": "Note content"},
			},
		},
		sbSplit: map[string]any{
			"type": "object",
			"description": "Break a task into subtasks when it's too broad.",
			"properties": map[string]any{
				"index": map[string]any{"type": "integer", "description": "Task index to replace"},
				"into":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Subtask names"},
			},
		},
	}
}

// extractSideband parses and removes sideband fields from raw tool input JSON.
// Returns cleaned input and any sideband data found.
func extractSideband(rawInput string) (cleanInput string, sb *sidebandData) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(rawInput), &parsed); err != nil {
		return rawInput, nil
	}

	var hasSideband bool
	var data sidebandData

	if doneRaw, ok := parsed[sbDone]; ok {
		hasSideband = true
		delete(parsed, sbDone)
		if doneBytes, err := json.Marshal(doneRaw); err == nil {
			_ = json.Unmarshal(doneBytes, &data.Done)
		}
	}

	if noteRaw, ok := parsed[sbNote]; ok {
		hasSideband = true
		delete(parsed, sbNote)
		if noteBytes, err := json.Marshal(noteRaw); err == nil {
			var nd noteData
			if json.Unmarshal(noteBytes, &nd) == nil && nd.Key != "" {
				data.Note = &nd
			}
		}
	}

	if splitRaw, ok := parsed[sbSplit]; ok {
		hasSideband = true
		delete(parsed, sbSplit)
		if splitBytes, err := json.Marshal(splitRaw); err == nil {
			var sd splitData
			if json.Unmarshal(splitBytes, &sd) == nil && len(sd.Into) > 0 {
				data.Split = &sd
			}
		}
	}

	if !hasSideband {
		return rawInput, nil
	}

	cleanBytes, err := json.Marshal(parsed)
	if err != nil {
		return rawInput, &data
	}
	return string(cleanBytes), &data
}

// processSideband executes sideband callbacks and returns a summary string.
func processSideband(sb *sidebandData, cb *SidebandCallbacks) string {
	if sb == nil || cb == nil {
		return ""
	}

	var parts []string

	if len(sb.Done) > 0 && cb.CompleteTasks != nil {
		if err := cb.CompleteTasks(sb.Done); err != nil {
			parts = append(parts, fmt.Sprintf("[task error: %s]", err))
		} else {
			parts = append(parts, fmt.Sprintf("[✓ completed %d task(s)]", len(sb.Done)))
		}
	}

	if sb.Note != nil && cb.SaveNote != nil {
		if err := cb.SaveNote(sb.Note.Key, sb.Note.Value); err != nil {
			parts = append(parts, fmt.Sprintf("[note error: %s]", err))
		} else {
			parts = append(parts, fmt.Sprintf("[📝 noted: %s]", sb.Note.Key))
		}
	}

	if sb.Split != nil && cb.SplitTask != nil {
		if err := cb.SplitTask(sb.Split.Index, sb.Split.Into); err != nil {
			parts = append(parts, fmt.Sprintf("[split error: %s]", err))
		} else {
			parts = append(parts, fmt.Sprintf("[📋 split into %d subtasks]", len(sb.Split.Into)))
		}
	}

	return strings.Join(parts, " ")
}
