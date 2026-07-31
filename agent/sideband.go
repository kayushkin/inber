package agent

import (
	"context"
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
	// CompleteTasks marks task indices as done and removes them. Completing
	// the last task fires the project's build command, which is the one
	// sideband callback that runs a subprocess, so it takes the turn's context
	// and an interrupt reaches the build.
	CompleteTasks func(ctx context.Context, indices []int) error
	// SaveNote upserts a scratchpad note.
	SaveNote func(key, value string) error
	// SplitTask replaces a task at index with subtasks.
	SplitTask func(index int, subtasks []string) error
}

// sidebandData parsed from tool input.
type sidebandData struct {
	Done  []int      `json:"done,omitempty"`
	Note  *noteData  `json:"note,omitempty"`
	Split *splitData `json:"split,omitempty"`
	// Ignored names each sideband field that was taken out of the tool input
	// and could not be used, with the reason. A sideband field is an
	// instruction from the model that is deleted from the input whatever
	// happens to it, so one that cannot be carried out has to be reported —
	// otherwise the model asks for a task to be completed and learns nothing
	// at all, which reads to it exactly like success.
	Ignored []string `json:"-"`
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

	ignore := func(field, reason string) {
		data.Ignored = append(data.Ignored, fmt.Sprintf("%s ignored: %s", field, reason))
	}

	if doneRaw, ok := parsed[sbDone]; ok {
		hasSideband = true
		delete(parsed, sbDone)
		doneBytes, err := json.Marshal(doneRaw)
		switch {
		case err != nil:
			ignore(sbDone, "it could not be read back out of the tool input")
		default:
			if err := json.Unmarshal(doneBytes, &data.Done); err != nil {
				ignore(sbDone, "it is not a list of task indices")
			} else if len(data.Done) == 0 {
				ignore(sbDone, "it names no task")
			}
		}
	}

	if noteRaw, ok := parsed[sbNote]; ok {
		hasSideband = true
		delete(parsed, sbNote)
		noteBytes, err := json.Marshal(noteRaw)
		switch {
		case err != nil:
			ignore(sbNote, "it could not be read back out of the tool input")
		default:
			var nd noteData
			if err := json.Unmarshal(noteBytes, &nd); err != nil {
				ignore(sbNote, "it is not an object with a key and a value")
			} else if nd.Key == "" {
				ignore(sbNote, "it has no key to save under")
			} else {
				data.Note = &nd
			}
		}
	}

	if splitRaw, ok := parsed[sbSplit]; ok {
		hasSideband = true
		delete(parsed, sbSplit)
		splitBytes, err := json.Marshal(splitRaw)
		switch {
		case err != nil:
			ignore(sbSplit, "it could not be read back out of the tool input")
		default:
			var sd splitData
			if err := json.Unmarshal(splitBytes, &sd); err != nil {
				ignore(sbSplit, "it is not an object with an index and subtasks")
			} else if len(sd.Into) == 0 {
				ignore(sbSplit, "it names no subtasks to split into")
			} else {
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
//
// It reports what it could not do as well as what it did. The sideband schema
// is injected into every tool of every agent (agent_run.go), so a model can
// always write "done" — including to an agent that has no task list behind it,
// where nothing is listening and the field is deleted from the input anyway.
// Saying so is the difference between a task the model knows was not completed
// and one it believes was.
func processSideband(ctx context.Context, sb *sidebandData, cb *SidebandCallbacks) string {
	if sb == nil {
		return ""
	}

	var parts []string
	for _, ignored := range sb.Ignored {
		parts = append(parts, fmt.Sprintf("[%s]", ignored))
	}

	unhandled := func(field string) {
		parts = append(parts, fmt.Sprintf("[%s ignored: this agent has nothing to apply it to]", field))
	}

	if len(sb.Done) > 0 {
		switch {
		case cb == nil || cb.CompleteTasks == nil:
			unhandled(sbDone)
		default:
			if err := cb.CompleteTasks(ctx, sb.Done); err != nil {
				parts = append(parts, fmt.Sprintf("[task error: %s]", err))
			} else {
				parts = append(parts, fmt.Sprintf("[✓ completed %d task(s)]", len(sb.Done)))
			}
		}
	}

	if sb.Note != nil {
		switch {
		case cb == nil || cb.SaveNote == nil:
			unhandled(sbNote)
		default:
			if err := cb.SaveNote(sb.Note.Key, sb.Note.Value); err != nil {
				parts = append(parts, fmt.Sprintf("[note error: %s]", err))
			} else {
				parts = append(parts, fmt.Sprintf("[📝 noted: %s]", sb.Note.Key))
			}
		}
	}

	if sb.Split != nil {
		switch {
		case cb == nil || cb.SplitTask == nil:
			unhandled(sbSplit)
		default:
			if err := cb.SplitTask(sb.Split.Index, sb.Split.Into); err != nil {
				parts = append(parts, fmt.Sprintf("[split error: %s]", err))
			} else {
				parts = append(parts, fmt.Sprintf("[📋 split into %d subtasks]", len(sb.Split.Into)))
			}
		}
	}

	return strings.Join(parts, " ")
}
